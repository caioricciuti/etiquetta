package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/settings"
)

// ListUsers returns all users with their domain access info
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, _ := h.db.Conn().Query("SELECT id, email, name, role, created_at FROM users ORDER BY created_at DESC")
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var id, email, name, role string
		var createdAt int64
		rows.Scan(&id, &email, &name, &role, &createdAt)

		// Get domain access for non-admin users
		domainIDs := make([]string, 0)
		if role != "admin" {
			daRows, err := h.db.Conn().Query("SELECT domain_id FROM domain_access WHERE user_id = ?", id)
			if err == nil {
				for daRows.Next() {
					var domainID string
					daRows.Scan(&domainID)
					domainIDs = append(domainIDs, domainID)
				}
				daRows.Close()
			}
		}

		users = append(users, map[string]interface{}{
			"id":         id,
			"email":      email,
			"name":       name,
			"role":       role,
			"created_at": createdAt,
			"domain_ids": domainIDs,
		})
	}

	writeJSON(w, http.StatusOK, users)
}

// CreateUser creates a new user
func (h *Handlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email     string   `json:"email"`
		Password  string   `json:"password"`
		Name      string   `json:"name"`
		Role      string   `json:"role"`
		DomainIDs []string `json:"domain_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate email
	if input.Email == "" || !strings.Contains(input.Email, "@") {
		writeError(w, http.StatusBadRequest, "Invalid email address")
		return
	}

	// Validate password
	if len(input.Password) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}

	// Validate role
	if input.Role != "admin" && input.Role != "viewer" {
		input.Role = "viewer" // Default to viewer if invalid
	}

	// Check user limit
	var count int
	h.db.Conn().QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	maxUsers := h.licenseManager.GetLimit("max_users")
	if maxUsers != -1 && count >= maxUsers {
		writeError(w, http.StatusPaymentRequired, "User limit reached")
		return
	}

	// Check if email already exists
	var existingID string
	err := h.db.Conn().QueryRow("SELECT id FROM users WHERE email = ?", input.Email).Scan(&existingID)
	if err == nil {
		writeError(w, http.StatusConflict, "Email already exists")
		return
	}

	// Hash password
	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	id := generateID()
	now := time.Now().UnixMilli()

	_, err = h.db.Conn().Exec(
		"INSERT INTO users (id, email, password_hash, name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, input.Email, passwordHash, input.Name, input.Role, now, now,
	)

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Set domain access for non-admin users
	if input.Role != "admin" && len(input.DomainIDs) > 0 {
		stmt, err := h.db.Conn().Prepare("INSERT INTO domain_access (user_id, domain_id, created_at) VALUES (?, ?, ?)")
		if err == nil {
			defer stmt.Close()
			for _, domainID := range input.DomainIDs {
				stmt.Exec(id, domainID, now)
			}
		}
	}

	h.logAudit(r, "create", "user", id, fmt.Sprintf("Created user %s (role: %s)", input.Email, input.Role))
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// DeleteUser removes a user and their domain access entries
func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Clean up domain access
	h.db.Conn().Exec("DELETE FROM domain_access WHERE user_id = ?", id)

	_, err := h.db.Conn().Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.logAudit(r, "delete", "user", id, "User deleted")
	w.WriteHeader(http.StatusNoContent)
}

// UpdateUser updates a user's details
func (h *Handlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input struct {
		Name     string `json:"name"`
		Role     string `json:"role"`
		Password string `json:"password,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Check if user exists
	var existingID string
	err := h.db.Conn().QueryRow("SELECT id FROM users WHERE id = ?", id).Scan(&existingID)
	if err != nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	// Validate role if provided
	if input.Role != "" && input.Role != "admin" && input.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "Role must be 'admin' or 'viewer'")
		return
	}

	now := time.Now().UnixMilli()

	// If password is provided, validate and hash it
	if input.Password != "" {
		if len(input.Password) < 8 {
			writeError(w, http.StatusBadRequest, "Password must be at least 8 characters")
			return
		}

		passwordHash, err := auth.HashPassword(input.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to hash password")
			return
		}

		_, err = h.db.Conn().Exec(
			"UPDATE users SET name = COALESCE(NULLIF(?, ''), name), role = COALESCE(NULLIF(?, ''), role), password_hash = ?, updated_at = ? WHERE id = ?",
			input.Name, input.Role, passwordHash, now, id,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		_, err = h.db.Conn().Exec(
			"UPDATE users SET name = COALESCE(NULLIF(?, ''), name), role = COALESCE(NULLIF(?, ''), role), updated_at = ? WHERE id = ?",
			input.Name, input.Role, now, id,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	h.logAudit(r, "update", "user", id, fmt.Sprintf("Updated user (name: %s, role: %s)", input.Name, input.Role))
	w.WriteHeader(http.StatusNoContent)
}

// ListDomains returns domains filtered by user access (admins see all, viewers see assigned only)
func (h *Handlers) ListDomains(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetUserFromContext(r.Context())

	var query string
	var args []interface{}

	if claims != nil && claims.Role != "admin" {
		// Non-admin: only show domains they have access to
		query = `
			SELECT d.id, d.name, d.domain, d.site_id, d.created_by, d.created_at, d.is_active
			FROM domains d
			INNER JOIN domain_access da ON da.domain_id = d.id
			WHERE da.user_id = ?
			ORDER BY d.created_at DESC
		`
		args = []interface{}{claims.UserID}
	} else {
		query = `
			SELECT id, name, domain, site_id, created_by, created_at, is_active
			FROM domains
			ORDER BY created_at DESC
		`
	}

	rows, err := h.db.Conn().Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	domains := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, name, domain string
		var siteID, createdBy *string
		var createdAt int64
		var isActive int

		rows.Scan(&id, &name, &domain, &siteID, &createdBy, &createdAt, &isActive)
		domains = append(domains, map[string]interface{}{
			"id":         id,
			"name":       name,
			"domain":     domain,
			"site_id":    siteID,
			"created_by": createdBy,
			"created_at": createdAt,
			"is_active":  isActive == 1,
		})
	}

	writeJSON(w, http.StatusOK, domains)
}

// CreateDomain adds a new domain
func (h *Handlers) CreateDomain(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetUserFromContext(r.Context())

	var input struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if input.Name == "" || input.Domain == "" {
		writeError(w, http.StatusBadRequest, "Name and domain are required")
		return
	}

	// Check domain limit based on license tier
	var domainCount int
	h.db.Conn().QueryRow("SELECT COUNT(*) FROM domains").Scan(&domainCount)

	// Domain limits: community=2, pro=10, enterprise=unlimited
	maxDomains := 2 // community default
	tier := h.licenseManager.GetTier()
	switch tier {
	case "pro":
		maxDomains = 10
	case "enterprise":
		maxDomains = -1 // unlimited
	}

	if maxDomains != -1 && domainCount >= maxDomains {
		writeError(w, http.StatusPaymentRequired, fmt.Sprintf("Domain limit reached (%d domains for %s tier)", maxDomains, tier))
		return
	}

	// Normalize domain (lowercase, no protocol)
	domain := strings.ToLower(input.Domain)
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimSuffix(domain, "/")

	id := auth.GenerateID()
	siteID := "site_" + generateID()[:16] // Generate unique site_id
	now := time.Now().UnixMilli()

	var createdBy *string
	if claims != nil {
		createdBy = &claims.UserID
	}

	_, err := h.db.Conn().Exec(
		"INSERT INTO domains (id, name, domain, site_id, created_by, created_at, is_active) VALUES (?, ?, ?, ?, ?, ?, 1)",
		id, input.Name, domain, siteID, createdBy, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeError(w, http.StatusConflict, "Domain already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Copy global default settings to new domain
	svc := newSettingsService(h)
	if err := svc.CopyDefaultsToDomain(id); err != nil {
		fmt.Printf("[domain] Failed to copy default settings to domain %s: %v\n", id, err)
	}

	h.logAudit(r, "create", "domain", id, fmt.Sprintf("Created domain %s (%s)", input.Name, domain))
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         id,
		"name":       input.Name,
		"domain":     domain,
		"site_id":    siteID,
		"created_at": now,
		"is_active":  true,
	})
}

// DeleteDomain removes a domain and its associated settings and access entries
func (h *Handlers) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.db.Conn().Exec("DELETE FROM domains WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Clean up domain settings and access entries
	h.db.Conn().Exec("DELETE FROM domain_settings WHERE domain_id = ?", id)
	h.db.Conn().Exec("DELETE FROM domain_access WHERE domain_id = ?", id)

	h.logAudit(r, "delete", "domain", id, "Domain deleted")
	w.WriteHeader(http.StatusNoContent)
}

// GetDomainSnippet returns the tracking snippet for a domain
func (h *Handlers) GetDomainSnippet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var domain, siteID string
	err := h.db.Conn().QueryRow("SELECT domain, site_id FROM domains WHERE id = ?", id).Scan(&domain, &siteID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Get the host from the request or use localhost for local dev
	host := r.Host
	scheme := "https"
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
		scheme = "http"
	}

	snippet := fmt.Sprintf(`<!-- Etiquetta Analytics -->
<script defer data-site="%s" src="%s://%s/s.js?id=%s"></script>`, siteID, scheme, host, siteID)

	writeJSON(w, http.StatusOK, map[string]string{
		"domain":  domain,
		"site_id": siteID,
		"snippet": snippet,
	})
}

