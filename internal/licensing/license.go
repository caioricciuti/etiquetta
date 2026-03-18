package licensing

import "time"

// License tiers
const (
	TierCommunity  = "community"
	TierPro        = "pro"
	TierEnterprise = "enterprise"
)

// Features
const (
	FeaturePerformance   = "performance"
	FeatureErrorTracking = "error_tracking"
	FeatureCustomEvents  = "custom_events"
	FeatureExport        = "export"
	FeatureAPI           = "api"
	FeatureSSO           = "sso"
	FeatureMultiUser     = "multi_user"
	FeatureWhiteLabel    = "white_label"
	FeatureAdFraud       = "ad_fraud"
	FeatureBotDetection  = "bot_detection"
	FeatureConsent       = "consent"
	FeatureTagManager    = "tag_manager"
	FeatureConnections    = "connections"
	FeatureSessionReplay = "session_replay"
)

// License represents a validated license
type License struct {
	ID        string            `json:"license_id"`
	Type      string            `json:"type"`
	Licensee  string            `json:"licensee"`
	ExpiresAt time.Time         `json:"expires_at"`
	Features  map[string]bool   `json:"features"`
	Limits    map[string]int    `json:"limits"`
	IssuedAt  time.Time         `json:"issued_at"`
}

// LicenseFile is the on-disk format
type LicenseFile struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// ValidationState represents the license validation status
type ValidationState string

const (
	StateValid    ValidationState = "valid"
	StateExpired  ValidationState = "expired"
	StateTampered ValidationState = "tampered"
	StateMissing  ValidationState = "missing"
)

// DefaultLimits returns limits for each tier
func DefaultLimits(tier string) map[string]int {
	switch tier {
	case TierEnterprise:
		return map[string]int{
			"max_users":          -1, // unlimited
			"max_retention_days": -1, // unlimited
			"max_connections":    -1, // unlimited
		}
	case TierPro:
		return map[string]int{
			"max_users":          10,
			"max_retention_days": -1, // unlimited — user configurable
			"max_connections":    -1, // unlimited
		}
	default: // community
		return map[string]int{
			"max_users":          3,
			"max_retention_days": 180,
			"max_connections":    1,
		}
	}
}

// DefaultFeatures returns features for each tier
func DefaultFeatures(tier string) map[string]bool {
	switch tier {
	case TierEnterprise:
		return map[string]bool{
			FeaturePerformance:   true,
			FeatureErrorTracking: true,
			FeatureCustomEvents:  true,
			FeatureExport:        true,
			FeatureAPI:           true,
			FeatureSSO:           true,
			FeatureMultiUser:     true,
			FeatureWhiteLabel:    true,
			FeatureAdFraud:       true,
			FeatureBotDetection:  true,
			FeatureConsent:       true,
			FeatureTagManager:    true,
			FeatureConnections:    true,
			FeatureSessionReplay: true,
		}
	case TierPro:
		return map[string]bool{
			FeaturePerformance:   true,
			FeatureErrorTracking: true,
			FeatureCustomEvents:  true,
			FeatureExport:        true,
			FeatureAPI:           true,
			FeatureSSO:           false,
			FeatureMultiUser:     true,
			FeatureWhiteLabel:    false,
			FeatureAdFraud:       true,
			FeatureBotDetection:  true,
			FeatureConsent:       true,
			FeatureTagManager:    true,
			FeatureConnections:    true,
			FeatureSessionReplay: true,
		}
	default: // community
		return map[string]bool{
			FeaturePerformance:   false,
			FeatureErrorTracking: false,
			FeatureCustomEvents:  false,
			FeatureExport:        false,
			FeatureAPI:           false,
			FeatureSSO:           false,
			FeatureMultiUser:     false,
			FeatureWhiteLabel:    false,
			FeatureAdFraud:       false,
			FeatureBotDetection:  true, // Basic bot detection available in community
			FeatureConsent:       false,
			FeatureTagManager:    false,
			FeatureConnections:    true, // Available in community (limited to 1)
			FeatureSessionReplay: false,
		}
	}
}
