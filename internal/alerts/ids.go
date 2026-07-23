package alerts

import (
	"crypto/rand"
	"encoding/hex"
)

// generateID returns a 16-byte hex identifier, matching the style used
// elsewhere in the codebase for primary keys.
func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}

// GenerateSecret returns a 32-byte hex string suitable for HMAC signing.
func GenerateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
