package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/sso"
	"github.com/go-chi/chi/v5"
)

// SSOProvider is the admin-facing representation. client_secret is write-only;
// responses return an empty string and GET masks with "••••" if configured.
type SSOProvider struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Issuer         string `json:"issuer"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret,omitempty"`
	Scopes         string `json:"scopes"`
	AllowedDomains string `json:"allowed_domains"`
	DefaultRole    string `json:"default_role"`
	Enabled        bool   `json:"enabled"`
	HasSecret      bool   `json:"has_secret"`
}

// PublicSSOProvider is returned on the login page (unauthenticated).
type PublicSSOProvider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListPublicSSOProviders returns enabled SSO providers for the login page.
func (h *Handlers) ListPublicSSOProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Conn().QueryContext(r.Context(),
		`SELECT id, name FROM sso_providers WHERE enabled = true ORDER BY name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list providers")
		return
	}
	defer rows.Close()
	out := []PublicSSOProvider{}
	for rows.Next() {
		var p PublicSSOProvider
		if err := rows.Scan(&p.ID, &p.Name); err == nil {
			out = append(out, p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Admin CRUD ---

func (h *Handlers) ListSSOProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Conn().QueryContext(r.Context(), `
		SELECT id, name, kind, issuer, client_id, client_secret, scopes,
			allowed_domains, default_role, enabled
		FROM sso_providers ORDER BY name
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list providers")
		return
	}
	defer rows.Close()
	out := []SSOProvider{}
	for rows.Next() {
		var p SSOProvider
		var secret string
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.Issuer, &p.ClientID,
			&secret, &p.Scopes, &p.AllowedDomains, &p.DefaultRole, &p.Enabled); err != nil {
			continue
		}
		p.HasSecret = secret != ""
		p.ClientSecret = ""
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) CreateSSOProvider(w http.ResponseWriter, r *http.Request) {
	var req SSOProvider
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name == "" || req.Issuer == "" || req.ClientID == "" || req.ClientSecret == "" {
		writeError(w, http.StatusBadRequest, "name, issuer, client_id, client_secret required")
		return
	}
	if req.Kind == "" {
		req.Kind = "oidc"
	}
	if req.Scopes == "" {
		req.Scopes = "openid email profile"
	}
	if req.DefaultRole == "" {
		req.DefaultRole = "viewer"
	}
	id := generateID()
	now := time.Now().UnixMilli()
	_, err := h.db.Conn().ExecContext(r.Context(), `
		INSERT INTO sso_providers (id, name, kind, issuer, client_id, client_secret,
			scopes, allowed_domains, default_role, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.Name, req.Kind, req.Issuer, req.ClientID, req.ClientSecret,
		req.Scopes, req.AllowedDomains, req.DefaultRole, req.Enabled, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create provider")
		return
	}
	h.logAudit(r, "create", "sso_provider", id, req.Name)
	req.ID = id
	req.ClientSecret = ""
	req.HasSecret = true
	writeJSON(w, http.StatusCreated, req)
}

func (h *Handlers) UpdateSSOProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req SSOProvider
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	now := time.Now().UnixMilli()

	// If client_secret is blank, preserve the existing one.
	if req.ClientSecret == "" {
		_, err := h.db.Conn().ExecContext(r.Context(), `
			UPDATE sso_providers SET name=?, issuer=?, client_id=?, scopes=?,
				allowed_domains=?, default_role=?, enabled=?, updated_at=?
			WHERE id=?
		`, req.Name, req.Issuer, req.ClientID, req.Scopes, req.AllowedDomains,
			req.DefaultRole, req.Enabled, now, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update provider")
			return
		}
	} else {
		_, err := h.db.Conn().ExecContext(r.Context(), `
			UPDATE sso_providers SET name=?, issuer=?, client_id=?, client_secret=?,
				scopes=?, allowed_domains=?, default_role=?, enabled=?, updated_at=?
			WHERE id=?
		`, req.Name, req.Issuer, req.ClientID, req.ClientSecret, req.Scopes,
			req.AllowedDomains, req.DefaultRole, req.Enabled, now, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update provider")
			return
		}
	}
	h.logAudit(r, "update", "sso_provider", id, req.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) DeleteSSOProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.db.Conn().ExecContext(r.Context(),
		"DELETE FROM sso_providers WHERE id=?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete")
		return
	}
	h.logAudit(r, "delete", "sso_provider", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Login flow ---

const ssoStateCookie = "etiquetta_sso_state"

type ssoStateClaims struct {
	ProviderID string `json:"p"`
	State      string `json:"s"`
	Nonce      string `json:"n"`
	RedirectTo string `json:"r"`
	Expires    int64  `json:"e"`
}

// InitSSOLogin begins the SSO flow by redirecting to the provider.
// GET /auth/sso/{id}/login?redirect=/some/path
func (h *Handlers) InitSSOLogin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := loadSSOProvider(r.Context(), h.db.Conn(), id)
	if err != nil || !p.Enabled {
		http.Error(w, "provider not found", http.StatusNotFound)
		return
	}

	redirectURI := absoluteURL(r, "/api/auth/sso/"+id+"/callback")
	authURL, state, nonce, err := h.ssoClient.AuthURL(r.Context(), p, redirectURI)
	if err != nil {
		http.Error(w, "provider discovery failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	redirectTo := r.URL.Query().Get("redirect")
	// Require a single leading slash: "//evil.com" and "/\evil.com" parse as
	// protocol-relative URLs and would redirect off-site after login.
	if redirectTo == "" || !strings.HasPrefix(redirectTo, "/") ||
		strings.HasPrefix(redirectTo, "//") || strings.HasPrefix(redirectTo, "/\\") {
		redirectTo = "/"
	}

	cookieVal, _ := json.Marshal(ssoStateClaims{
		ProviderID: id, State: state, Nonce: nonce, RedirectTo: redirectTo,
		Expires: time.Now().Add(15 * time.Minute).Unix(),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     ssoStateCookie,
		Value:    string(cookieVal),
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   900,
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

// SSOCallback finishes the SSO flow.
// GET /auth/sso/{id}/callback?code=...&state=...
func (h *Handlers) SSOCallback(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing code or state")
		return
	}

	cookie, err := r.Cookie(ssoStateCookie)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing state cookie")
		return
	}
	var saved ssoStateClaims
	if err := json.Unmarshal([]byte(cookie.Value), &saved); err != nil {
		writeError(w, http.StatusBadRequest, "corrupt state cookie")
		return
	}
	if saved.State != state || saved.ProviderID != id {
		writeError(w, http.StatusBadRequest, "state mismatch")
		return
	}
	if time.Now().Unix() > saved.Expires {
		writeError(w, http.StatusBadRequest, "state expired")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: ssoStateCookie, Value: "", Path: "/", MaxAge: -1,
	})

	p, err := loadSSOProvider(r.Context(), h.db.Conn(), id)
	if err != nil || !p.Enabled {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}

	redirectURI := absoluteURL(r, "/api/auth/sso/"+id+"/callback")
	info, err := h.ssoClient.Exchange(r.Context(), p, code, redirectURI, saved.Nonce)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "token exchange failed: "+err.Error())
		return
	}
	if info.Email == "" {
		writeError(w, http.StatusBadRequest, "provider did not return email")
		return
	}

	if p.AllowedDomains != "" && !emailDomainAllowed(info.Email, p.AllowedDomains) {
		writeError(w, http.StatusForbidden, "email domain not allowed")
		return
	}

	user, err := h.findOrCreateSSOUser(r.Context(), p, info)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to link user: "+err.Error())
		return
	}

	token, err := h.auth.GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	h.auth.SetAuthCookie(w, token)

	dest := saved.RedirectTo
	if dest == "" {
		dest = "/"
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

func (h *Handlers) findOrCreateSSOUser(
	ctx context.Context,
	p *sso.Provider,
	info *sso.UserInfo,
) (*auth.User, error) {
	// 1. Existing link?
	var userID string
	err := h.db.Conn().QueryRowContext(ctx,
		"SELECT user_id FROM user_identities WHERE provider_id = ? AND subject = ?",
		p.ID, info.Subject,
	).Scan(&userID)
	if err == nil {
		return loadUserCtx(ctx, h.db.Conn(), userID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// 2. Existing user by email → link. Only trust the email claim for linking
	// when the IdP asserts it verified the address; otherwise anyone able to
	// register an IdP account with an arbitrary email could take over the
	// matching local account (including admins).
	if info.EmailVerified {
		err = h.db.Conn().QueryRowContext(ctx,
			"SELECT id FROM users WHERE email = ?", info.Email,
		).Scan(&userID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	} else {
		var existing int
		if err := h.db.Conn().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM users WHERE email = ?", info.Email,
		).Scan(&existing); err != nil {
			return nil, err
		}
		if existing > 0 {
			return nil, errors.New("an account with this email exists but the identity provider did not verify the address")
		}
	}
	if userID == "" {
		// 3. Create new user with a random unusable password hash.
		userID = generateID()
		now := time.Now().UnixMilli()
		name := info.Name
		if name == "" {
			name = info.Email
		}
		randBytes := make([]byte, 32)
		_, _ = rand.Read(randBytes)
		pwHash, _ := auth.HashPassword(hex.EncodeToString(randBytes))
		_, err := h.db.Conn().ExecContext(ctx, `
			INSERT INTO users (id, email, password_hash, name, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, userID, info.Email, pwHash, name, p.DefaultRole, now, now)
		if err != nil {
			return nil, err
		}
	}

	// Record the identity.
	linkID := generateID()
	_, err = h.db.Conn().ExecContext(ctx, `
		INSERT INTO user_identities (id, user_id, provider_id, subject, email, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, linkID, userID, p.ID, info.Subject, info.Email, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}

	return loadUserCtx(ctx, h.db.Conn(), userID)
}

func loadUserCtx(ctx context.Context, db *sql.DB, id string) (*auth.User, error) {
	var u auth.User
	err := db.QueryRowContext(ctx,
		"SELECT id, email, name, role FROM users WHERE id = ?", id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func loadSSOProvider(ctx context.Context, db *sql.DB, id string) (*sso.Provider, error) {
	var p sso.Provider
	err := db.QueryRowContext(ctx, `
		SELECT id, name, issuer, client_id, client_secret, scopes,
			allowed_domains, default_role, enabled
		FROM sso_providers WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Issuer, &p.ClientID, &p.ClientSecret,
		&p.Scopes, &p.AllowedDomains, &p.DefaultRole, &p.Enabled)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func emailDomainAllowed(email, allowedCSV string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, d := range strings.Split(allowedCSV, ",") {
		if strings.EqualFold(strings.TrimSpace(d), domain) {
			return true
		}
	}
	return false
}

func absoluteURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	return scheme + "://" + host + path
}
