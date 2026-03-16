package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// ServeConsentScript serves the embedded consent banner JavaScript
func (h *Handlers) ServeConsentScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	script, err := consentJS.ReadFile("consent.js")
	if err != nil {
		http.Error(w, "Script not found", http.StatusNotFound)
		return
	}

	w.Write(script)
}

// GetPublicConsentConfig returns the active consent config for a site (public, no auth)
func (h *Handlers) GetPublicConsentConfig(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteId")
	if siteID == "" {
		writeError(w, http.StatusBadRequest, "Missing site ID")
		return
	}

	log.Printf("[consent] GetPublicConsentConfig: siteId=%s", siteID)

	// Look up domain by site_id
	var domainID string
	err := h.db.Conn().QueryRow("SELECT id FROM domains WHERE site_id = ? AND is_active = 1", siteID).Scan(&domainID)
	if err != nil {
		log.Printf("[consent] GetPublicConsentConfig: domain lookup failed for siteId=%s: %v", siteID, err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	log.Printf("[consent] GetPublicConsentConfig: found domainId=%s", domainID)

	// Find active consent config for this domain
	// Scan JSON columns into strings first
	var (
		id, domID                                          string
		version, cookieExpiry                               int
		categories, appearance, translations, geoTargeting string
		cookieName                                         string
		autoLanguage                                       int
	)

	err = h.db.Conn().QueryRow(`
		SELECT id, domain_id, version, categories, appearance, translations,
		       cookie_name, cookie_expiry_days, auto_language, geo_targeting
		FROM consent_configs
		WHERE domain_id = ? AND is_active = 1
		ORDER BY version DESC
		LIMIT 1
	`, domainID).Scan(
		&id, &domID, &version,
		&categories, &appearance, &translations,
		&cookieName, &cookieExpiry, &autoLanguage,
		&geoTargeting,
	)
	if err != nil {
		log.Printf("[consent] GetPublicConsentConfig: config query failed for domainId=%s: %v", domainID, err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	log.Printf("[consent] GetPublicConsentConfig: found config id=%s version=%d", id, version)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                 id,
		"domain_id":          domID,
		"version":            version,
		"categories":         json.RawMessage(categories),
		"appearance":         json.RawMessage(appearance),
		"translations":       json.RawMessage(translations),
		"cookie_name":        cookieName,
		"cookie_expiry_days": cookieExpiry,
		"auto_language":      autoLanguage != 0,
		"geo_targeting":      json.RawMessage(geoTargeting),
	})
}

// RecordConsent records a visitor's consent decision (public, no auth)
func (h *Handlers) RecordConsent(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteId")
	if siteID == "" {
		writeError(w, http.StatusBadRequest, "Missing site ID")
		return
	}

	// Look up domain by site_id
	var domainID string
	err := h.db.Conn().QueryRow("SELECT id FROM domains WHERE site_id = ? AND is_active = 1", siteID).Scan(&domainID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Domain not found")
		return
	}

	var req struct {
		VisitorHash   string          `json:"visitor_hash"`
		Categories    json.RawMessage `json:"categories"`
		ConfigVersion int             `json:"config_version"`
		Action        string          `json:"action"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Hash the IP with a salt from config
	clientIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		clientIP = fwd
	} else if real := r.Header.Get("X-Real-IP"); real != "" {
		clientIP = real
	}
	ipHash := hashIPWithSalt(clientIP, h.cfg.SecretKey)

	// Get user agent and geo country
	userAgent := r.Header.Get("User-Agent")
	enriched := h.enricher.Enrich(clientIP, userAgent, "")
	geoCountry := enriched.GeoCountry

	// For 'show' events, categories are optional (empty object)
	var categoriesJSON []byte
	if req.Categories != nil {
		categoriesJSON, _ = json.Marshal(req.Categories)
	} else {
		categoriesJSON = []byte("{}")
	}

	now := time.Now().UnixMilli()
	id := generateID()

	_, err = h.db.Conn().Exec(`
		INSERT INTO consent_records (id, domain_id, visitor_hash, ip_hash, categories, config_version, action, user_agent, geo_country, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, domainID, req.VisitorHash, ipHash, string(categoriesJSON), req.ConfigVersion, req.Action, userAgent, geoCountry, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to record consent")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetConsentConfig returns the active consent config for a domain (auth required)
func (h *Handlers) GetConsentConfig(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainId")

	log.Printf("[consent] GetConsentConfig: domainId=%s", domainID)

	var (
		id, domID                                          string
		version, cookieExpiry                               int
		isActive, autoLanguage                             int
		categories, appearance, translations, geoTargeting string
		cookieName                                         string
		createdAt, updatedAt                               int64
	)

	err := h.db.Conn().QueryRow(`
		SELECT id, domain_id, version, is_active, categories, appearance, translations,
		       cookie_name, cookie_expiry_days, auto_language, geo_targeting, created_at, updated_at
		FROM consent_configs
		WHERE domain_id = ?
		ORDER BY version DESC
		LIMIT 1
	`, domainID).Scan(
		&id, &domID, &version, &isActive,
		&categories, &appearance, &translations,
		&cookieName, &cookieExpiry, &autoLanguage,
		&geoTargeting, &createdAt, &updatedAt,
	)
	if err != nil {
		log.Printf("[consent] GetConsentConfig: query failed for domainId=%s: %v", domainID, err)
		writeError(w, http.StatusNotFound, "No consent config found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                 id,
		"domain_id":          domID,
		"version":            version,
		"is_active":          isActive != 0,
		"categories":         json.RawMessage(categories),
		"appearance":         json.RawMessage(appearance),
		"translations":       json.RawMessage(translations),
		"cookie_name":        cookieName,
		"cookie_expiry_days": cookieExpiry,
		"auto_language":      autoLanguage != 0,
		"geo_targeting":      json.RawMessage(geoTargeting),
		"created_at":         createdAt,
		"updated_at":         updatedAt,
	})
}

// SaveConsentConfig creates a new version of the consent config (auth required)
func (h *Handlers) SaveConsentConfig(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainId")

	var req struct {
		Categories       json.RawMessage `json:"categories"`
		Appearance       json.RawMessage `json:"appearance"`
		Translations     json.RawMessage `json:"translations"`
		CookieName       string          `json:"cookie_name"`
		CookieExpiryDays int             `json:"cookie_expiry_days"`
		AutoLanguage     bool            `json:"auto_language"`
		GeoTargeting     json.RawMessage `json:"geo_targeting"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Defaults
	if req.CookieName == "" {
		req.CookieName = "etiquetta_consent"
	}
	if req.CookieExpiryDays == 0 {
		req.CookieExpiryDays = 365
	}
	if req.Categories == nil {
		req.Categories = json.RawMessage("[]")
	}
	if req.Appearance == nil {
		req.Appearance = json.RawMessage("{}")
	}
	if req.Translations == nil {
		req.Translations = json.RawMessage("{}")
	}
	if req.GeoTargeting == nil {
		req.GeoTargeting = json.RawMessage("[]")
	}

	tx, err := h.db.Conn().Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	// Get current max version
	var maxVersion int
	tx.QueryRow("SELECT COALESCE(MAX(version), 0) FROM consent_configs WHERE domain_id = ?", domainID).Scan(&maxVersion)
	newVersion := maxVersion + 1

	// Deactivate previous versions
	_, err = tx.Exec("UPDATE consent_configs SET is_active = 0 WHERE domain_id = ?", domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to deactivate old configs")
		return
	}

	now := time.Now().UnixMilli()
	id := generateID()

	_, err = tx.Exec(`
		INSERT INTO consent_configs (id, domain_id, version, is_active, categories, appearance, translations,
		                             cookie_name, cookie_expiry_days, auto_language, geo_targeting, created_at, updated_at)
		VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, domainID, newVersion,
		string(req.Categories), string(req.Appearance), string(req.Translations),
		req.CookieName, req.CookieExpiryDays, req.AutoLanguage,
		string(req.GeoTargeting), now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save config")
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to commit")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                id,
		"domain_id":         domainID,
		"version":           newVersion,
		"is_active":         true,
		"categories":        req.Categories,
		"appearance":        req.Appearance,
		"translations":      req.Translations,
		"cookie_name":       req.CookieName,
		"cookie_expiry_days": req.CookieExpiryDays,
		"auto_language":     req.AutoLanguage,
		"geo_targeting":     req.GeoTargeting,
		"created_at":        now,
		"updated_at":        now,
	})
}

// GetConsentConfigHistory returns all config versions for a domain (auth required)
func (h *Handlers) GetConsentConfigHistory(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainId")

	rows, err := h.db.Conn().Query(`
		SELECT id, domain_id, version, is_active, categories, appearance, translations,
		       cookie_name, cookie_expiry_days, auto_language, geo_targeting, created_at, updated_at
		FROM consent_configs
		WHERE domain_id = ?
		ORDER BY version DESC
	`, domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var configs []map[string]interface{}
	for rows.Next() {
		var (
			id, domID, cookieName                          string
			categories, appearance, translations, geoTarget string
			version, cookieExpiry                           int
			isActive, autoLang                             bool
			createdAt, updatedAt                           int64
		)
		if err := rows.Scan(&id, &domID, &version, &isActive, &categories, &appearance, &translations,
			&cookieName, &cookieExpiry, &autoLang, &geoTarget, &createdAt, &updatedAt); err != nil {
			continue
		}
		configs = append(configs, map[string]interface{}{
			"id":                 id,
			"domain_id":          domID,
			"version":            version,
			"is_active":          isActive,
			"categories":         json.RawMessage(categories),
			"appearance":         json.RawMessage(appearance),
			"translations":       json.RawMessage(translations),
			"cookie_name":        cookieName,
			"cookie_expiry_days": cookieExpiry,
			"auto_language":      autoLang,
			"geo_targeting":      json.RawMessage(geoTarget),
			"created_at":         createdAt,
			"updated_at":         updatedAt,
		})
	}

	if configs == nil {
		configs = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, configs)
}

// GetConsentAnalytics returns consent analytics for a domain (auth required)
func (h *Handlers) GetConsentAnalytics(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainId")
	startMs, endMs := getDateRangeParams(r, 30)

	// Action counts (single query)
	var shows, acceptAll, rejectAll, custom int
	rows, err := h.db.Conn().Query(`
		SELECT action, COUNT(*) as cnt
		FROM consent_records
		WHERE domain_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY action
	`, domainID, startMs, endMs)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var action string
			var cnt int
			if rows.Scan(&action, &cnt) == nil {
				switch action {
				case "show":
					shows = cnt
				case "accept_all":
					acceptAll = cnt
				case "reject_all":
					rejectAll = cnt
				case "custom":
					custom = cnt
				}
			}
		}
	}

	totalResponses := acceptAll + rejectAll + custom
	var consentRate, responseRate float64
	if totalResponses > 0 {
		consentRate = float64(acceptAll+custom) / float64(totalResponses)
	}
	if shows > 0 {
		responseRate = float64(totalResponses) / float64(shows)
	}

	// Timeseries — daily breakdown
	var timeseries []map[string]interface{}
	tsRows, err := h.db.Conn().Query(`
		SELECT strftime('%Y-%m-%d', to_timestamp(timestamp / 1000)::TIMESTAMP) as day,
			SUM(CASE WHEN action = 'show' THEN 1 ELSE 0 END) as shows,
			SUM(CASE WHEN action = 'accept_all' THEN 1 ELSE 0 END) as accept_all,
			SUM(CASE WHEN action = 'reject_all' THEN 1 ELSE 0 END) as reject_all,
			SUM(CASE WHEN action = 'custom' THEN 1 ELSE 0 END) as custom
		FROM consent_records
		WHERE domain_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY day
		ORDER BY day ASC
	`, domainID, startMs, endMs)
	if err == nil {
		defer tsRows.Close()
		for tsRows.Next() {
			var day string
			var s, a, rj, c int
			if tsRows.Scan(&day, &s, &a, &rj, &c) == nil {
				timeseries = append(timeseries, map[string]interface{}{
					"period":     day,
					"shows":      s,
					"accept_all": a,
					"reject_all": rj,
					"custom":     c,
				})
			}
		}
	}
	if timeseries == nil {
		timeseries = []map[string]interface{}{}
	}

	// Geo breakdown — top countries
	var geoBreakdown []map[string]interface{}
	geoRows, err := h.db.Conn().Query(`
		SELECT COALESCE(geo_country, 'Unknown') as country,
			SUM(CASE WHEN action = 'show' THEN 1 ELSE 0 END) as shows,
			SUM(CASE WHEN action = 'accept_all' THEN 1 ELSE 0 END) as accept_all,
			SUM(CASE WHEN action = 'reject_all' THEN 1 ELSE 0 END) as reject_all,
			SUM(CASE WHEN action = 'custom' THEN 1 ELSE 0 END) as custom
		FROM consent_records
		WHERE domain_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY country
		ORDER BY shows DESC
		LIMIT 20
	`, domainID, startMs, endMs)
	if err == nil {
		defer geoRows.Close()
		for geoRows.Next() {
			var country string
			var s, a, rj, c int
			if geoRows.Scan(&country, &s, &a, &rj, &c) == nil {
				responses := a + rj + c
				var rate float64
				if responses > 0 {
					rate = float64(a+c) / float64(responses)
				}
				geoBreakdown = append(geoBreakdown, map[string]interface{}{
					"country":      country,
					"shows":        s,
					"responses":    responses,
					"accept_all":   a,
					"reject_all":   rj,
					"custom":       c,
					"consent_rate": rate,
				})
			}
		}
	}
	if geoBreakdown == nil {
		geoBreakdown = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"shows":            shows,
		"total_responses":  totalResponses,
		"accept_all_count": acceptAll,
		"reject_all_count": rejectAll,
		"custom_count":     custom,
		"consent_rate":     consentRate,
		"response_rate":    responseRate,
		"timeseries":       timeseries,
		"geo_breakdown":    geoBreakdown,
	})
}

// GetConsentRecords returns paginated consent records for a domain (auth required)
func (h *Handlers) GetConsentRecords(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainId")

	page := 1
	perPage := 50
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 200 {
			perPage = v
		}
	}
	offset := (page - 1) * perPage

	// Get total count
	var total int
	h.db.Conn().QueryRow("SELECT COUNT(*) FROM consent_records WHERE domain_id = ?", domainID).Scan(&total)

	rows, err := h.db.Conn().Query(`
		SELECT id, domain_id, visitor_hash, ip_hash, categories, config_version, action, user_agent, geo_country, timestamp
		FROM consent_records
		WHERE domain_id = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, domainID, perPage, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var records []map[string]interface{}
	for rows.Next() {
		var (
			id, domID, visitorHash, action string
			ipHash, userAgent, geoCountry  *string
			categories                     string
			configVersion                  int
			timestamp                      int64
		)
		if err := rows.Scan(&id, &domID, &visitorHash, &ipHash, &categories, &configVersion, &action, &userAgent, &geoCountry, &timestamp); err != nil {
			continue
		}
		record := map[string]interface{}{
			"id":             id,
			"domain_id":      domID,
			"visitor_hash":   visitorHash,
			"ip_hash":        ipHash,
			"categories":     json.RawMessage(categories),
			"config_version": configVersion,
			"action":         action,
			"user_agent":     userAgent,
			"geo_country":    geoCountry,
			"timestamp":      timestamp,
		}
		records = append(records, record)
	}

	if records == nil {
		records = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"records":  records,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// ToggleConsentBanner enables or disables the consent banner for a domain
func (h *Handlers) ToggleConsentBanner(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainId")

	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	now := time.Now().UnixMilli()

	result, err := h.db.Conn().Exec(`
		UPDATE consent_configs SET is_active = ?, updated_at = ?
		WHERE domain_id = ? AND version = (SELECT MAX(version) FROM consent_configs WHERE domain_id = ?)
	`, req.IsActive, now, domainID, domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to toggle consent banner")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "No consent config found for this domain")
		return
	}

	action := "disabled"
	if req.IsActive {
		action = "enabled"
	}
	h.logAudit(r, "toggle", "consent_banner", domainID, "Consent banner "+action)

	// Return updated config
	var (
		id, domID                                          string
		version, cookieExpiry                               int
		isActive, autoLanguage                             int
		categories, appearance, translations, geoTargeting string
		cookieName                                         string
		createdAt, updatedAt                               int64
	)
	err = h.db.Conn().QueryRow(`
		SELECT id, domain_id, version, is_active, categories, appearance, translations,
		       cookie_name, cookie_expiry_days, auto_language, geo_targeting, created_at, updated_at
		FROM consent_configs
		WHERE domain_id = ?
		ORDER BY version DESC
		LIMIT 1
	`, domainID).Scan(
		&id, &domID, &version, &isActive,
		&categories, &appearance, &translations,
		&cookieName, &cookieExpiry, &autoLanguage,
		&geoTargeting, &createdAt, &updatedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to read updated config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                 id,
		"domain_id":          domID,
		"version":            version,
		"is_active":          isActive != 0,
		"categories":         json.RawMessage(categories),
		"appearance":         json.RawMessage(appearance),
		"translations":       json.RawMessage(translations),
		"cookie_name":        cookieName,
		"cookie_expiry_days": cookieExpiry,
		"auto_language":      autoLanguage != 0,
		"geo_targeting":      json.RawMessage(geoTargeting),
		"created_at":         createdAt,
		"updated_at":         updatedAt,
	})
}

// hashIPWithSalt creates a SHA-256 hash of an IP with a secret salt
func hashIPWithSalt(ip, salt string) string {
	h := sha256.New()
	h.Write([]byte(ip + salt))
	return hex.EncodeToString(h.Sum(nil))
}

