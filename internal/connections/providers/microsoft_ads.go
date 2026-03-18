package providers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MicrosoftAds implements the Provider interface for Microsoft (Bing) Ads
type MicrosoftAds struct {
	// GetCredentials returns client ID, client secret, and developer token from settings at runtime
	GetCredentials func() (clientID, clientSecret, developerToken string, err error)
}

func (ms *MicrosoftAds) Name() string {
	return "microsoft_ads"
}

func (ms *MicrosoftAds) RefreshToken(ctx context.Context, refreshToken string) (*TokenSet, error) {
	clientID, clientSecret, _, err := ms.getCredentials()
	if err != nil {
		return nil, err
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
		"scope":         {"https://ads.microsoft.com/.default offline_access"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://login.microsoftonline.com/common/oauth2/v2.0/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed (status %d): %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}

	newRefresh := tokenResp.RefreshToken
	if newRefresh == "" {
		newRefresh = refreshToken
	}

	return &TokenSet{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: newRefresh,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

func (ms *MicrosoftAds) FetchDailySpend(ctx context.Context, accessToken, accountID string, start, end time.Time) ([]DailySpend, error) {
	_, _, devToken, err := ms.getCredentials()
	if err != nil {
		return nil, err
	}

	if accountID == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	startDate := start.Format("2006-01-02")
	endDate := end.Format("2006-01-02")

	// Step 1: Submit report request
	reportReq := map[string]any{
		"ReportRequest": map[string]any{
			"Format":             "Csv",
			"ReportName":         "CampaignPerformance",
			"ReturnOnlyCompleteData": false,
			"Aggregation":        "Daily",
			"Columns": []string{
				"TimePeriod", "CampaignId", "CampaignName",
				"Spend", "Impressions", "Clicks", "CurrencyCode",
			},
			"Scope": map[string]any{
				"AccountIds": []string{accountID},
			},
			"Time": map[string]any{
				"CustomDateRangeStart": map[string]int{
					"Year":  start.Year(),
					"Month": int(start.Month()),
					"Day":   start.Day(),
				},
				"CustomDateRangeEnd": map[string]int{
					"Year":  end.Year(),
					"Month": int(end.Month()),
					"Day":   end.Day(),
				},
			},
			"Type": "CampaignPerformanceReportRequest",
		},
	}

	reqBody, _ := json.Marshal(reportReq)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://reporting.api.bingads.microsoft.com/Reporting/v13/GenerateReport",
		bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	ms.setHeaders(req, accessToken, devToken, accountID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("submit report: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("report submit error (status %d): %s", resp.StatusCode, body)
	}

	var submitResp struct {
		ReportRequestId string `json:"ReportRequestId"`
	}
	if err := json.Unmarshal(body, &submitResp); err != nil {
		return nil, fmt.Errorf("parse submit response: %w", err)
	}

	// Step 2: Poll for completion
	pollURL := fmt.Sprintf("https://reporting.api.bingads.microsoft.com/Reporting/v13/GetReportStatus?ReportRequestId=%s", submitResp.ReportRequestId)
	var downloadURL string

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		pollReq, err := http.NewRequestWithContext(ctx, "GET", pollURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build poll request: %w", err)
		}
		ms.setHeaders(pollReq, accessToken, devToken, accountID)

		pollResp, err := httpClient.Do(pollReq)
		if err != nil {
			return nil, fmt.Errorf("poll report: %w", err)
		}

		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var statusResp struct {
			Status            string `json:"Status"`
			ReportDownloadUrl string `json:"ReportDownloadUrl"`
		}
		if err := json.Unmarshal(pollBody, &statusResp); err != nil {
			return nil, fmt.Errorf("parse poll response: %w", err)
		}

		if statusResp.Status == "Success" {
			downloadURL = statusResp.ReportDownloadUrl
			break
		}
		if statusResp.Status == "Error" {
			return nil, fmt.Errorf("report generation failed")
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	if downloadURL == "" {
		return nil, fmt.Errorf("report timed out after 60s (date range %s to %s)", startDate, endDate)
	}

	// Step 3: Download and parse CSV
	dlReq, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build download request: %w", err)
	}

	dlResp, err := httpClient.Do(dlReq)
	if err != nil {
		return nil, fmt.Errorf("download report: %w", err)
	}
	defer dlResp.Body.Close()

	dlBody, _ := io.ReadAll(dlResp.Body)
	return ms.parseCSVReport(dlBody)
}

func (ms *MicrosoftAds) parseCSVReport(data []byte) ([]DailySpend, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, nil // empty report
	}

	// Find header row and column indices
	headerIdx := -1
	for i, row := range records {
		if len(row) > 0 && (row[0] == "TimePeriod" || row[0] == "GregorianDate") {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return nil, nil // no data rows
	}

	header := records[headerIdx]
	colIdx := map[string]int{}
	for i, col := range header {
		colIdx[col] = i
	}

	var result []DailySpend
	for _, row := range records[headerIdx+1:] {
		if len(row) <= 1 {
			continue // skip summary/empty rows
		}

		date := safeCol(row, colIdx, "TimePeriod")
		if date == "" {
			date = safeCol(row, colIdx, "GregorianDate")
		}
		if date == "" {
			continue
		}

		spendStr := safeCol(row, colIdx, "Spend")
		spend, _ := strconv.ParseFloat(spendStr, 64)
		costMicros := int64(spend * 1_000_000)

		impressions, _ := strconv.Atoi(safeCol(row, colIdx, "Impressions"))
		clicks, _ := strconv.Atoi(safeCol(row, colIdx, "Clicks"))

		currency := safeCol(row, colIdx, "CurrencyCode")
		if currency == "" {
			currency = "USD"
		}

		result = append(result, DailySpend{
			Date:         date,
			CampaignID:   safeCol(row, colIdx, "CampaignId"),
			CampaignName: safeCol(row, colIdx, "CampaignName"),
			CostMicros:   costMicros,
			Impressions:  impressions,
			Clicks:       clicks,
			Currency:     currency,
		})
	}

	return result, nil
}

func (ms *MicrosoftAds) ValidateCredentials(ctx context.Context, accessToken string) error {
	_, _, devToken, err := ms.getCredentials()
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://campaign.api.bingads.microsoft.com/Api/Advertiser/V13/User/GetUser",
		nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("DeveloperToken", devToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("validate credentials: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("validation failed (status %d): %s", resp.StatusCode, body)
	}
	return nil
}

func (ms *MicrosoftAds) setHeaders(req *http.Request, accessToken, devToken, accountID string) {
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("DeveloperToken", devToken)
	req.Header.Set("AccountId", accountID)
}

func (ms *MicrosoftAds) getCredentials() (clientID, clientSecret, devToken string, err error) {
	if ms.GetCredentials == nil {
		return "", "", "", fmt.Errorf("microsoft_ads: credentials not configured")
	}
	return ms.GetCredentials()
}

func safeCol(row []string, colIdx map[string]int, name string) string {
	idx, ok := colIdx[name]
	if !ok || idx >= len(row) {
		return ""
	}
	return row[idx]
}

func init() {
	Register(&MicrosoftAds{})
}
