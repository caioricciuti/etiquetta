package identification

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Identity contains visitor identification data
type Identity struct {
	SessionID   string
	VisitorHash string
}

// Generator creates session and visitor identifiers
type Generator struct {
	secretKey      string
	sessionTimeout time.Duration
}

// New creates a new identity generator
func New(secretKey string, sessionTimeoutMinutes int) *Generator {
	return &Generator{
		secretKey:      secretKey,
		sessionTimeout: time.Duration(sessionTimeoutMinutes) * time.Minute,
	}
}

// GenerateSessionID creates a session ID based on IP, UA, visitor hash, and time window.
// The visitor hash differentiates users who share the same IP and browser (e.g. corporate offices).
// This is GDPR-compliant as it's server-side only (no cookies).
func (g *Generator) GenerateSessionID(ip, userAgent, visitorHash string) string {
	// Round timestamp to session window
	windowStart := time.Now().Truncate(g.sessionTimeout)

	// Create HMAC of IP + UA + visitor hash + time window
	data := ip + "|" + userAgent + "|" + visitorHash + "|" + windowStart.Format(time.RFC3339)
	return g.hmacHash(data)
}

// GenerateVisitorHash creates a fallback visitor hash from IP subnet + UA
// Used when client doesn't provide a fingerprint
func (g *Generator) GenerateVisitorHash(ip, userAgent string) string {
	// Use /24 subnet for IPv4 privacy
	subnet := maskIPSubnet(ip)
	data := subnet + "|" + userAgent
	return g.hmacHash(data)
}

// ValidateClientFingerprint checks if a client fingerprint looks valid
func ValidateClientFingerprint(fingerprint string) bool {
	// Should be a hex string of reasonable length
	if len(fingerprint) < 16 || len(fingerprint) > 64 {
		return false
	}

	// Should be valid hex
	_, err := hex.DecodeString(fingerprint)
	return err == nil
}

func (g *Generator) hmacHash(data string) string {
	h := hmac.New(sha256.New, []byte(g.secretKey))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func maskIPSubnet(ip string) string {
	// Simple /24 masking for IPv4
	parts := splitIP(ip)
	if len(parts) >= 4 {
		return parts[0] + "." + parts[1] + "." + parts[2] + ".0"
	}
	return ip
}

func splitIP(ip string) []string {
	var parts []string
	current := ""
	for _, c := range ip {
		if c == '.' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
