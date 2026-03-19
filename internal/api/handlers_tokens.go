package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/go-chi/chi/v5"
)

// apiKeyResponse is the JSON shape returned when listing API keys.
type apiKeyResponse struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	KeyPrefix  string  `json:"key_prefix"`
	CreatedAt  int64   `json:"created_at"`
	LastUsedAt *int64  `json:"last_used_at"`
	RevokedAt  *int64  `json:"revoked_at"`
}

// CreateAPIToken generates a new read-only API key for the current user.
// The plaintext key is returned only once in the response.
func (h *Handlers) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Generate a random API key: etq_ + 32 random bytes (64 hex chars)
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}
	plainKey := "etq_" + hex.EncodeToString(rawBytes)
	prefix := plainKey[:12] + "..." // etq_XXXXXXXX...

	// Hash the key for storage
	hash := sha256.Sum256([]byte(plainKey))
	keyHash := hex.EncodeToString(hash[:])

	id := auth.GenerateID()
	now := time.Now().UnixMilli()

	_, err := h.db.Conn().Exec(
		"INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, claims.UserID, input.Name, keyHash, prefix, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}

	h.logAudit(r, "create", "api_key", id, fmt.Sprintf("Created API key: %s", input.Name))

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         id,
		"name":       input.Name,
		"key":        plainKey,
		"key_prefix": prefix,
		"created_at": now,
	})
}

// ListAPITokens returns all API keys for the current user.
func (h *Handlers) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	rows, err := h.db.Conn().Query(
		"SELECT id, name, key_prefix, created_at, last_used_at, revoked_at FROM api_keys WHERE user_id = ? ORDER BY created_at DESC",
		claims.UserID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list API keys")
		return
	}
	defer rows.Close()

	keys := []apiKeyResponse{}
	for rows.Next() {
		var k apiKeyResponse
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt); err != nil {
			continue
		}
		keys = append(keys, k)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"keys": keys})
}

// RevokeAPIToken soft-deletes an API key.
func (h *Handlers) RevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	now := time.Now().UnixMilli()

	result, err := h.db.Conn().Exec(
		"UPDATE api_keys SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL",
		now, id, claims.UserID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke API key")
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "API key not found or already revoked")
		return
	}

	h.logAudit(r, "revoke", "api_key", id, "Revoked API key")

	w.WriteHeader(http.StatusNoContent)
}

// ValidateAPIKey looks up an API key by its SHA-256 hash and returns Claims.
// Used as the APIKeyValidator callback for the auth middleware.
func (h *Handlers) ValidateAPIKey(key string) (*auth.Claims, error) {
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	var userID string
	var revokedAt *int64
	var keyID string
	err := h.db.Conn().QueryRow(
		"SELECT id, user_id, revoked_at FROM api_keys WHERE key_hash = ?",
		keyHash,
	).Scan(&keyID, &userID, &revokedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	if revokedAt != nil {
		return nil, fmt.Errorf("API key has been revoked")
	}

	// Look up the user
	var email, role string
	err = h.db.Conn().QueryRow(
		"SELECT email, role FROM users WHERE id = ?",
		userID,
	).Scan(&email, &role)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// Update last_used_at (fire-and-forget)
	go func() {
		h.db.Conn().Exec(
			"UPDATE api_keys SET last_used_at = ? WHERE id = ?",
			time.Now().UnixMilli(), keyID,
		)
	}()

	return &auth.Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
	}, nil
}
