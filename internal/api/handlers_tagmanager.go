package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// containerCache stores generated container JS keyed by site_id
var containerCache sync.Map // map[string][]byte

// debugTokens stores short-lived preview tokens: token → {containerID, expiresAt}
var debugTokens sync.Map

type debugTokenEntry struct {
	containerID string
	expiresAt   time.Time
}

// pickResults stores element picker selections: token → suggestion JSON
var pickResults sync.Map

// SubmitPickResult receives a picker selection from the target site via sendBeacon.
// POST /api/tagmanager/pick-result (no auth — token-validated, called cross-origin)
func (h *Handlers) SubmitPickResult(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad request")
		return
	}
	var payload struct {
		Token      string          `json:"token"`
		Suggestion json.RawMessage `json:"suggestion"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Token == "" || len(payload.Suggestion) == 0 {
		writeError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	// Validate token exists
	if _, ok := debugTokens.Load(payload.Token); !ok {
		writeError(w, http.StatusForbidden, "Invalid token")
		return
	}
	pickResults.Store(payload.Token, payload.Suggestion)
	w.WriteHeader(http.StatusNoContent)
}

// GetPickResult returns the picker selection for a token (polled by the dashboard).
// GET /api/tagmanager/pick-result?token=TOKEN
func (h *Handlers) GetPickResult(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token required")
		return
	}
	if result, ok := pickResults.LoadAndDelete(token); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Write(result.(json.RawMessage))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// proxyClient is used by PickProxy to fetch target pages
var proxyClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

var reCSPMeta = regexp.MustCompile(`(?i)<meta[^>]+http-equiv\s*=\s*["']Content-Security-Policy["'][^>]*>`)
var reAbsPath = regexp.MustCompile(`((?:src|href|action)\s*=\s*)(["'])(\/[^"']*)(["'])`)

// rewriteAbsPaths rewrites absolute-path URLs (/foo) to go through the resource proxy.
// Skips protocol-relative URLs (//cdn.example.com).
func rewriteAbsPaths(html, proxyBase string) string {
	return reAbsPath.ReplaceAllStringFunc(html, func(match string) string {
		sub := reAbsPath.FindStringSubmatch(match)
		if len(sub) < 5 {
			return match
		}
		attr, quote, path, closeQuote := sub[1], sub[2], sub[3], sub[4]
		if strings.HasPrefix(path, "//") {
			return match
		}
		return attr + quote + proxyBase + strings.TrimPrefix(path, "/") + closeQuote
	})
}

// preparePickerHTML rewrites absolute-path URLs, strips CSP, injects Worker override, and injects the picker script.
func preparePickerHTML(html, proxyBase string) string {
	html = rewriteAbsPaths(html, proxyBase)
	html = reCSPMeta.ReplaceAllString(html, "")

	// No JS-level overrides needed — the SPA catch-all in router.go handles all
	// sub-resource proxying during picker sessions via the __etq_pick cookie.

	pickerScript := "<script>" + generatePickerJS() + "</script>"
	if idx := strings.LastIndex(strings.ToLower(html), "</body>"); idx != -1 {
		html = html[:idx] + pickerScript + html[idx:]
	} else {
		html += pickerScript
	}
	return html
}

// validatePickToken checks if a pick token is valid and not expired.
func validatePickToken(token string) bool {
	if entry, ok := debugTokens.Load(token); ok {
		de := entry.(debugTokenEntry)
		if time.Now().Before(de.expiresAt) {
			return true
		}
		debugTokens.Delete(token)
	}
	return false
}

// PickProxy fetches a target URL, rewrites absolute-path URLs through the resource proxy,
// injects the picker script, and serves it for the iframe.
func (h *Handlers) PickProxy(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	token := r.URL.Query().Get("token")
	if rawURL == "" || token == "" {
		writeError(w, http.StatusBadRequest, "url and token are required")
		return
	}
	if !validatePickToken(token) {
		writeError(w, http.StatusForbidden, "Invalid or expired token")
		return
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "Invalid URL")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", rawURL, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL")
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := proxyClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to fetch URL")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to read response")
		return
	}

	proxyBase := fmt.Sprintf("/_etq_res/%s/%s/%s/", token, parsed.Scheme, parsed.Host)
	html := preparePickerHTML(string(body), proxyBase)

	// Set picker cookie so the SPA catch-all can proxy sub-resource requests
	http.SetCookie(w, &http.Cookie{
		Name:     "__etq_pick",
		Value:    fmt.Sprintf("%s/%s/%s", token, parsed.Scheme, parsed.Host),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Write([]byte(html))
}

// PickProxyResource proxies sub-resources (JS, CSS, images, WASM) for the element picker.
// Non-HTML content is passed through without any modification.
func (h *Handlers) PickProxyResource(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	scheme := chi.URLParam(r, "scheme")
	host := chi.URLParam(r, "host")
	rest := chi.URLParam(r, "*")

	if !validatePickToken(token) {
		writeError(w, http.StatusForbidden, "Invalid or expired token")
		return
	}
	if scheme != "http" && scheme != "https" {
		writeError(w, http.StatusBadRequest, "Invalid scheme")
		return
	}

	targetURL := fmt.Sprintf("%s://%s/%s", scheme, host, rest)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", targetURL, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL")
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")

	resp, err := proxyClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to fetch resource")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to read response")
		return
	}

	ct := resp.Header.Get("Content-Type")

	// If HTML (in-picker navigation), rewrite URLs and inject picker script
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		proxyBase := fmt.Sprintf("/_etq_res/%s/%s/%s/", token, scheme, host)
		html := preparePickerHTML(string(body), proxyBase)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Write([]byte(html))
		return
	}

	// Non-HTML: pass through untouched
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Write(body)
}

// ServeContainerScript serves the published (or debug draft) container JS for a site
func (h *Handlers) ServeContainerScript(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteId")
	if siteID == "" {
		log.Printf("[tm] ServeContainerScript: missing siteId")
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte("/* etiquetta: container not available */"))
		return
	}

	// Element Picker mode: ?pick=<token>
	pickToken := r.URL.Query().Get("pick")
	if pickToken != "" {
		if entry, ok := debugTokens.Load(pickToken); ok {
			de := entry.(debugTokenEntry)
			if time.Now().Before(de.expiresAt) {
				w.Header().Set("Content-Type", "application/javascript")
				w.Header().Set("Cache-Control", "no-cache, no-store")
				w.Write([]byte(generatePickerJS()))
				return
			}
			debugTokens.Delete(pickToken)
		}
	}

	// Debug/Preview mode: ?debug=<token>
	debugToken := r.URL.Query().Get("debug")
	if debugToken != "" {
		if entry, ok := debugTokens.Load(debugToken); ok {
			de := entry.(debugTokenEntry)
			if time.Now().Before(de.expiresAt) {
				h.serveDebugContainer(w, de.containerID, siteID)
				return
			}
			debugTokens.Delete(debugToken)
		}
		// Invalid/expired token — fall through to normal
	}

	// Check cache first
	if cached, ok := containerCache.Load(siteID); ok {
		log.Printf("[tm] ServeContainerScript: serving cached JS for siteId=%s", siteID)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(cached.([]byte))
		return
	}

	// Look up domain by site_id
	var domainID string
	err := h.db.Conn().QueryRow("SELECT id FROM domains WHERE site_id = ? AND is_active = 1", siteID).Scan(&domainID)
	if err != nil {
		log.Printf("[tm] ServeContainerScript: domain not found for siteId=%s: %v", siteID, err)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte("/* etiquetta: container not available */"))
		return
	}

	// Find container for domain
	var containerID string
	var publishedVersion int
	err = h.db.Conn().QueryRow(`
		SELECT id, published_version FROM tm_containers WHERE domain_id = ? AND published_version > 0
	`, domainID).Scan(&containerID, &publishedVersion)
	if err != nil {
		log.Printf("[tm] ServeContainerScript: no published container for domainId=%s: %v", domainID, err)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte("/* etiquetta: container not available */"))
		return
	}

	// Get published snapshot
	var snapshotJSON string
	err = h.db.Conn().QueryRow(`
		SELECT snapshot FROM tm_snapshots WHERE container_id = ? AND version = ?
	`, containerID, publishedVersion).Scan(&snapshotJSON)
	if err != nil {
		log.Printf("[tm] ServeContainerScript: snapshot not found for container=%s version=%d: %v", containerID, publishedVersion, err)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte("/* etiquetta: container not available */"))
		return
	}

	js := generateContainerJS(snapshotJSON, false)
	jsBytes := []byte(js)

	// Cache it
	containerCache.Store(siteID, jsBytes)

	log.Printf("[tm] ServeContainerScript: generated and cached JS for siteId=%s (%d bytes)", siteID, len(jsBytes))
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(jsBytes)
}

// serveDebugContainer builds and serves the draft container with debug overlay
func (h *Handlers) serveDebugContainer(w http.ResponseWriter, containerID, siteID string) {
	snapshot, err := buildContainerSnapshot(h, containerID)
	if err != nil {
		log.Printf("[tm] serveDebugContainer: snapshot build failed: %v", err)
		w.Header().Set("Content-Type", "application/javascript")
		w.Write([]byte("/* etiquetta: debug container error */"))
		return
	}
	snapshotJSON, _ := json.Marshal(snapshot)
	js := generateContainerJS(string(snapshotJSON), true)
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Write([]byte(js))
}

