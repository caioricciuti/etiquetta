package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/go-chi/chi/v5"
)

type ShareLink struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	DomainID  string `json:"domain_id"`
	Name      string `json:"name"`
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt *int64 `json:"expires_at"`
}

func generateShareToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ListShareLinks returns share links for a domain
func (h *Handlers) ListShareLinks(w http.ResponseWriter, r *http.Request) {
	domainID := r.URL.Query().Get("domain_id")
	if domainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}

	rows, err := h.db.Conn().QueryContext(r.Context(),
		"SELECT id, token, domain_id, name, created_by, created_at, expires_at FROM share_links WHERE domain_id = ? ORDER BY created_at DESC",
		domainID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query share links")
		return
	}
	defer rows.Close()

	var links []ShareLink
	for rows.Next() {
		var l ShareLink
		if err := rows.Scan(&l.ID, &l.Token, &l.DomainID, &l.Name, &l.CreatedBy, &l.CreatedAt, &l.ExpiresAt); err != nil {
			continue
		}
		links = append(links, l)
	}
	if links == nil {
		links = []ShareLink{}
	}
	writeJSON(w, http.StatusOK, links)
}

// CreateShareLink creates a new public share link for a domain
func (h *Handlers) CreateShareLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DomainID  string `json:"domain_id"`
		Name      string `json:"name"`
		ExpiresIn *int   `json:"expires_in_days"` // nil = never expires
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DomainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}
	if req.Name == "" {
		req.Name = "Shared Dashboard"
	}

	claims := auth.GetUserFromContext(r.Context())
	createdBy := ""
	if claims != nil {
		createdBy = claims.UserID
	}

	id := generateID()
	token := generateShareToken()
	now := time.Now().UnixMilli()

	var expiresAt *int64
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		exp := time.Now().Add(time.Duration(*req.ExpiresIn) * 24 * time.Hour).UnixMilli()
		expiresAt = &exp
	}

	_, err := h.db.Conn().ExecContext(r.Context(),
		"INSERT INTO share_links (id, token, domain_id, name, created_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, token, req.DomainID, req.Name, createdBy, now, expiresAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create share link")
		return
	}

	h.logAudit(r, "create", "share_link", id, req.Name)

	writeJSON(w, http.StatusCreated, ShareLink{
		ID:        id,
		Token:     token,
		DomainID:  req.DomainID,
		Name:      req.Name,
		CreatedBy: createdBy,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	})
}

// DeleteShareLink deletes a share link
func (h *Handlers) DeleteShareLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := h.db.Conn().ExecContext(r.Context(), "DELETE FROM share_links WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete share link")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "share link not found")
		return
	}
	h.logAudit(r, "delete", "share_link", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// validateShareToken validates a share token and returns the domain info.
// Returns domain name and site_id, or empty strings if invalid.
func (h *Handlers) validateShareToken(token string) (domainID string, domainName string, ok bool) {
	var expiresAt *int64
	err := h.db.Conn().QueryRow(
		"SELECT sl.domain_id, d.domain, sl.expires_at FROM share_links sl JOIN domains d ON sl.domain_id = d.id WHERE sl.token = ? AND d.is_active = 1",
		token,
	).Scan(&domainID, &domainName, &expiresAt)
	if err != nil {
		return "", "", false
	}
	if expiresAt != nil && *expiresAt < time.Now().UnixMilli() {
		return "", "", false
	}
	return domainID, domainName, true
}

// GetSharedDashboard returns the dashboard config for a share token
func (h *Handlers) GetSharedDashboard(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "shareToken")
	_, domainName, ok := h.validateShareToken(token)
	if !ok {
		writeError(w, http.StatusNotFound, "invalid or expired share link")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"domain": domainName,
	})
}

// SharedStatsProxy validates the share token then proxies the request to the appropriate stat handler.
// The stat type is extracted from the URL path after /api/shared/stats/{token}/
func (h *Handlers) SharedStatsProxy(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "shareToken")
	_, domainName, ok := h.validateShareToken(token)
	if !ok {
		writeError(w, http.StatusNotFound, "invalid or expired share link")
		return
	}

	// Inject domain filter so all queries are scoped
	q := r.URL.Query()
	q.Set("domain", domainName)
	r.URL.RawQuery = q.Encode()

	statType := chi.URLParam(r, "*")

	switch statType {
	case "overview":
		h.GetStatsOverview(w, r)
	case "timeseries":
		h.GetStatsTimeseries(w, r)
	case "pages":
		h.GetStatsPages(w, r)
	case "referrers":
		h.GetStatsReferrers(w, r)
	case "geo":
		h.GetStatsGeo(w, r)
	case "map":
		h.GetStatsMapData(w, r)
	case "devices":
		h.GetStatsDevices(w, r)
	case "browsers":
		h.GetStatsBrowsers(w, r)
	case "campaigns":
		h.GetStatsCampaigns(w, r)
	case "events":
		h.GetStatsCustomEvents(w, r)
	case "outbound":
		h.GetStatsOutbound(w, r)
	default:
		writeError(w, http.StatusNotFound, "unknown stat type")
	}
}
