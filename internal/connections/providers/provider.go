package providers

import (
	"context"
	"time"
)

// TokenSet holds OAuth tokens for a provider connection
type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// DailySpend represents one day of spend data for a campaign
type DailySpend struct {
	Date         string // YYYY-MM-DD
	CampaignID   string
	CampaignName string
	CostMicros   int64  // cost in microcurrency (e.g. $1.50 = 1500000)
	Impressions  int
	Clicks       int
	Currency     string
}

// Provider defines the interface all ad platform integrations must implement
type Provider interface {
	// Name returns the provider identifier (e.g. "google_ads")
	Name() string

	// RefreshToken refreshes an expired access token
	RefreshToken(ctx context.Context, refreshToken string) (*TokenSet, error)

	// FetchDailySpend pulls campaign-level spend data for a date range
	FetchDailySpend(ctx context.Context, accessToken, accountID string, start, end time.Time) ([]DailySpend, error)

	// ValidateCredentials checks if the current tokens are still valid
	ValidateCredentials(ctx context.Context, accessToken string) error
}

// Registry holds all registered providers
var Registry = map[string]Provider{}

// Register adds a provider to the registry
func Register(p Provider) {
	Registry[p.Name()] = p
}

// Get returns a provider by name
func Get(name string) (Provider, bool) {
	p, ok := Registry[name]
	return p, ok
}

// Available returns all registered provider names
func Available() []string {
	names := make([]string, 0, len(Registry))
	for name := range Registry {
		names = append(names, name)
	}
	return names
}