// PreviewToken generates a short-lived debug token for preview mode
func (h *Handlers) PreviewToken(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")

	// Verify container exists
	var domainID string
	err := h.db.Conn().QueryRow("SELECT domain_id FROM tm_containers WHERE id = ?", containerID).Scan(&domainID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	// Get site_id for the container's domain
	var siteID string
	err = h.db.Conn().QueryRow("SELECT site_id FROM domains WHERE id = ?", domainID).Scan(&siteID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Get domain URL
	var domain string
	h.db.Conn().QueryRow("SELECT domain FROM domains WHERE id = ?", domainID).Scan(&domain)

	// Generate token: HMAC-SHA256 of containerID + timestamp
	mac := hmac.New(sha256.New, []byte(h.cfg.SecretKey))
	mac.Write([]byte(containerID + time.Now().String()))
	token := hex.EncodeToString(mac.Sum(nil))[:32]

	// Store with 30 min expiry
	debugTokens.Store(token, debugTokenEntry{
		containerID: containerID,
		expiresAt:   time.Now().Add(30 * time.Minute),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":   token,
		"site_id": siteID,
		"domain":  domain,
	})
}

// ListContainers returns all tag manager containers with domain info
func (h *Handlers) ListContainers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Conn().Query(`
		SELECT c.id, c.domain_id, c.name, c.published_version, c.draft_version,
		       c.published_at, c.published_by, c.created_at, c.updated_at,
		       d.name as domain_name, d.domain, d.site_id
		FROM tm_containers c
		JOIN domains d ON d.id = c.domain_id
		ORDER BY c.created_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var containers []map[string]interface{}
	for rows.Next() {
		var (
			id, domainID, name                string
			publishedVersion, draftVersion    int
			publishedAt                       *int64
			publishedBy                       *string
			createdAt, updatedAt              int64
			domainName, domain, siteID        string
		)
		if err := rows.Scan(&id, &domainID, &name, &publishedVersion, &draftVersion,
			&publishedAt, &publishedBy, &createdAt, &updatedAt,
			&domainName, &domain, &siteID); err != nil {
			continue
		}
		containers = append(containers, map[string]interface{}{
			"id":                id,
			"domain_id":         domainID,
			"name":              name,
			"published_version": publishedVersion,
			"draft_version":     draftVersion,
			"published_at":      publishedAt,
			"published_by":      publishedBy,
			"created_at":        createdAt,
			"updated_at":        updatedAt,
			"domain_name":       domainName,
			"domain":            domain,
			"site_id":           siteID,
		})
	}

	if containers == nil {
		containers = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, containers)
}

// CreateContainer creates a new tag manager container
func (h *Handlers) CreateContainer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DomainID string `json:"domain_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.DomainID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "domain_id and name are required")
		return
	}

	now := time.Now().UnixMilli()
	id := generateID()

	_, err := h.db.Conn().Exec(`
		INSERT INTO tm_containers (id, domain_id, name, published_version, draft_version, created_at, updated_at)
		VALUES (?, ?, ?, 0, 1, ?, ?)
	`, id, req.DomainID, req.Name, now, now)
	if err != nil {
		writeError(w, http.StatusConflict, "Container already exists for this domain")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                id,
		"domain_id":         req.DomainID,
		"name":              req.Name,
		"published_version": 0,
		"draft_version":     1,
		"created_at":        now,
		"updated_at":        now,
	})
}

// GetContainer returns a container by ID
func (h *Handlers) GetContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var (
		domainID, name                    string
		publishedVersion, draftVersion    int
		publishedAt                       *int64
		publishedBy                       *string
		createdAt, updatedAt              int64
		domainName, domain, siteID        string
	)

	err := h.db.Conn().QueryRow(`
		SELECT c.id, c.domain_id, c.name, c.published_version, c.draft_version,
		       c.published_at, c.published_by, c.created_at, c.updated_at,
		       d.name, d.domain, d.site_id
		FROM tm_containers c
		JOIN domains d ON d.id = c.domain_id
		WHERE c.id = ?
	`, id).Scan(&id, &domainID, &name, &publishedVersion, &draftVersion,
		&publishedAt, &publishedBy, &createdAt, &updatedAt,
		&domainName, &domain, &siteID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                id,
		"domain_id":         domainID,
		"name":              name,
		"published_version": publishedVersion,
		"draft_version":     draftVersion,
		"published_at":      publishedAt,
		"published_by":      publishedBy,
		"created_at":        createdAt,
		"updated_at":        updatedAt,
		"domain_name":       domainName,
		"domain":            domain,
		"site_id":           siteID,
	})
}

