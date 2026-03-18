package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MetaAds implements the Provider interface for Meta (Facebook) Ads
type MetaAds struct {
	// GetCredentials returns app ID and app secret from settings at runtime
	GetCredentials func() (appID, appSecret string, err error)
}

func (m *MetaAds) Name() string {
	return "meta_ads"
}

func (m *MetaAds) RefreshToken(ctx context.Context, token string) (*TokenSet, error) {
	appID, appSecret, err := m.getCredentials()
	if err != nil {
		return nil, err
	}

	// Exchange short-lived token for a long-lived one
	params := url.Values{
		"grant_type":        {"fb_exchange_token"},
		"client_id":         {appID},
		"client_secret":     {appSecret},
		"fb_exchange_token": {token},
	}

	reqURL := "https://graph.facebook.com/v21.0/oauth/access_token?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	// System User tokens never expire (expires_in == 0) — set 10 years out
	if tokenResp.ExpiresIn == 0 {
		expiresAt = time.Now().Add(10 * 365 * 24 * time.Hour)
	}

	// Meta's exchange returns a single token; use it as both access and refresh
	return &TokenSet{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.AccessToken,
		ExpiresAt:    expiresAt,
	}, nil
}

func (m *MetaAds) FetchDailySpend(ctx context.Context, accessToken, accountID string, start, end time.Time) ([]DailySpend, error) {
	// Normalize account ID: strip "act_" prefix if included by user
	accountID = strings.TrimPrefix(accountID, "act_")
	if accountID == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	startDate := start.Format("2006-01-02")
	endDate := end.Format("2006-01-02")

	params := url.Values{
		"fields":         {"campaign_id,campaign_name,spend,impressions,clicks"},
		"level":          {"campaign"},
		"time_increment": {"1"},
		"time_range":     {fmt.Sprintf(`{"since":"%s","until":"%s"}`, startDate, endDate)},
		"access_token":   {accessToken},
		"limit":          {"500"},
	}

	apiURL := fmt.Sprintf("https://graph.facebook.com/v21.0/act_%s/insights?%s", accountID, params.Encode())

	var result []DailySpend

	for apiURL != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("meta ads API: %w", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("meta ads API error (status %d): %s", resp.StatusCode, body)
		}

		var insightsResp struct {
			Data []struct {
				CampaignID   string `json:"campaign_id"`
				CampaignName string `json:"campaign_name"`
				Spend        string `json:"spend"`
				Impressions  string `json:"impressions"`
				Clicks       string `json:"clicks"`
				DateStart    string `json:"date_start"`
			} `json:"data"`
			Paging struct {
				Next string `json:"next"`
			} `json:"paging"`
		}

		if err := json.Unmarshal(body, &insightsResp); err != nil {
			return nil, fmt.Errorf("parse response: %w", err)
		}

		for _, row := range insightsResp.Data {
			spend, _ := strconv.ParseFloat(row.Spend, 64)
			costMicros := int64(spend * 1_000_000)
			impressions, _ := strconv.Atoi(row.Impressions)
			clicks, _ := strconv.Atoi(row.Clicks)

			result = append(result, DailySpend{
				Date:         row.DateStart,
				CampaignID:   row.CampaignID,
				CampaignName: row.CampaignName,
				CostMicros:   costMicros,
				Impressions:  impressions,
				Clicks:       clicks,
				Currency:     "USD",
			})
		}

		apiURL = insightsResp.Paging.Next
	}

	return result, nil
}

func (m *MetaAds) ValidateCredentials(ctx context.Context, accessToken string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://graph.facebook.com/v21.0/me?access_token="+accessToken, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

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

func (m *MetaAds) getCredentials() (appID, appSecret string, err error) {
	if m.GetCredentials == nil {
		return "", "", fmt.Errorf("meta_ads: credentials not configured")
	}
	return m.GetCredentials()
}

func init() {
	Register(&MetaAds{})
}