// GetDomainSettings returns merged settings for a specific domain (global defaults + domain overrides)
func (h *Handlers) GetDomainSettings(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainId")
	if domainID == "" {
		writeError(w, http.StatusBadRequest, "Domain ID required")
		return
	}

	// Verify domain exists
	var exists int
	if err := h.db.Conn().QueryRow("SELECT COUNT(*) FROM domains WHERE id = ?", domainID).Scan(&exists); err != nil || exists == 0 {
		writeError(w, http.StatusNotFound, "Domain not found")
		return
	}

	svc := newSettingsService(h)
	result, err := svc.GetAllForDomain(domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Filter to only domain-scoped keys + their scope indicators
	filtered := make(map[string]string)
	for key, value := range result {
		if settings.IsDomainScopedKey(key) || strings.HasPrefix(key, "scope:") {
			filtered[key] = value
		}
	}

	writeJSON(w, http.StatusOK, filtered)
}

// UpdateDomainSettings updates settings for a specific domain
func (h *Handlers) UpdateDomainSettings(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainId")
	if domainID == "" {
		writeError(w, http.StatusBadRequest, "Domain ID required")
		return
	}

	// Verify domain exists
	var exists int
	if err := h.db.Conn().QueryRow("SELECT COUNT(*) FROM domains WHERE id = ?", domainID).Scan(&exists); err != nil || exists == 0 {
		writeError(w, http.StatusNotFound, "Domain not found")
		return
	}

	var input map[string]string
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Only allow domain-scoped keys
	filtered := make(map[string]string)
	for key, value := range input {
		if settings.IsDomainScopedKey(key) {
			filtered[key] = value
		}
	}

	if len(filtered) == 0 {
		writeError(w, http.StatusBadRequest, "No valid domain-scoped settings provided")
		return
	}

	svc := newSettingsService(h)
	if err := svc.SetManyForDomain(domainID, filtered); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	changedKeys := make([]string, 0, len(filtered))
	for key := range filtered {
		changedKeys = append(changedKeys, key)
	}
	h.logAudit(r, "update", "domain_settings", domainID, "Updated domain settings: "+strings.Join(changedKeys, ", "))
	w.WriteHeader(http.StatusNoContent)
}

// GetUserDomains returns the domains a user has access to
func (h *Handlers) GetUserDomains(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	// Check if user exists and get role
	var role string
	if err := h.db.Conn().QueryRow("SELECT role FROM users WHERE id = ?", userID).Scan(&role); err != nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	// Admins have access to all domains
	if role == "admin" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"access": "all",
			"domain_ids": []string{},
		})
		return
	}

	rows, err := h.db.Conn().Query("SELECT domain_id FROM domain_access WHERE user_id = ?", userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	domainIDs := make([]string, 0)
	for rows.Next() {
		var domainID string
		rows.Scan(&domainID)
		domainIDs = append(domainIDs, domainID)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access":     "restricted",
		"domain_ids": domainIDs,
	})
}

// UpdateUserDomains sets the domains a user has access to
func (h *Handlers) UpdateUserDomains(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	var input struct {
		DomainIDs []string `json:"domain_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Check if user exists
	var role string
	if err := h.db.Conn().QueryRow("SELECT role FROM users WHERE id = ?", userID).Scan(&role); err != nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	// Admins don't need domain access entries
	if role == "admin" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "admins have access to all domains"})
		return
	}

	tx, err := h.db.Conn().Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// Remove all existing access
	if _, err := tx.Exec("DELETE FROM domain_access WHERE user_id = ?", userID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Insert new access entries
	now := time.Now().UnixMilli()
	stmt, err := tx.Prepare("INSERT INTO domain_access (user_id, domain_id, created_at) VALUES (?, ?, ?)")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer stmt.Close()

	for _, domainID := range input.DomainIDs {
		if _, err := stmt.Exec(userID, domainID, now); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.logAudit(r, "update", "domain_access", userID, fmt.Sprintf("Updated domain access: %d domains", len(input.DomainIDs)))
	w.WriteHeader(http.StatusNoContent)
}