// UpdateContainer updates a container's name
func (h *Handlers) UpdateContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	now := time.Now().UnixMilli()
	result, err := h.db.Conn().Exec("UPDATE tm_containers SET name = ?, updated_at = ? WHERE id = ?", req.Name, now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Update failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteContainer deletes a container and all associated data
func (h *Handlers) DeleteContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.db.Conn().Exec("DELETE FROM tm_containers WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Delete failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PublishContainer publishes the current container state as a new version
func (h *Handlers) PublishContainer(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")
	log.Printf("[tm] PublishContainer: containerID=%s", containerID)

	// Get container info
	var domainID string
	var currentPublished int
	err := h.db.Conn().QueryRow("SELECT domain_id, published_version FROM tm_containers WHERE id = ?", containerID).Scan(&domainID, &currentPublished)
	if err != nil {
		log.Printf("[tm] PublishContainer: container not found: %v", err)
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	// Build snapshot: all tags, triggers, variables, and associations
	snapshot, err := buildContainerSnapshot(h, containerID)
	if err != nil {
		log.Printf("[tm] PublishContainer: failed to build snapshot: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to build snapshot")
		return
	}

	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("[tm] PublishContainer: failed to serialize snapshot: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to serialize snapshot")
		return
	}

	newVersion := currentPublished + 1
	now := time.Now().UnixMilli()
	snapshotID := generateID()

	// Get publisher info from context if available
	publishedBy := ""

	tx, err := h.db.Conn().Begin()
	if err != nil {
		log.Printf("[tm] PublishContainer: begin tx failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	// Insert snapshot
	_, err = tx.Exec(`
		INSERT INTO tm_snapshots (id, container_id, version, snapshot, published_by, published_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, snapshotID, containerID, newVersion, string(snapshotJSON), publishedBy, now)
	if err != nil {
		log.Printf("[tm] PublishContainer: insert snapshot failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to save snapshot")
		return
	}

	// Update container
	_, err = tx.Exec(`
		UPDATE tm_containers SET published_version = ?, published_at = ?, published_by = ?, updated_at = ?
		WHERE id = ?
	`, newVersion, now, publishedBy, now, containerID)
	if err != nil {
		log.Printf("[tm] PublishContainer: update container failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to update container")
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[tm] PublishContainer: commit failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Commit failed")
		return
	}

	// Clear cache for this domain's site_id
	var siteID string
	if err := h.db.Conn().QueryRow("SELECT site_id FROM domains WHERE id = ?", domainID).Scan(&siteID); err == nil {
		containerCache.Delete(siteID)
	}

	log.Printf("[tm] PublishContainer: published version %d for container %s", newVersion, containerID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":      newVersion,
		"published_at": now,
	})
}

// GetContainerVersions lists all published snapshots for a container
func (h *Handlers) GetContainerVersions(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")

	rows, err := h.db.Conn().Query(`
		SELECT id, container_id, version, published_by, published_at
		FROM tm_snapshots
		WHERE container_id = ?
		ORDER BY version DESC
	`, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var versions []map[string]interface{}
	for rows.Next() {
		var (
			id, cID   string
			version   int
			pubBy     *string
			pubAt     int64
		)
		if err := rows.Scan(&id, &cID, &version, &pubBy, &pubAt); err != nil {
			continue
		}
		versions = append(versions, map[string]interface{}{
			"id":           id,
			"container_id": cID,
			"version":      version,
			"published_by": pubBy,
			"published_at": pubAt,
		})
	}

	if versions == nil {
		versions = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, versions)
}

// RollbackContainer sets a previous snapshot as the new published version
func (h *Handlers) RollbackContainer(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")
	versionStr := chi.URLParam(r, "version")
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid version")
		return
	}

	// Verify the snapshot exists
	var snapshotJSON string
	err = h.db.Conn().QueryRow("SELECT snapshot FROM tm_snapshots WHERE container_id = ? AND version = ?", containerID, version).Scan(&snapshotJSON)
	if err != nil {
		writeError(w, http.StatusNotFound, "Snapshot not found")
		return
	}

	// Get domain_id for cache clearing
	var domainID string
	h.db.Conn().QueryRow("SELECT domain_id FROM tm_containers WHERE id = ?", containerID).Scan(&domainID)

	now := time.Now().UnixMilli()
	_, err = h.db.Conn().Exec(`
		UPDATE tm_containers SET published_version = ?, published_at = ?, updated_at = ? WHERE id = ?
	`, version, now, now, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Rollback failed")
		return
	}

	// Clear cache
	var siteID string
	if err := h.db.Conn().QueryRow("SELECT site_id FROM domains WHERE id = ?", domainID).Scan(&siteID); err == nil {
		containerCache.Delete(siteID)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":      version,
		"published_at": now,
	})
}

// ========== Container Import/Export ==========

// ExportContainer returns the full container state as a JSON download
func (h *Handlers) ExportContainer(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")

	// Get container metadata
	var name, domainID string
	var publishedVersion, draftVersion int
	err := h.db.Conn().QueryRow(`
		SELECT name, domain_id, published_version, draft_version FROM tm_containers WHERE id = ?
	`, containerID).Scan(&name, &domainID, &publishedVersion, &draftVersion)
	if err != nil {
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	// Build full snapshot of current draft state
	snapshot, err := buildContainerSnapshot(h, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to build export")
		return
	}

	export := map[string]interface{}{
		"format":      "etiquetta_container_v1",
		"exported_at": time.Now().UnixMilli(),
		"container": map[string]interface{}{
			"name":              name,
			"published_version": publishedVersion,
			"draft_version":     draftVersion,
		},
		"data": snapshot,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-export.json"`, name))
	json.NewEncoder(w).Encode(export)
}

// ImportContainer replaces the container's draft with imported data
func (h *Handlers) ImportContainer(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")

	// Verify container exists
	var existingID string
	err := h.db.Conn().QueryRow("SELECT id FROM tm_containers WHERE id = ?", containerID).Scan(&existingID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	// Parse the import payload
	var importData struct {
		Format string `json:"format"`
		Data   struct {
			Tags      []json.RawMessage `json:"tags"`
			Triggers  []json.RawMessage `json:"triggers"`
			Variables []json.RawMessage `json:"variables"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&importData); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if importData.Format != "etiquetta_container_v1" {
		writeError(w, http.StatusBadRequest, "Unsupported import format. Expected etiquetta_container_v1")
		return
	}

	now := time.Now().UnixMilli()

	tx, err := h.db.Conn().Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	// Delete existing entities
	tx.Exec("DELETE FROM tm_tag_triggers WHERE tag_id IN (SELECT id FROM tm_tags WHERE container_id = ?)", containerID)
	tx.Exec("DELETE FROM tm_tags WHERE container_id = ?", containerID)
	tx.Exec("DELETE FROM tm_triggers WHERE container_id = ?", containerID)
	tx.Exec("DELETE FROM tm_variables WHERE container_id = ?", containerID)

	// ID remapping: old ID → new ID
	idMap := make(map[string]string)

	// Import triggers first (tags reference them)
	for _, raw := range importData.Data.Triggers {
		var t struct {
			ID          string          `json:"id"`
			Name        string          `json:"name"`
			TriggerType string          `json:"trigger_type"`
			Config      json.RawMessage `json:"config"`
		}
		if json.Unmarshal(raw, &t) != nil {
			continue
		}
		newID := generateID()
		idMap[t.ID] = newID
		configStr := "{}"
		if t.Config != nil {
			configStr = string(t.Config)
		}
		tx.Exec(`INSERT INTO tm_triggers (id, container_id, name, trigger_type, config, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, newID, containerID, t.Name, t.TriggerType, configStr, now, now)
	}

	// Import variables
	for _, raw := range importData.Data.Variables {
		var v struct {
			ID           string          `json:"id"`
			Name         string          `json:"name"`
			VariableType string          `json:"variable_type"`
			Config       json.RawMessage `json:"config"`
		}
		if json.Unmarshal(raw, &v) != nil {
			continue
		}
		newID := generateID()
		configStr := "{}"
		if v.Config != nil {
			configStr = string(v.Config)
		}
		tx.Exec(`INSERT INTO tm_variables (id, container_id, name, variable_type, config, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, newID, containerID, v.Name, v.VariableType, configStr, now, now)
	}

	// Import tags with trigger associations
	for _, raw := range importData.Data.Tags {
		var t struct {
			ID                  string          `json:"id"`
			Name                string          `json:"name"`
			TagType             string          `json:"tag_type"`
			Config              json.RawMessage `json:"config"`
			ConsentCategory     string          `json:"consent_category"`
			Priority            int             `json:"priority"`
			TriggerIDs          []string        `json:"trigger_ids"`
			ExceptionTriggerIDs []string        `json:"exception_trigger_ids"`
		}
		if json.Unmarshal(raw, &t) != nil {
			continue
		}
		newTagID := generateID()
		configStr := "{}"
		if t.Config != nil {
			configStr = string(t.Config)
		}
		if t.ConsentCategory == "" {
			t.ConsentCategory = "marketing"
		}
		tx.Exec(`INSERT INTO tm_tags (id, container_id, name, tag_type, config, consent_category, priority, is_enabled, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1, 1, ?, ?)`, newTagID, containerID, t.Name, t.TagType, configStr, t.ConsentCategory, t.Priority, now, now)

		// Firing triggers
		for _, oldTriggerID := range t.TriggerIDs {
			if newTriggerID, ok := idMap[oldTriggerID]; ok {
				tx.Exec("INSERT INTO tm_tag_triggers (tag_id, trigger_id, is_exception) VALUES (?, ?, 0)", newTagID, newTriggerID)
			}
		}
		// Exception triggers
		for _, oldTriggerID := range t.ExceptionTriggerIDs {
			if newTriggerID, ok := idMap[oldTriggerID]; ok {
				tx.Exec("INSERT INTO tm_tag_triggers (tag_id, trigger_id, is_exception) VALUES (?, ?, 1)", newTagID, newTriggerID)
			}
		}
	}

	// Bump draft version
	tx.Exec("UPDATE tm_containers SET draft_version = draft_version + 1, updated_at = ? WHERE id = ?", now, containerID)

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Import failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"imported": map[string]int{
			"tags":      len(importData.Data.Tags),
			"triggers":  len(importData.Data.Triggers),
			"variables": len(importData.Data.Variables),
		},
	})
}

// ========== Tag CRUD ==========

// ListTags lists all tags for a container, including trigger associations
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	rows, err := h.db.Conn().Query(`
		SELECT id, container_id, name, tag_type, config, consent_category, priority, is_enabled, version, created_at, updated_at
		FROM tm_tags WHERE container_id = ?
		ORDER BY priority DESC, name ASC
	`, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	type tagRow struct {
		id, cID, name, tagType, config, consentCat string
		priority, version                          int
		isEnabled                                  bool
		createdAt, updatedAt                       int64
	}
	var tagRows []tagRow
	for rows.Next() {
		var t tagRow
		if err := rows.Scan(&t.id, &t.cID, &t.name, &t.tagType, &t.config, &t.consentCat, &t.priority, &t.isEnabled, &t.version, &t.createdAt, &t.updatedAt); err != nil {
			continue
		}
		tagRows = append(tagRows, t)
	}
	rows.Close()

	// Batch-fetch all trigger associations for this container
	triggerMaps := getContainerTagTriggerMaps(h, containerID)

	tags := make([]map[string]interface{}, 0, len(tagRows))
	for _, t := range tagRows {
		firingIDs := triggerMaps.firing[t.id]
		if firingIDs == nil {
			firingIDs = []string{}
		}
		exceptionIDs := triggerMaps.exception[t.id]
		if exceptionIDs == nil {
			exceptionIDs = []string{}
		}
		tags = append(tags, map[string]interface{}{
			"id":                    t.id,
			"container_id":          t.cID,
			"name":                  t.name,
			"tag_type":              t.tagType,
			"config":                json.RawMessage(t.config),
			"consent_category":      t.consentCat,
			"priority":              t.priority,
			"is_enabled":            t.isEnabled,
			"version":               t.version,
			"trigger_ids":           firingIDs,
			"exception_trigger_ids": exceptionIDs,
			"created_at":            t.createdAt,
			"updated_at":            t.updatedAt,
		})
	}

	writeJSON(w, http.StatusOK, tags)
}

// CreateTag creates a new tag in a container
func (h *Handlers) CreateTag(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	var req struct {
		Name                string          `json:"name"`
		TagType             string          `json:"tag_type"`
		Config              json.RawMessage `json:"config"`
		ConsentCategory     string          `json:"consent_category"`
		Priority            int             `json:"priority"`
		TriggerIDs          []string        `json:"trigger_ids"`
		ExceptionTriggerIDs []string        `json:"exception_trigger_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Name == "" || req.TagType == "" {
		writeError(w, http.StatusBadRequest, "name and tag_type are required")
		return
	}
	if req.ConsentCategory == "" {
		req.ConsentCategory = "marketing"
	}
	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	now := time.Now().UnixMilli()
	id := generateID()

	tx, err := h.db.Conn().Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO tm_tags (id, container_id, name, tag_type, config, consent_category, priority, is_enabled, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, 1, ?, ?)
	`, id, containerID, req.Name, req.TagType, string(req.Config), req.ConsentCategory, req.Priority, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create tag")
		return
	}

	// Insert firing trigger associations
	for _, triggerID := range req.TriggerIDs {
		_, err = tx.Exec("INSERT INTO tm_tag_triggers (tag_id, trigger_id, is_exception) VALUES (?, ?, 0)", id, triggerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to associate triggers")
			return
		}
	}

	// Insert exception trigger associations
	for _, triggerID := range req.ExceptionTriggerIDs {
		_, err = tx.Exec("INSERT INTO tm_tag_triggers (tag_id, trigger_id, is_exception) VALUES (?, ?, 1)", id, triggerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to associate exception triggers")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                    id,
		"container_id":          containerID,
		"name":                  req.Name,
		"tag_type":              req.TagType,
		"config":                req.Config,
		"consent_category":      req.ConsentCategory,
		"priority":              req.Priority,
		"is_enabled":            true,
		"version":               1,
		"trigger_ids":           req.TriggerIDs,
		"exception_trigger_ids": req.ExceptionTriggerIDs,
		"created_at":            now,
		"updated_at":            now,
	})
}

// GetTag returns a single tag by ID
func (h *Handlers) GetTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "id")

	var (
		id, cID, name, tagType, config, consentCat string
		priority, version                          int
		isEnabled                                  bool
		createdAt, updatedAt                       int64
	)
	err := h.db.Conn().QueryRow(`
		SELECT id, container_id, name, tag_type, config, consent_category, priority, is_enabled, version, created_at, updated_at
		FROM tm_tags WHERE id = ?
	`, tagID).Scan(&id, &cID, &name, &tagType, &config, &consentCat, &priority, &isEnabled, &version, &createdAt, &updatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "Tag not found")
		return
	}

	firingIDs, exceptionIDs := getTagTriggerIDsSplit(h, id)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                    id,
		"container_id":          cID,
		"name":                  name,
		"tag_type":              tagType,
		"config":                json.RawMessage(config),
		"consent_category":      consentCat,
		"priority":              priority,
		"is_enabled":            isEnabled,
		"version":               version,
		"trigger_ids":           firingIDs,
		"exception_trigger_ids": exceptionIDs,
		"created_at":            createdAt,
		"updated_at":            updatedAt,
	})
}

// UpdateTag updates a tag and its trigger associations
func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "id")

	var req struct {
		Name                string          `json:"name"`
		TagType             string          `json:"tag_type"`
		Config              json.RawMessage `json:"config"`
		ConsentCategory     string          `json:"consent_category"`
		Priority            int             `json:"priority"`
		IsEnabled           *bool           `json:"is_enabled"`
		TriggerIDs          []string        `json:"trigger_ids"`
		ExceptionTriggerIDs []string        `json:"exception_trigger_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	now := time.Now().UnixMilli()

	tx, err := h.db.Conn().Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	isEnabled := 1
	if req.IsEnabled != nil && !*req.IsEnabled {
		isEnabled = 0
	}

	configStr := "{}"
	if req.Config != nil {
		configStr = string(req.Config)
	}

	result, err := tx.Exec(`
		UPDATE tm_tags SET name = ?, tag_type = ?, config = ?, consent_category = ?, priority = ?, is_enabled = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.TagType, configStr, req.ConsentCategory, req.Priority, isEnabled, now, tagID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Update failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Tag not found")
		return
	}

	// Replace all trigger associations (firing + exception)
	tx.Exec("DELETE FROM tm_tag_triggers WHERE tag_id = ?", tagID)
	for _, triggerID := range req.TriggerIDs {
		tx.Exec("INSERT INTO tm_tag_triggers (tag_id, trigger_id, is_exception) VALUES (?, ?, 0)", tagID, triggerID)
	}
	for _, triggerID := range req.ExceptionTriggerIDs {
		tx.Exec("INSERT INTO tm_tag_triggers (tag_id, trigger_id, is_exception) VALUES (?, ?, 1)", tagID, triggerID)
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Commit failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteTag deletes a tag
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "id")

	result, err := h.db.Conn().Exec("DELETE FROM tm_tags WHERE id = ?", tagID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Delete failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Tag not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ========== Trigger CRUD ==========

// ListTriggers lists all triggers for a container
func (h *Handlers) ListTriggers(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	rows, err := h.db.Conn().Query(`
		SELECT id, container_id, name, trigger_type, config, created_at, updated_at
		FROM tm_triggers WHERE container_id = ?
		ORDER BY name ASC
	`, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var triggers []map[string]interface{}
	for rows.Next() {
		var (
			id, cID, name, triggerType, config string
			createdAt, updatedAt               int64
		)
		if err := rows.Scan(&id, &cID, &name, &triggerType, &config, &createdAt, &updatedAt); err != nil {
			continue
		}
		triggers = append(triggers, map[string]interface{}{
			"id":           id,
			"container_id": cID,
			"name":         name,
			"trigger_type": triggerType,
			"config":       json.RawMessage(config),
			"created_at":   createdAt,
			"updated_at":   updatedAt,
		})
	}

	if triggers == nil {
		triggers = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, triggers)
}

// CreateTrigger creates a new trigger in a container
func (h *Handlers) CreateTrigger(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	var req struct {
		Name        string          `json:"name"`
		TriggerType string          `json:"trigger_type"`
		Config      json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Name == "" || req.TriggerType == "" {
		writeError(w, http.StatusBadRequest, "name and trigger_type are required")
		return
	}
	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	now := time.Now().UnixMilli()
	id := generateID()

	_, err := h.db.Conn().Exec(`
		INSERT INTO tm_triggers (id, container_id, name, trigger_type, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, containerID, req.Name, req.TriggerType, string(req.Config), now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create trigger")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":           id,
		"container_id": containerID,
		"name":         req.Name,
		"trigger_type": req.TriggerType,
		"config":       req.Config,
		"created_at":   now,
		"updated_at":   now,
	})
}

// UpdateTrigger updates a trigger
func (h *Handlers) UpdateTrigger(w http.ResponseWriter, r *http.Request) {
	triggerID := chi.URLParam(r, "id")

	var req struct {
		Name        string          `json:"name"`
		TriggerType string          `json:"trigger_type"`
		Config      json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	configStr := "{}"
	if req.Config != nil {
		configStr = string(req.Config)
	}

	now := time.Now().UnixMilli()
	result, err := h.db.Conn().Exec(`
		UPDATE tm_triggers SET name = ?, trigger_type = ?, config = ?, updated_at = ? WHERE id = ?
	`, req.Name, req.TriggerType, configStr, now, triggerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Update failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Trigger not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteTrigger deletes a trigger
func (h *Handlers) DeleteTrigger(w http.ResponseWriter, r *http.Request) {
	triggerID := chi.URLParam(r, "id")

	result, err := h.db.Conn().Exec("DELETE FROM tm_triggers WHERE id = ?", triggerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Delete failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Trigger not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ========== Variable CRUD ==========

// ListVariables lists all variables for a container
func (h *Handlers) ListVariables(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	rows, err := h.db.Conn().Query(`
		SELECT id, container_id, name, variable_type, config, created_at, updated_at
		FROM tm_variables WHERE container_id = ?
		ORDER BY name ASC
	`, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var variables []map[string]interface{}
	for rows.Next() {
		var (
			id, cID, name, varType, config string
			createdAt, updatedAt           int64
		)
		if err := rows.Scan(&id, &cID, &name, &varType, &config, &createdAt, &updatedAt); err != nil {
			continue
		}
		variables = append(variables, map[string]interface{}{
			"id":            id,
			"container_id":  cID,
			"name":          name,
			"variable_type": varType,
			"config":        json.RawMessage(config),
			"created_at":    createdAt,
			"updated_at":    updatedAt,
		})
	}

	if variables == nil {
		variables = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, variables)
}

// CreateVariable creates a new variable in a container
func (h *Handlers) CreateVariable(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	var req struct {
		Name         string          `json:"name"`
		VariableType string          `json:"variable_type"`
		Config       json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Name == "" || req.VariableType == "" {
		writeError(w, http.StatusBadRequest, "name and variable_type are required")
		return
	}
	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	now := time.Now().UnixMilli()
	id := generateID()

	_, err := h.db.Conn().Exec(`
		INSERT INTO tm_variables (id, container_id, name, variable_type, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, containerID, req.Name, req.VariableType, string(req.Config), now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create variable")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            id,
		"container_id":  containerID,
		"name":          req.Name,
		"variable_type": req.VariableType,
		"config":        req.Config,
		"created_at":    now,
		"updated_at":    now,
	})
}

// UpdateVariable updates a variable
func (h *Handlers) UpdateVariable(w http.ResponseWriter, r *http.Request) {
	varID := chi.URLParam(r, "id")

	var req struct {
		Name         string          `json:"name"`
		VariableType string          `json:"variable_type"`
		Config       json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	configStr := "{}"
	if req.Config != nil {
		configStr = string(req.Config)
	}

	now := time.Now().UnixMilli()
	result, err := h.db.Conn().Exec(`
		UPDATE tm_variables SET name = ?, variable_type = ?, config = ?, updated_at = ? WHERE id = ?
	`, req.Name, req.VariableType, configStr, now, varID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Update failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Variable not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteVariable deletes a variable
func (h *Handlers) DeleteVariable(w http.ResponseWriter, r *http.Request) {
	varID := chi.URLParam(r, "id")

	result, err := h.db.Conn().Exec("DELETE FROM tm_variables WHERE id = ?", varID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Delete failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Variable not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ========== Helpers ==========

// tagTriggerMaps holds firing and exception trigger IDs per tag
type tagTriggerMaps struct {
	firing    map[string][]string
	exception map[string][]string
}

// getContainerTagTriggerMaps batch-fetches all tag→trigger associations for a container,
// split into firing and exception maps.
func getContainerTagTriggerMaps(h *Handlers, containerID string) tagTriggerMaps {
	rows, err := h.db.Conn().Query(`
		SELECT tt.tag_id, tt.trigger_id, tt.is_exception
		FROM tm_tag_triggers tt
		JOIN tm_tags t ON t.id = tt.tag_id
		WHERE t.container_id = ?
	`, containerID)
	if err != nil {
		return tagTriggerMaps{firing: map[string][]string{}, exception: map[string][]string{}}
	}
	defer rows.Close()

	m := tagTriggerMaps{
		firing:    make(map[string][]string),
		exception: make(map[string][]string),
	}
	for rows.Next() {
		var tagID, triggerID string
		var isException bool
		if rows.Scan(&tagID, &triggerID, &isException) == nil {
			if isException {
				m.exception[tagID] = append(m.exception[tagID], triggerID)
			} else {
				m.firing[tagID] = append(m.firing[tagID], triggerID)
			}
		}
	}
	return m
}

// getTagTriggerIDsSplit returns firing and exception trigger IDs for a single tag.
func getTagTriggerIDsSplit(h *Handlers, tagID string) (firing []string, exception []string) {
	rows, err := h.db.Conn().Query("SELECT trigger_id, is_exception FROM tm_tag_triggers WHERE tag_id = ?", tagID)
	if err != nil {
		return []string{}, []string{}
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var isExc bool
		if rows.Scan(&id, &isExc) == nil {
			if isExc {
				exception = append(exception, id)
			} else {
				firing = append(firing, id)
			}
		}
	}
	if firing == nil {
		firing = []string{}
	}
	if exception == nil {
		exception = []string{}
	}
	return
}

// buildContainerSnapshot builds a full snapshot of the container state
func buildContainerSnapshot(h *Handlers, containerID string) (map[string]interface{}, error) {
	// Tags
	tagRows, err := h.db.Conn().Query(`
		SELECT id, name, tag_type, config, consent_category, priority, is_enabled
		FROM tm_tags WHERE container_id = ? AND is_enabled = 1
	`, containerID)
	if err != nil {
		return nil, err
	}

	type snapTag struct {
		id, name, tagType, config, consentCat string
		priority                              int
	}
	var rawTags []snapTag
	for tagRows.Next() {
		var t snapTag
		var isEnabled bool
		if err := tagRows.Scan(&t.id, &t.name, &t.tagType, &t.config, &t.consentCat, &t.priority, &isEnabled); err != nil {
			continue
		}
		rawTags = append(rawTags, t)
	}
	tagRows.Close()

	// Batch-fetch trigger associations (split firing/exception)
	triggerMaps := getContainerTagTriggerMaps(h, containerID)

	tags := make([]map[string]interface{}, 0, len(rawTags))
	for _, t := range rawTags {
		firingIDs := triggerMaps.firing[t.id]
		if firingIDs == nil {
			firingIDs = []string{}
		}
		exceptionIDs := triggerMaps.exception[t.id]
		if exceptionIDs == nil {
			exceptionIDs = []string{}
		}
		tags = append(tags, map[string]interface{}{
			"id":                    t.id,
			"name":                  t.name,
			"tag_type":              t.tagType,
			"config":                json.RawMessage(t.config),
			"consent_category":      t.consentCat,
			"priority":              t.priority,
			"trigger_ids":           firingIDs,
			"exception_trigger_ids": exceptionIDs,
		})
	}

	// Triggers
	triggerRows, err := h.db.Conn().Query(`
		SELECT id, name, trigger_type, config FROM tm_triggers WHERE container_id = ?
	`, containerID)
	if err != nil {
		return nil, err
	}
	defer triggerRows.Close()

	var triggers []map[string]interface{}
	for triggerRows.Next() {
		var id, name, triggerType, config string
		if err := triggerRows.Scan(&id, &name, &triggerType, &config); err != nil {
			continue
		}
		triggers = append(triggers, map[string]interface{}{
			"id":           id,
			"name":         name,
			"trigger_type": triggerType,
			"config":       json.RawMessage(config),
		})
	}
	if triggers == nil {
		triggers = []map[string]interface{}{}
	}

	// Variables
	varRows, err := h.db.Conn().Query(`
		SELECT id, name, variable_type, config FROM tm_variables WHERE container_id = ?
	`, containerID)
	if err != nil {
		return nil, err
	}
	defer varRows.Close()

	var variables []map[string]interface{}
	for varRows.Next() {
		var id, name, varType, config string
		if err := varRows.Scan(&id, &name, &varType, &config); err != nil {
			continue
		}
		variables = append(variables, map[string]interface{}{
			"id":            id,
			"name":          name,
			"variable_type": varType,
			"config":        json.RawMessage(config),
		})
	}
	if variables == nil {
		variables = []map[string]interface{}{}
	}

	return map[string]interface{}{
		"tags":      tags,
		"triggers":  triggers,
		"variables": variables,
	}, nil
}

// generateContainerJS creates a self-executing JS string from a snapshot JSON.
// If debug is true, it includes a full debug console with timeline, tag summary,
// variable inspector, and data layer viewer.
func generateContainerJS(snapshotJSON string, debug bool) string {
	dbg := func(line string) string {
		if debug {
			return line
		}
		return ""
	}
	debugPrefix := ""
	debugInit := ""
	debugSuffix := ""
	if debug {
		debugPrefix = "/* ETIQUETTA DEBUG/PREVIEW MODE */\n"
		debugInit = generateDebugJS()
		debugSuffix = `_renderConsole();`
	}

	return fmt.Sprintf(`%s(function(){
"use strict";
var C=%s;
var _cl=[];
%swindow.etiquettaDataLayer=window.etiquettaDataLayer||[];
var consent=window.__ETIQUETTA_CONSENT__||null;
function hasConsent(cat){return !consent||(consent[cat]===true);}
function resolveVar(v){
switch(v.variable_type){
case"data_layer":var dl=window.etiquettaDataLayer;var k=v.config.variable_name||"";for(var i=dl.length-1;i>=0;i--){if(dl[i]&&dl[i][k]!==undefined)return dl[i][k];}return v.config.default_value||"";
case"url_param":return new URL(location.href).searchParams.get(v.config.param_name||"")||"";
case"cookie":var cn=v.config.cookie_name||"";var m=document.cookie.match(new RegExp("(?:^|; )"+cn.replace(/[.*+?^${}()|[\]\\]/g,"\\$&")+"=([^;]*)"));return m?decodeURIComponent(m[1]):"";
case"dom_element":var el=document.querySelector(v.config.selector||"");return el?(v.config.attribute?el.getAttribute(v.config.attribute):el.textContent)||"":"";
case"js_variable":try{return(new Function("return "+v.config.variable_name))();}catch(e){return"";}
case"constant":return v.config.value||"";
case"referrer":return document.referrer;
case"page_url":return location.href;
case"page_path":return location.pathname;
case"page_hostname":return location.hostname;
default:return"";}
}
function resolveCondVar(name){
var v=C.variables.find(function(v){return v.name===name;});
if(v)return String(resolveVar(v));
switch(name){
case"page_path":return location.pathname;
case"page_url":return location.href;
case"page_hostname":return location.hostname;
case"referrer":return document.referrer;
default:return"";}
}
function evalOp(val,op,expected){
val=String(val||"");expected=String(expected||"");
switch(op){
case"equals":return val===expected;
case"not_equals":return val!==expected;
case"contains":return val.indexOf(expected)>=0;
case"not_contains":return val.indexOf(expected)<0;
case"starts_with":return val.indexOf(expected)===0;
case"ends_with":return val.length>=expected.length&&val.slice(-expected.length)===expected;
case"matches_regex":try{return new RegExp(expected).test(val);}catch(e){return false;}
default:return true;}
}
function interpolate(str){
if(typeof str!=="string")return str;
return str.replace(/\{\{(.+?)\}\}/g,function(m,name){
name=name.trim();
var v=C.variables.find(function(v){return v.name===name;});
return v?String(resolveVar(v)):m;
});
}
function interpolateConfig(cfg){
var r={};for(var k in cfg){if(cfg.hasOwnProperty(k)){r[k]=interpolate(cfg[k]);}}return r;
}
function loadScript(src,cb){var s=document.createElement("script");s.src=src;s.async=true;if(cb)s.onload=cb;document.head.appendChild(s);}
function fireTag(tag){
if(!hasConsent(tag.consent_category)){%sreturn;}
var cfg=interpolateConfig(tag.config);%s
switch(tag.tag_type){
case"custom_html":var d=document.createElement("div");d.innerHTML=cfg.html||"";var scripts=d.getElementsByTagName("script");for(var i=0;i<scripts.length;i++){var s=document.createElement("script");if(scripts[i].src){s.src=scripts[i].src;}else{s.textContent=scripts[i].textContent;}document.head.appendChild(s);}break;
case"ga4":if(!window.gtag){window.dataLayer=window.dataLayer||[];window.gtag=function(){window.dataLayer.push(arguments);};window.gtag("js",new Date());loadScript("https://www.googletagmanager.com/gtag/js?id="+cfg.measurement_id);}window.gtag("config",cfg.measurement_id);break;
case"meta_pixel":if(!window.fbq){var f=function(){f.callMethod?f.callMethod.apply(f,arguments):f.queue.push(arguments);};window.fbq=f;f.push=f;f.loaded=true;f.version="2.0";f.queue=[];loadScript("https://connect.facebook.net/en_US/fbevents.js");window.fbq("init",cfg.pixel_id);}window.fbq("track","PageView");break;
case"google_ads":if(!window.gtag){window.dataLayer=window.dataLayer||[];window.gtag=function(){window.dataLayer.push(arguments);};window.gtag("js",new Date());loadScript("https://www.googletagmanager.com/gtag/js?id="+cfg.conversion_id);}window.gtag("config",cfg.conversion_id);if(cfg.conversion_label){window.gtag("event","conversion",{send_to:cfg.conversion_id+"/"+cfg.conversion_label});}break;
case"linkedin":if(!window._linkedin_partner_id){window._linkedin_partner_id=cfg.partner_id;window._linkedin_data_partner_ids=window._linkedin_data_partner_ids||[];window._linkedin_data_partner_ids.push(cfg.partner_id);loadScript("https://snap.licdn.com/li.lms-analytics/insight.min.js");}break;
case"tiktok":if(!window.ttq){var tt=function(){tt.methods.forEach(function(m){tt[m]=function(){var a=Array.prototype.slice.call(arguments);a.unshift(m);tt.queue.push(a);};});};tt.methods=["page","track","identify","instances","debug","on","off","once","ready","alias","group","enableCookie","disableCookie"];tt.queue=[];tt();window.ttq=tt;loadScript("https://analytics.tiktok.com/i18n/pixel/events.js");window.ttq.load(cfg.pixel_id);window.ttq.page();}break;
case"etiquetta_event":if(window.etiquetta&&window.etiquetta.track){var props={};try{props=JSON.parse(cfg.event_props||"{}");}catch(e){}window.etiquetta.track(cfg.event_name||"event",props);}break;
}
}
function matchElement(target,cfg){
if(!target||!cfg.selector)return false;
var mt=cfg.match_type||"css";
if(mt==="text"){
var txt=cfg.selector||"";var mode=cfg.text_match_mode||"contains";
var el=target.closest?target:target.parentElement;
while(el){
var elText=(el.textContent||"").trim();
if(mode==="exact"?elText===txt:elText.indexOf(txt)>=0)return true;
el=el.parentElement;
}
return false;
}
if(mt==="id"){return target.closest&&!!target.closest(cfg.selector);}
if(mt==="data_attr"){return target.closest&&!!target.closest(cfg.selector);}
if(mt==="link_url"){return target.closest&&!!target.closest(cfg.selector);}
return target.closest&&!!target.closest(cfg.selector);
}
function evalTrigger(trigger,evType,evData){
var t=trigger.trigger_type,cfg=trigger.config||{};
var baseMatch=false;
if(t==="page_load"||t==="dom_ready")baseMatch=!evType||evType===t;
else if(t==="click_all"&&evType==="click")baseMatch=true;
else if(t==="click_specific"&&evType==="click"){if(!cfg.selector)baseMatch=false;else baseMatch=evData&&evData.target&&matchElement(evData.target,cfg);}
else if(t==="custom_event"&&evType==="custom_event"&&evData===cfg.event_name)baseMatch=true;
else if(t==="scroll_depth"&&evType==="scroll_depth")baseMatch=true;
else if(t==="timer"&&evType==="timer")baseMatch=true;
else if(t==="history_change"&&evType==="history_change")baseMatch=true;
else if(t==="form_submit"&&evType==="form_submit"){if(!cfg.selector)baseMatch=true;else baseMatch=evData&&evData.target&&matchElement(evData.target,cfg);}
else if(t==="element_visibility"&&evType==="element_visibility")baseMatch=true;
else if(!evType&&(t==="page_load"||t==="dom_ready"))baseMatch=true;
if(!baseMatch)return false;
var conditions=cfg.conditions;
if(!conditions||!conditions.length)return true;
return conditions.every(function(cond){
var val=resolveCondVar(cond.variable);
var result=evalOp(val,cond.operator,cond.value);%s
return result;
});
}
function init(){
_cl.forEach(function(fn){fn();});_cl=[];
C.tags.sort(function(a,b){return(b.priority||0)-(a.priority||0);});
C.tags.forEach(function(tag){
var firingTriggers=tag.trigger_ids.map(function(tid){return C.triggers.find(function(t){return t.id===tid;});}).filter(Boolean);
var exceptionTriggers=(tag.exception_trigger_ids||[]).map(function(tid){return C.triggers.find(function(t){return t.id===tid;});}).filter(Boolean);
function isBlocked(evType,evData){
for(var i=0;i<exceptionTriggers.length;i++){if(evalTrigger(exceptionTriggers[i],evType,evData)){%sreturn true;}}
return false;
}
var immediate=firingTriggers.length===0||firingTriggers.some(function(tr){return evalTrigger(tr);});
if(immediate&&!isBlocked()){%sfireTag(tag);}
firingTriggers.forEach(function(tr){
var t=tr.trigger_type,cfg=tr.config||{};
if(t==="click_all"||t==="click_specific"){var h=function(e){if(evalTrigger(tr,"click",{target:e.target})&&!isBlocked("click",{target:e.target})){%sfireTag(tag);}};document.addEventListener("click",h);_cl.push(function(){document.removeEventListener("click",h);});}
if(t==="custom_event"&&cfg.event_name){var ce=function(){if(!isBlocked("custom_event",cfg.event_name)){%sfireTag(tag);}};window.addEventListener(cfg.event_name,ce);_cl.push(function(){window.removeEventListener(cfg.event_name,ce);});}
if(t==="scroll_depth"){var pct=parseInt(cfg.percentage,10)||50;var fired=false;var sh=function(){if(fired)return;var scrollTop=window.pageYOffset||document.documentElement.scrollTop;var docHeight=Math.max(document.body.scrollHeight,document.documentElement.scrollHeight)-window.innerHeight;if(docHeight>0&&(scrollTop/docHeight)*100>=pct){fired=true;if(!isBlocked("scroll_depth")){%sfireTag(tag);}}};window.addEventListener("scroll",sh,{passive:true});_cl.push(function(){window.removeEventListener("scroll",sh);});}
if(t==="timer"){var interval=parseInt(cfg.interval_ms,10)||5000;var limit=parseInt(cfg.limit,10)||0;var count=0;var tid=setInterval(function(){count++;if(!isBlocked("timer")){%sfireTag(tag);}if(limit>0&&count>=limit)clearInterval(tid);},interval);_cl.push(function(){clearInterval(tid);});}
if(t==="history_change"){var hp=function(){if(!isBlocked("history_change")){%sfireTag(tag);}};window.addEventListener("popstate",hp);var origPush=history.pushState;var origReplace=history.replaceState;history.pushState=function(){origPush.apply(history,arguments);hp();};history.replaceState=function(){origReplace.apply(history,arguments);hp();};_cl.push(function(){window.removeEventListener("popstate",hp);history.pushState=origPush;history.replaceState=origReplace;});}
if(t==="form_submit"){var fh=function(e){if(evalTrigger(tr,"form_submit",{target:e.target})&&!isBlocked("form_submit",{target:e.target})){%sfireTag(tag);}};document.addEventListener("submit",fh);_cl.push(function(){document.removeEventListener("submit",fh);});}
if(t==="element_visibility"&&cfg.selector){var threshold=parseInt(cfg.threshold,10)||50;var fireOnce=cfg.fire_once!=="false";var vFired=false;try{var vEls=document.querySelectorAll(cfg.selector);if(vEls.length>0){var vObs=new IntersectionObserver(function(entries){entries.forEach(function(entry){if(entry.isIntersecting&&!vFired){if(!isBlocked("element_visibility")){%sfireTag(tag);}if(fireOnce){vFired=true;vObs.disconnect();}}});},{threshold:threshold/100});vEls.forEach(function(el){vObs.observe(el);});_cl.push(function(){vObs.disconnect();});}}catch(e){}}
});
});
}
window.addEventListener("etiquetta:consent",function(){consent=window.__ETIQUETTA_CONSENT__;init();});
if(document.readyState==="loading"){document.addEventListener("DOMContentLoaded",init);}else{init();}
%s})();`,
		debugPrefix,
		snapshotJSON,
		debugInit,
		// fireTag consent block
		dbg(`_recordTag(_curEvt,tag,"blocked_consent",tag.consent_category);`),
		// fireTag fired
		dbg(`_recordTag(_curEvt,tag,"fired","");`),
		// condition eval
		dbg(`_recordCondition(_curEvt,cond,val,result);`),
		// exception blocked
		dbg(`_recordTag(_curEvt,tag,"blocked_exception",exceptionTriggers[i].name);`),
		// immediate fire
		dbg(`_curEvt=_beginEvent("page_load","Page Load");`),
		// click fire
		dbg(`_curEvt=_beginEvent("click","Click"+(tr.config.selector?" \u2014 "+tr.config.selector:""));`),
		// custom event fire
		dbg(`_curEvt=_beginEvent("custom_event","Event: "+cfg.event_name);`),
		// scroll fire
		dbg(`_curEvt=_beginEvent("scroll_depth","Scroll "+pct+"%%");`),
		// timer fire
		dbg(`_curEvt=_beginEvent("timer","Timer #"+count);`),
		// history fire
		dbg(`_curEvt=_beginEvent("history_change","Navigation");`),
		// form fire
		dbg(`_curEvt=_beginEvent("form_submit","Form Submit"+(tr.config.selector?" \u2014 "+tr.config.selector:""));`),
		// element visibility fire
		dbg(`_curEvt=_beginEvent("element_visibility","Visible: "+(cfg.selector||""));`),
		// debug panel suffix
		debugSuffix,
	)
}

// generateDebugJS returns the debug console infrastructure JS injected into the container.
func generateDebugJS() string {
	return `var _events=[],_tagStats={},_curEvt=null,_eid=0,_startTs=Date.now();
function _snapVars(){var r={};C.variables.forEach(function(v){try{r[v.name]=String(resolveVar(v));}catch(e){r[v.name]="[error]";}});return r;}
function _beginEvent(type,label){var ev={id:++_eid,ts:Date.now(),type:type,label:label,tags:[],triggers:[],conditions:[],variables:_snapVars()};_events.push(ev);_renderConsole();return ev;}
function _recordTag(ev,tag,status,reason){if(!ev)return;ev.tags.push({name:tag.name,type:tag.tag_type,status:status,reason:reason});if(!_tagStats[tag.name])_tagStats[tag.name]={fired:0,blocked:0,errors:0,last:""};if(status==="fired"){_tagStats[tag.name].fired++;_tagStats[tag.name].last="fired";}else{_tagStats[tag.name].blocked++;_tagStats[tag.name].last=status;}_renderConsole();}
function _recordCondition(ev,cond,actual,passed){if(!ev)return;ev.conditions.push({variable:cond.variable,operator:cond.operator,expected:cond.value,actual:actual,passed:passed});_renderConsole();}
var _panel=null,_body=null,_badge=null,_activeTab="timeline",_collapsed=false,_panelH=320;
function _el(tag,css,html){var e=document.createElement(tag);if(css)e.style.cssText=css;if(html)e.innerHTML=html;return e;}
function _renderConsole(){
if(!document.body)return;
if(!_panel){_buildPanel();}
if(_collapsed){_badge.textContent=_events.length+" events";return;}
_body.innerHTML="";
if(_activeTab==="timeline")_renderTimeline();
else if(_activeTab==="tags")_renderTags();
else if(_activeTab==="variables")_renderVariables();
else if(_activeTab==="datalayer")_renderDataLayer();
else _renderSummary();
_updateTabBar();
}
function _buildPanel(){
_panel=_el("div","position:fixed;bottom:0;left:0;right:0;height:"+_panelH+"px;background:#111827;color:#d1d5db;font:12px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace;z-index:2147483647;border-top:2px solid #6366f1;display:flex;flex-direction:column;");
var drag=_el("div","height:6px;cursor:ns-resize;background:#1f2937;display:flex;align-items:center;justify-content:center;flex-shrink:0;","<div style='width:40px;height:2px;background:#4b5563;border-radius:1px;'></div>");
drag.addEventListener("mousedown",function(me){me.preventDefault();var startY=me.clientY,startH=_panelH;function onMove(e){_panelH=Math.max(160,Math.min(window.innerHeight-40,startH+(startY-e.clientY)));_panel.style.height=_panelH+"px";}function onUp(){document.removeEventListener("mousemove",onMove);document.removeEventListener("mouseup",onUp);}document.addEventListener("mousemove",onMove);document.addEventListener("mouseup",onUp);});
var hdr=_el("div","background:#1e1b4b;color:#e0e7ff;padding:0 12px;display:flex;align-items:center;gap:8px;height:36px;flex-shrink:0;");
hdr.appendChild(_el("span","font-weight:700;font-size:13px;","Etiquetta Debug"));
hdr.appendChild(_el("span","background:#6366f1;color:#fff;font-size:10px;padding:1px 6px;border-radius:3px;font-weight:600;","DRAFT"));
var spacer=_el("div","flex:1;");hdr.appendChild(spacer);
var collapseBtn=_el("button","background:none;border:none;color:#a5b4fc;cursor:pointer;font:12px monospace;padding:4px 8px;","Collapse");
collapseBtn.onclick=function(){_collapsed=true;_panel.style.display="none";_badge.style.display="flex";};
hdr.appendChild(collapseBtn);
var tabs=_el("div","display:flex;gap:0;background:#1f2937;flex-shrink:0;border-bottom:1px solid #374151;");tabs.id="__etq_tabs";
["summary","timeline","tags","variables","datalayer"].forEach(function(t){
var btn=_el("button","background:none;border:none;border-bottom:2px solid transparent;color:#9ca3af;cursor:pointer;padding:6px 14px;font:12px monospace;white-space:nowrap;",t==="datalayer"?"Data Layer":t.charAt(0).toUpperCase()+t.slice(1));
btn.setAttribute("data-tab",t);
btn.onclick=function(){_activeTab=t;_renderConsole();};
tabs.appendChild(btn);
});
_body=_el("div","flex:1;overflow-y:auto;padding:8px 12px;");
_panel.appendChild(drag);_panel.appendChild(hdr);_panel.appendChild(tabs);_panel.appendChild(_body);
document.body.appendChild(_panel);
_badge=_el("div","position:fixed;bottom:12px;right:12px;background:#6366f1;color:#fff;padding:6px 14px;border-radius:20px;font:12px/1 monospace;font-weight:600;cursor:pointer;z-index:2147483647;display:none;align-items:center;gap:6px;box-shadow:0 2px 8px rgba(0,0,0,.3);",_events.length+" events");
_badge.onclick=function(){_collapsed=false;_panel.style.display="flex";_badge.style.display="none";_renderConsole();};
document.body.appendChild(_badge);
}
function _updateTabBar(){
var tabs=document.getElementById("__etq_tabs");if(!tabs)return;
var btns=tabs.getElementsByTagName("button");
for(var i=0;i<btns.length;i++){
var active=btns[i].getAttribute("data-tab")===_activeTab;
btns[i].style.color=active?"#a5b4fc":"#9ca3af";
btns[i].style.borderBottomColor=active?"#6366f1":"transparent";
btns[i].style.background=active?"#111827":"none";
}
}
function _renderSummary(){
var fired=0,blocked=0;_events.forEach(function(ev){ev.tags.forEach(function(t){if(t.status==="fired")fired++;else blocked++;});});
var h="<div style='display:grid;grid-template-columns:1fr 1fr 1fr;gap:12px;margin-bottom:16px;'>";
h+="<div style='background:#1f2937;padding:12px;border-radius:6px;'><div style='color:#6b7280;font-size:11px;'>Events</div><div style='font-size:22px;font-weight:700;color:#e5e7eb;'>"+_events.length+"</div></div>";
h+="<div style='background:#1f2937;padding:12px;border-radius:6px;'><div style='color:#6b7280;font-size:11px;'>Tags Fired</div><div style='font-size:22px;font-weight:700;color:#4ade80;'>"+fired+"</div></div>";
h+="<div style='background:#1f2937;padding:12px;border-radius:6px;'><div style='color:#6b7280;font-size:11px;'>Tags Blocked</div><div style='font-size:22px;font-weight:700;color:#f87171;'>"+blocked+"</div></div>";
h+="</div>";
h+="<div style='background:#1f2937;padding:12px;border-radius:6px;margin-bottom:8px;'>";
h+="<div style='color:#6b7280;font-size:11px;margin-bottom:6px;'>Container</div>";
h+="<div style='color:#e5e7eb;'>Tags: "+C.tags.length+" &middot; Triggers: "+C.triggers.length+" &middot; Variables: "+C.variables.length+"</div>";
h+="</div>";
_body.innerHTML=h;
}
function _renderTimeline(){
if(_events.length===0){_body.innerHTML="<div style='color:#6b7280;padding:20px;text-align:center;'>Waiting for events...</div>";return;}
var h="";
for(var i=_events.length-1;i>=0;i--){
var ev=_events[i];
var elapsed=((ev.ts-_startTs)/1000).toFixed(2);
var icon=ev.type==="click"?"\u{1F5B1}":ev.type==="scroll_depth"?"\u{1F4DC}":ev.type==="timer"?"\u23F1":ev.type==="form_submit"?"\u{1F4DD}":ev.type==="history_change"?"\u{1F517}":ev.type==="custom_event"?"\u26A1":"\u{1F4C4}";
h+="<details style='background:#1f2937;border-radius:6px;margin-bottom:4px;'>";
h+="<summary style='padding:8px 12px;cursor:pointer;display:flex;align-items:center;gap:8px;list-style:none;'>";
h+="<span>"+icon+"</span>";
h+="<span style='flex:1;color:#e5e7eb;font-weight:500;'>"+ev.label+"</span>";
var firedCount=0,blockedCount=0;
ev.tags.forEach(function(t){if(t.status==="fired")firedCount++;else blockedCount++;});
if(firedCount)h+="<span style='background:#065f46;color:#6ee7b7;padding:1px 6px;border-radius:3px;font-size:10px;'>"+firedCount+" fired</span>";
if(blockedCount)h+="<span style='background:#7f1d1d;color:#fca5a5;padding:1px 6px;border-radius:3px;font-size:10px;'>"+blockedCount+" blocked</span>";
h+="<span style='color:#6b7280;font-size:11px;'>+"+elapsed+"s</span>";
h+="</summary>";
h+="<div style='padding:4px 12px 10px;border-top:1px solid #374151;'>";
if(ev.tags.length){
h+="<div style='color:#9ca3af;font-size:11px;margin:6px 0 4px;font-weight:600;'>TAGS</div>";
ev.tags.forEach(function(t){
var statusColor=t.status==="fired"?"#4ade80":t.status==="blocked_consent"?"#fb923c":"#f87171";
var statusLabel=t.status==="fired"?"Fired":t.status==="blocked_consent"?"Blocked (consent: "+t.reason+")":t.status==="blocked_exception"?"Blocked (exception: "+t.reason+")":"Error";
h+="<div style='display:flex;align-items:center;gap:8px;padding:2px 0;'>";
h+="<span style='width:6px;height:6px;border-radius:50%;background:"+statusColor+";flex-shrink:0;'></span>";
h+="<span style='color:#e5e7eb;'>"+t.name+"</span>";
h+="<span style='color:#6b7280;font-size:11px;'>("+t.type+")</span>";
h+="<span style='color:"+statusColor+";font-size:11px;margin-left:auto;'>"+statusLabel+"</span>";
h+="</div>";
});
}
if(ev.conditions.length){
h+="<div style='color:#9ca3af;font-size:11px;margin:8px 0 4px;font-weight:600;'>CONDITIONS</div>";
ev.conditions.forEach(function(c){
var passColor=c.passed?"#4ade80":"#f87171";
h+="<div style='display:flex;align-items:center;gap:6px;padding:2px 0;font-size:11px;'>";
h+="<span style='color:"+passColor+";'>"+(c.passed?"\u2713":"\u2717")+"</span>";
h+="<span style='color:#93c5fd;'>"+c.variable+"</span>";
h+="<span style='color:#6b7280;'>"+c.operator+"</span>";
h+="<span style='color:#fbbf24;'>\""+c.expected+"\"</span>";
h+="<span style='color:#6b7280;'>(actual: \""+c.actual+"\")</span>";
h+="</div>";
});
}
h+="</div></details>";
}
_body.innerHTML=h;
}
function _renderTags(){
var h="";
C.tags.forEach(function(tag){
var s=_tagStats[tag.name]||{fired:0,blocked:0,errors:0,last:""};
var dotColor=s.last==="fired"?"#4ade80":s.last?"#f87171":"#6b7280";
h+="<div style='display:flex;align-items:center;gap:8px;padding:8px 4px;border-bottom:1px solid #1f2937;'>";
h+="<span style='width:8px;height:8px;border-radius:50%;background:"+dotColor+";flex-shrink:0;'></span>";
h+="<div style='flex:1;'><div style='color:#e5e7eb;font-weight:500;'>"+tag.name+"</div><div style='color:#6b7280;font-size:11px;'>"+tag.tag_type+(tag.consent_category?" &middot; consent: "+tag.consent_category:"")+"</div></div>";
h+="<span style='color:#4ade80;font-size:11px;'>"+s.fired+" fired</span>";
if(s.blocked)h+="<span style='color:#f87171;font-size:11px;margin-left:4px;'>"+s.blocked+" blocked</span>";
h+="</div>";
});
if(!C.tags.length)h="<div style='color:#6b7280;padding:20px;text-align:center;'>No tags configured</div>";
_body.innerHTML=h;
}
function _renderVariables(){
var h="";
C.variables.forEach(function(v){
var val;try{val=String(resolveVar(v));}catch(e){val="[error]";}
h+="<div style='display:flex;align-items:flex-start;gap:8px;padding:6px 4px;border-bottom:1px solid #1f2937;'>";
h+="<div style='flex:1;'><div style='color:#93c5fd;font-weight:500;'>"+v.name+"</div><div style='color:#6b7280;font-size:11px;'>"+v.variable_type+"</div></div>";
h+="<div style='color:#fbbf24;font-size:11px;max-width:50%%;word-break:break-all;text-align:right;'>"+(val||"<span style='color:#6b7280;'>(empty)</span>")+"</div>";
h+="</div>";
});
if(!C.variables.length)h="<div style='color:#6b7280;padding:20px;text-align:center;'>No variables configured</div>";
_body.innerHTML=h;
}
function _renderDataLayer(){
var dl=window.etiquettaDataLayer||[];
if(!dl.length){_body.innerHTML="<div style='color:#6b7280;padding:20px;text-align:center;'>Data layer is empty</div>";return;}
var h="<div style='color:#9ca3af;font-size:11px;margin-bottom:8px;'>"+dl.length+" entries in etiquettaDataLayer</div>";
for(var i=dl.length-1;i>=0;i--){
h+="<details style='background:#1f2937;border-radius:4px;margin-bottom:4px;'>";
h+="<summary style='padding:6px 10px;cursor:pointer;color:#e5e7eb;font-size:11px;list-style:none;'>["+i+"] "+Object.keys(dl[i]||{}).join(", ")+"</summary>";
h+="<pre style='padding:6px 10px;color:#a5b4fc;font-size:11px;overflow-x:auto;margin:0;border-top:1px solid #374151;'>"+JSON.stringify(dl[i],null,2)+"</pre>";
h+="</details>";
}
_body.innerHTML=h;
}
setInterval(function(){if(!_collapsed&&(_activeTab==="variables"||_activeTab==="datalayer"))_renderConsole();},2000);
`
}

// generatePickerJS returns a self-contained element picker script for the new-tab approach.
// It highlights elements on hover, generates selector suggestions on click, and shows
// a built-in suggestion panel. The chosen selector is sent to window.opener via postMessage.
func generatePickerJS() string {
	return `(function(){
"use strict";
var parent=(window.parent!==window)?window.parent:null;
var active=true,panel=null,lastHighlighted=null;

var style=document.createElement("style");
style.textContent=[
".__etq_pick_hl{outline:2px solid #6366f1!important;outline-offset:-1px;cursor:crosshair!important;}",
".__etq_pick_tooltip{position:fixed;z-index:2147483647;background:#1e1b4b;color:#e0e7ff;font:12px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;padding:6px 10px;border-radius:6px;pointer-events:none;max-width:400px;white-space:nowrap;box-shadow:0 4px 12px rgba(0,0,0,.3);}",
".__etq_pick_panel{position:fixed;bottom:0;left:0;right:0;z-index:2147483647;background:#1e1b4b;color:#e0e7ff;font:13px/1.4 system-ui,sans-serif;box-shadow:0 -4px 16px rgba(0,0,0,.3);max-height:45vh;overflow-y:auto;}",
".__etq_pick_panel_header{display:flex;align-items:center;gap:12px;padding:12px 16px;border-bottom:1px solid rgba(255,255,255,.1);}",
".__etq_pick_panel_header span{font-weight:600;}",
".__etq_pick_panel_list{padding:8px;}",
".__etq_pick_panel_item{display:flex;align-items:center;gap:10px;padding:8px 12px;border-radius:6px;cursor:pointer;transition:background .15s;width:100%;border:none;background:none;color:inherit;font:inherit;text-align:left;}",
".__etq_pick_panel_item:hover{background:rgba(99,102,241,.25);}",
".__etq_pick_panel_item .badge{background:rgba(99,102,241,.3);color:#a5b4fc;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600;white-space:nowrap;}",
".__etq_pick_panel_item .best{background:#6366f1;color:#fff;}",
".__etq_pick_panel_item .selector{font:11px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;color:#c7d2fe;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1;min-width:0;}"
].join("");
document.head.appendChild(style);


// Tooltip
var tooltip=document.createElement("div");
tooltip.className="__etq_pick_tooltip";
tooltip.style.display="none";
document.body.appendChild(tooltip);

var typeLabels={id:"Element ID",css:"CSS Selector",data_attr:"Data Attribute",text:"Text Content",link_url:"Link URL"};

function getElementInfo(el){
var tag=el.tagName.toLowerCase();
var id=el.id||"";
var classes=Array.from(el.classList).filter(function(c){return c.indexOf("__etq_pick")===-1;});
var text=(el.textContent||"").trim().replace(/\s+/g," ");
if(text.length>80)text=text.slice(0,80)+"...";
var dataAttrs={};
Array.from(el.attributes).forEach(function(attr){
if(attr.name.indexOf("data-")===0){
dataAttrs[attr.name.replace("data-","")]=attr.value;
}
});
var href=el.tagName==="A"?el.getAttribute("href")||"":"";
return {tag:tag,id:id,classes:classes,text:text,dataAttrs:dataAttrs,href:href};
}

function generateSelectors(el){
var info=getElementInfo(el);
var suggestions=[];
if(info.id){
suggestions.push({type:"id",label:"#"+info.id,selector:"#"+info.id,specificity:100,data_attr_name:"",data_attr_value:""});
}
for(var key in info.dataAttrs){
var val=info.dataAttrs[key];
var sel=val?"[data-"+key+'="'+val+'"]':"[data-"+key+"]";
suggestions.push({type:"data_attr",label:sel,selector:sel,specificity:80,data_attr_name:key,data_attr_value:val});
}
if(info.href&&info.href!=="/"&&info.href!=="#"){
suggestions.push({type:"link_url",label:'a[href*="'+info.href+'"]',selector:'a[href*="'+info.href+'"]',specificity:70});
}
if(info.text&&info.text.length>0&&info.text.length<=60){
suggestions.push({type:"text",label:'"'+info.text+'"',selector:info.text,specificity:60});
}
if(info.classes.length>0){
var classSel=info.tag+"."+info.classes.join(".");
suggestions.push({type:"css",label:classSel,selector:classSel,specificity:50});
}
var parent=el.parentElement;
if(parent){
var siblings=Array.from(parent.children);
var idx=siblings.indexOf(el)+1;
var structSel=info.tag+":nth-child("+idx+")";
var path=[structSel];
var cur=parent;
for(var i=0;i<2&&cur&&cur!==document.body;i++){
var pTag=cur.tagName.toLowerCase();
if(cur.id){path.unshift(pTag+"#"+cur.id);break;}
var pSiblings=cur.parentElement?Array.from(cur.parentElement.children):[];
var pIdx=pSiblings.indexOf(cur)+1;
path.unshift(pTag+":nth-child("+pIdx+")");
cur=cur.parentElement;
}
suggestions.push({type:"css",label:path.join(" > "),selector:path.join(" > "),specificity:30});
}
if(suggestions.length===0){
suggestions.push({type:"css",label:info.tag,selector:info.tag,specificity:10});
}
suggestions.sort(function(a,b){return b.specificity-a.specificity;});
return {suggestions:suggestions,tag:info.tag,text:info.text};
}

function showTooltip(el,x,y){
var info=getElementInfo(el);
var parts=["&lt;"+info.tag+"&gt;"];
if(info.id)parts.push('<span style="color:#a5b4fc;">#'+info.id+"</span>");
if(info.classes.length)parts.push('<span style="color:#86efac;">.'+info.classes.slice(0,3).join(".")+"</span>");
if(info.text)parts.push('<span style="opacity:.6;margin-left:4px;">'+info.text.slice(0,40)+"</span>");
tooltip.innerHTML=parts.join(" ");
tooltip.style.display="block";
var tw=tooltip.offsetWidth,th=tooltip.offsetHeight;
var tx=Math.min(x+12,window.innerWidth-tw-8);
var ty=y-th-8;if(ty<40)ty=y+16;
tooltip.style.left=tx+"px";
tooltip.style.top=ty+"px";
}

function showPanel(result){
if(panel)panel.parentNode.removeChild(panel);
panel=document.createElement("div");
panel.className="__etq_pick_panel";
var header=document.createElement("div");
header.className="__etq_pick_panel_header";
header.innerHTML='<span>Selected: &lt;'+result.tag+'&gt;</span>'+(result.text?'<span style="opacity:.6;font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:300px;">"'+result.text.slice(0,50)+'"</span>':'');
var pickAnotherBtn=document.createElement("button");
pickAnotherBtn.style.cssText="margin-left:auto;background:#6366f1;color:#fff;border:none;padding:6px 14px;border-radius:4px;cursor:pointer;font:12px/1 system-ui;font-weight:600;";
pickAnotherBtn.textContent="Pick Another";
pickAnotherBtn.onclick=function(){
hidePanel();
active=true;
};
header.appendChild(pickAnotherBtn);
panel.appendChild(header);
var list=document.createElement("div");
list.className="__etq_pick_panel_list";
result.suggestions.forEach(function(s,i){
var btn=document.createElement("button");
btn.className="__etq_pick_panel_item";
btn.innerHTML='<span class="badge'+(i===0?" best":"")+'">'+((typeLabels[s.type])||s.type)+'</span><span class="selector">'+s.selector+'</span>';
btn.onclick=function(){
// Send to parent iframe via postMessage
document.cookie="__etq_pick=;Path=/;Max-Age=0";if(parent){try{parent.postMessage({type:"etiquetta_picker_select",suggestion:s},"*");}catch(err){}}
cleanup();
};
list.appendChild(btn);
});
panel.appendChild(list);
document.body.appendChild(panel);
}

function hidePanel(){
if(panel&&panel.parentNode){panel.parentNode.removeChild(panel);panel=null;}
}

function onMouseMove(e){
if(!active)return;
var el=document.elementFromPoint(e.clientX,e.clientY);
if(!el||el===tooltip||(panel&&panel.contains(el)))return;
if(lastHighlighted&&lastHighlighted!==el){
lastHighlighted.classList.remove("__etq_pick_hl");
}
el.classList.add("__etq_pick_hl");
lastHighlighted=el;
showTooltip(el,e.clientX,e.clientY);
}

function onClick(e){
if(!active)return;
if(panel&&panel.contains(e.target))return;
e.preventDefault();
e.stopPropagation();
e.stopImmediatePropagation();
var el=lastHighlighted||document.elementFromPoint(e.clientX,e.clientY);
if(!el||el===tooltip)return;
var result=generateSelectors(el);
el.style.outline="3px solid #22c55e";
setTimeout(function(){el.style.outline="";el.classList.remove("__etq_pick_hl");},800);
active=false;
if(lastHighlighted){lastHighlighted.classList.remove("__etq_pick_hl");lastHighlighted=null;}
tooltip.style.display="none";
showPanel(result);
}

function cleanup(){
active=false;
document.cookie="__etq_pick=;Path=/;Max-Age=0";
document.removeEventListener("mousemove",onMouseMove,true);
document.removeEventListener("click",onClick,true);
if(lastHighlighted)lastHighlighted.classList.remove("__etq_pick_hl");
if(tooltip.parentNode)tooltip.parentNode.removeChild(tooltip);
if(panel&&panel.parentNode)panel.parentNode.removeChild(panel);
if(style.parentNode)style.parentNode.removeChild(style);
}

document.addEventListener("mousemove",onMouseMove,true);
document.addEventListener("click",onClick,true);

// Listen for close signal from opener
window.addEventListener("message",function(e){
if(e.data==="etiquetta_picker_close")cleanup();
if(e.data==="etiquetta_picker_resume"){active=true;hidePanel();}
});

// Signal ready
if(parent){try{parent.postMessage({type:"etiquetta_picker_ready"},"*");}catch(e){}}
})();`
}
