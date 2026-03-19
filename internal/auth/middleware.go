package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey string

const (
	UserContextKey contextKey = "user"
)

// APIKeyValidator validates an API key and returns user claims.
// Implemented outside the auth package (e.g. in the API layer) where DB access is available.
type APIKeyValidator func(key string) (*Claims, error)

// Middleware creates authentication middleware
type Middleware struct {
	auth            *Auth
	validateAPIKey  APIKeyValidator
}

// NewMiddleware creates a new auth middleware
func NewMiddleware(auth *Auth) *Middleware {
	return &Middleware{auth: auth}
}

// SetAPIKeyValidator registers a callback for validating API keys (etq_ prefixed tokens).
func (m *Middleware) SetAPIKeyValidator(v APIKeyValidator) {
	m.validateAPIKey = v
}

// RequireAuth ensures the request has a valid authentication token.
// Supports both JWT tokens and API keys (etq_ prefix).
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := GetTokenFromRequest(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		var claims *Claims
		var err error

		if strings.HasPrefix(token, "etq_") && m.validateAPIKey != nil {
			claims, err = m.validateAPIKey(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
		} else {
			claims, err = m.auth.ValidateToken(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
		}

		// Add claims to context
		ctx := context.WithValue(r.Context(), UserContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin ensures the request has admin privileges
func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetUserFromContext(r.Context())
		if claims == nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		if claims.Role != "admin" {
			writeError(w, http.StatusForbidden, "admin privileges required")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// OptionalAuth adds user info to context if token is present, but doesn't require it
func (m *Middleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := GetTokenFromRequest(r)
		if token != "" {
			claims, err := m.auth.ValidateToken(token)
			if err == nil {
				ctx := context.WithValue(r.Context(), UserContextKey, claims)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// GetUserFromContext retrieves user claims from context
func GetUserFromContext(ctx context.Context) *Claims {
	claims, ok := ctx.Value(UserContextKey).(*Claims)
	if !ok {
		return nil
	}
	return claims
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
