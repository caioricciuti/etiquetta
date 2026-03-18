package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GoogleAds implements the Provider interface for Google Ads
type GoogleAds struct {
	// GetCredentials returns client ID, client secret, and developer token from settings at runtime
	GetCredentials func() (clientID, clientSecret, developerToken string, err error)
}

func (g *GoogleAds) Name() string {
	return "google_ads"
}

func (g *GoogleAds) RefreshToken(ctx context.Context, refreshToken string) (*TokenSet, error) {
	clientID, clientSecret, _, err := g.getCredentials()
	if err != nil {
		return nil, err
	}

	data := url.Values{
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
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
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}

	// Refresh response may not include a new refresh_token — keep the original
	return &TokenSet{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

func (g *GoogleAds) FetchDailySpend(ctx context.Context, accessToken, accountID string, start, end time.Time) ([]DailySpend, error) {
	_, _, devToken, err := g.getCredentials()
	if err != nil {
		return nil, err
	}

	// Strip dashes from account ID (123-456-7890 → 1234567890)
	customerID := strings.ReplaceAll(accountID, "-", "")
	if customerID == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	startDate := start.Format("2006-01-02")
	endDate := end.Format("2006-01-02")

	gaql := fmt.Sprintf(
		`SELECT campaign.id, campaign.name, metrics.cost_micros, metrics.impressions, metrics.clicks, segments.date FROM campaign WHERE segments.date BETWEEN '%s' AND '%s'`,
		startDate, endDate,
	)

	apiURL := fmt.Sprintf("https://googleads.googleapis.com/v18/customers/%s/googleAds:searchStream", customerID)
	reqBody, _ := json.Marshal(map[string]string{"query": gaql})

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("developer-token", devToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google ads API: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google ads API error (status %d): %s", resp.StatusCode, body)
	}

	// searchStream returns an array of batches
	var batches []struct {
		Results []struct {
			Campaign struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"campaign"`
			Metrics struct {
				CostMicros  json.Number `json:"costMicros"`
				Impressions json.Number `json:"impressions"`
				Clicks      json.Number `json:"clicks"`
			} `json:"metrics"`
			Segments struct {
				Date string `json:"date"`
			} `json:"segments"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &batches); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var result []DailySpend
	for _, batch := range batches {
		for _, row := range batch.Results {
			costMicros, _ := row.Metrics.CostMicros.Int64()
			impressions, _ := row.Metrics.Impressions.Int64()
			clicks, _ := row.Metrics.Clicks.Int64()

			result = append(result, DailySpend{
				Date:         row.Segments.Date,
				CampaignID:   row.Campaign.ID,
				CampaignName: row.Campaign.Name,
				CostMicros:   costMicros,
				Impressions:  int(impressions),
				Clicks:       int(clicks),
				Currency:     "USD",
			})
		}
	}

	return result, nil
}

func (g *GoogleAds) ValidateCredentials(ctx context.Context, accessToken string) error {
	_, _, devToken, err := g.getCredentials()
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://googleads.googleapis.com/v18/customers:listAccessibleCustomers", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("developer-token", devToken)

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

func (g *GoogleAds) getCredentials() (clientID, clientSecret, devToken string, err error) {
	if g.GetCredentials == nil {
		return "", "", "", fmt.Errorf("google_ads: credentials not configured")
	}
	return g.GetCredentials()
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func init() {
	// Register with nil credentials — will be configured at runtime via serve.go
	Register(&GoogleAds{})
}
