package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/connections"
	"github.com/caioricciuti/etiquetta/internal/connections/providers"
)

// ListConnections returns all ad platform connections
func (h *Handlers) ListConnections(w http.ResponseWriter, r *http.Request) {
	conns, err := h.connStore.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list connections")
		return
	}
	writeJSON(w, http.StatusOK, conns)
}

// GetConnection returns a single connection by ID
func (h *Handlers) GetConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	conn, err := h.connStore.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}
	writeJSON(w, http.StatusOK, conn)
}

// CreateConnection creates a new ad platform connection.
// If refresh_token is provided in the request body, it validates the token
// immediately and sets the connection to "active" status.
func (h *Handlers) CreateConnection(w http.ResponseWriter, r *http.Request) {
	// Check connection limit
	count, err := h.connStore.Count()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check connection count")
		return
	}

	maxConns := h.licenseManager.GetLimit("max_connections")
	if maxConns >= 0 && count >= maxConns {
		writeError(w, http.StatusForbidden, "connection limit reached for your license tier")
		return
	}

	var req struct {
		Provider     string            `json:"provider"`
		Name         string            `json:"name"`
		AccountID    string            `json:"account_id"`
		Config       map[string]string `json:"config"`
		RefreshToken string            `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Provider == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "provider and name are required")
		return
	}

	// Validate provider exists
	provider, ok := providers.Get(req.Provider)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported provider: "+req.Provider)
		return
	}

	claims := auth.GetUserFromContext(r.Context())
	createdBy := ""
	if claims != nil {
		createdBy = claims.UserID
	}

	conn := &connections.Connection{
		ID:        generateID(),
		Provider:  req.Provider,
		Name:      req.Name,
		AccountID: req.AccountID,
		Status:    "pending",
		Config:    req.Config,
		CreatedBy: createdBy,
	}
	if conn.Config == nil {
		conn.Config = map[string]string{}
	}

	// If refresh token provided, validate it immediately
	var tokens *providers.TokenSet
	if req.RefreshToken != "" {
		tokens, err = provider.RefreshToken(r.Context(), req.RefreshToken)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid refresh token: "+err.Error())
			return
		}
		conn.Status = "active"
	} else {
		tokens = &providers.TokenSet{
			AccessToken:  "",
			RefreshToken: "",
			ExpiresAt:    time.Now(),
		}
	}

	if err := h.connStore.Create(conn, tokens); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create connection")
		return
	}

	h.logAudit(r, "create", "connection", conn.ID, "provider="+req.Provider)
	writeJSON(w, http.StatusCreated, conn)
}

// DeleteConnection removes a connection and its spend data
func (h *Handlers) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify it exists
	if _, err := h.connStore.Get(id); err != nil {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}

	if err := h.connStore.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete connection")
		return
	}

	h.logAudit(r, "delete", "connection", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// SyncConnection triggers a manual sync for a connection
func (h *Handlers) SyncConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if h.syncManager == nil {
		writeError(w, http.StatusServiceUnavailable, "sync manager not available")
		return
	}

	if err := h.syncManager.SyncConnection(id); err != nil {
		writeError(w, http.StatusInternalServerError, "sync failed: "+err.Error())
		return
	}

	h.logAudit(r, "sync", "connection", id, "manual")
	writeJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

// UpdateConnectionToken accepts a refresh token for an existing connection,
// validates it via the provider, and stores the resulting tokens.
func (h *Handlers) UpdateConnectionToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	conn, err := h.connStore.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}

	provider, ok := providers.Get(conn.Provider)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported provider")
		return
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	tokens, err := provider.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		errMsg := err.Error()
		h.connStore.UpdateStatus(id, "error", &errMsg)
		writeError(w, http.StatusBadRequest, "invalid refresh token: "+err.Error())
		return
	}

	if err := h.connStore.UpdateTokens(id, tokens); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store tokens")
		return
	}

	h.connStore.UpdateStatus(id, "active", nil)
	h.logAudit(r, "update_token", "connection", id, "provider="+conn.Provider)
	writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

// GetAdSpend returns aggregated ad spend data for the dashboard
func (h *Handlers) GetAdSpend(w http.ResponseWriter, r *http.Request) {
	f := parseStatsFilter(r)

	startDate := time.UnixMilli(f.startMs).Format("2006-01-02")
	endDate := time.UnixMilli(f.endMs).Format("2006-01-02")

	rows, err := h.db.Conn().QueryContext(r.Context(), `
		SELECT
			date,
			provider,
			SUM(cost_micros) as total_cost_micros,
			SUM(impressions) as total_impressions,
			SUM(clicks) as total_clicks
		FROM ad_spend_daily
		WHERE date >= ? AND date <= ?
		GROUP BY date, provider
		ORDER BY date
	`, startDate, endDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query ad spend")
		return
	}
	defer rows.Close()

	type SpendPoint struct {
		Date        string  `json:"date"`
		Provider    string  `json:"provider"`
		Cost        float64 `json:"cost"`
		Impressions int     `json:"impressions"`
		Clicks      int     `json:"clicks"`
	}

	var result []SpendPoint
	for rows.Next() {
		var sp SpendPoint
		var costMicros int64
		if err := rows.Scan(&sp.Date, &sp.Provider, &costMicros, &sp.Impressions, &sp.Clicks); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		sp.Cost = float64(costMicros) / 1_000_000
		result = append(result, sp)
	}
	if result == nil {
		result = []SpendPoint{}
	}

	writeJSON(w, http.StatusOK, result)
}

// GetAdAttribution returns UTM-matched spend + analytics per campaign
func (h *Handlers) GetAdAttribution(w http.ResponseWriter, r *http.Request) {
	f := parseStatsFilter(r)

	startDate := time.UnixMilli(f.startMs).Format("2006-01-02")
	endDate := time.UnixMilli(f.endMs).Format("2006-01-02")

	where, args := f.where("e.timestamp >= ? AND e.timestamp <= ? AND e.is_bot = 0", f.startMs, f.endMs)

	// Join ad_spend_daily with events via UTM campaign matching
	query := `
		WITH campaign_traffic AS (
			SELECT
				e.utm_campaign,
				e.utm_source,
				e.utm_medium,
				COUNT(*) as visits,
				COUNT(DISTINCT e.visitor_hash) as visitors,
				COUNT(DISTINCT e.session_id) as sessions
			FROM events e
			WHERE ` + where + ` AND e.utm_campaign IS NOT NULL AND e.utm_campaign != ''
			GROUP BY e.utm_campaign, e.utm_source, e.utm_medium
		),
		campaign_spend AS (
			SELECT
				campaign_name,
				provider,
				SUM(cost_micros) as total_cost_micros,
				SUM(impressions) as total_impressions,
				SUM(clicks) as total_clicks
			FROM ad_spend_daily
			WHERE date >= ? AND date <= ?
			GROUP BY campaign_name, provider
		)
		SELECT
			COALESCE(ct.utm_campaign, cs.campaign_name) as campaign,
			COALESCE(ct.utm_source, cs.provider) as source,
			ct.utm_medium as medium,
			COALESCE(ct.visits, 0) as visits,
			COALESCE(ct.visitors, 0) as visitors,
			COALESCE(ct.sessions, 0) as sessions,
			COALESCE(cs.total_cost_micros, 0) as cost_micros,
			COALESCE(cs.total_impressions, 0) as impressions,
			COALESCE(cs.total_clicks, 0) as ad_clicks
		FROM campaign_traffic ct
		FULL OUTER JOIN campaign_spend cs ON LOWER(ct.utm_campaign) = LOWER(cs.campaign_name)
		ORDER BY cost_micros DESC, visits DESC
	`

	args = append(args, startDate, endDate)

	rows, err := h.db.Conn().QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query attribution")
		return
	}
	defer rows.Close()

	type AttributionRow struct {
		Campaign    string  `json:"campaign"`
		Source      string  `json:"source"`
		Medium      string  `json:"medium"`
		Visits      int     `json:"visits"`
		Visitors    int     `json:"visitors"`
		Sessions    int     `json:"sessions"`
		Cost        float64 `json:"cost"`
		Impressions int     `json:"impressions"`
		AdClicks    int     `json:"ad_clicks"`
		CPC         float64 `json:"cpc"`
		CPA         float64 `json:"cpa"`
	}

	var result []AttributionRow
	for rows.Next() {
		var ar AttributionRow
		var costMicros int64
		var medium *string

		if err := rows.Scan(&ar.Campaign, &ar.Source, &medium, &ar.Visits, &ar.Visitors, &ar.Sessions, &costMicros, &ar.Impressions, &ar.AdClicks); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}

		ar.Cost = float64(costMicros) / 1_000_000
		if medium != nil {
			ar.Medium = *medium
		}
		if ar.AdClicks > 0 {
			ar.CPC = ar.Cost / float64(ar.AdClicks)
		}
		if ar.Visitors > 0 {
			ar.CPA = ar.Cost / float64(ar.Visitors)
		}

		result = append(result, ar)
	}
	if result == nil {
		result = []AttributionRow{}
	}

	writeJSON(w, http.StatusOK, result)
}

// GetProviders returns available ad platform providers
func (h *Handlers) GetProviders(w http.ResponseWriter, r *http.Request) {
	type ProviderInfo struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Available   bool   `json:"available"`
	}

	result := []ProviderInfo{
		{Name: "google_ads", DisplayName: "Google Ads", Available: true},
		{Name: "meta_ads", DisplayName: "Meta Ads", Available: true},
		{Name: "microsoft_ads", DisplayName: "Microsoft Ads", Available: true},
	}

	writeJSON(w, http.StatusOK, result)
}

// --- Google Ads Settings Handlers ---

// GetGoogleAdsSettings returns the current Google Ads settings with masked secrets
func (h *Handlers) GetGoogleAdsSettings(w http.ResponseWriter, r *http.Request) {
	svc := newSettingsService(h)

	clientID, _ := svc.Get("google_ads_client_id")
	clientSecret, _ := svc.Get("google_ads_client_secret")
	devToken, _ := svc.Get("google_ads_developer_token")

	maskedSecret := ""
	if clientSecret != "" {
		maskedSecret = "••••••••"
	}
	maskedToken := ""
	if devToken != "" {
		maskedToken = "••••••••"
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"google_ads_client_id":       clientID,
		"google_ads_client_secret":   maskedSecret,
		"google_ads_developer_token": maskedToken,
	})
}

// UpdateGoogleAdsSettings updates the Google Ads settings
func (h *Handlers) UpdateGoogleAdsSettings(w http.ResponseWriter, r *http.Request) {
	var input map[string]any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	svc := newSettingsService(h)

	allowedKeys := map[string]bool{
		"google_ads_client_id":       true,
		"google_ads_client_secret":   true,
		"google_ads_developer_token": true,
	}

	for key, val := range input {
		if !allowedKeys[key] {
			continue
		}
		strVal, ok := val.(string)
		if !ok {
			continue
		}
		svc.Set(key, strVal)
	}

	h.logAudit(r, "update", "settings", "google_ads", "Google Ads settings updated")
	w.WriteHeader(http.StatusNoContent)
}

// TestGoogleAdsSettings tests the Google Ads credentials
func (h *Handlers) TestGoogleAdsSettings(w http.ResponseWriter, r *http.Request) {
	svc := newSettingsService(h)

	clientID, _ := svc.Get("google_ads_client_id")
	clientSecret, _ := svc.Get("google_ads_client_secret")
	devToken, _ := svc.Get("google_ads_developer_token")

	if clientID == "" || clientSecret == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"message": "Client ID and Client Secret are required",
		})
		return
	}

	if devToken == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "Credentials are configured (developer token not set — required for API calls)",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "All Google Ads credentials are configured",
	})
}

// --- Meta Ads Settings Handlers ---

// GetMetaAdsSettings returns the current Meta Ads settings with masked secrets
func (h *Handlers) GetMetaAdsSettings(w http.ResponseWriter, r *http.Request) {
	svc := newSettingsService(h)

	appID, _ := svc.Get("meta_ads_app_id")
	appSecret, _ := svc.Get("meta_ads_app_secret")

	maskedSecret := ""
	if appSecret != "" {
		maskedSecret = "••••••••"
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"meta_ads_app_id":     appID,
		"meta_ads_app_secret": maskedSecret,
	})
}

// UpdateMetaAdsSettings updates the Meta Ads settings
func (h *Handlers) UpdateMetaAdsSettings(w http.ResponseWriter, r *http.Request) {
	var input map[string]any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	svc := newSettingsService(h)

	allowedKeys := map[string]bool{
		"meta_ads_app_id":     true,
		"meta_ads_app_secret": true,
	}

	for key, val := range input {
		if !allowedKeys[key] {
			continue
		}
		strVal, ok := val.(string)
		if !ok {
			continue
		}
		svc.Set(key, strVal)
	}

	h.logAudit(r, "update", "settings", "meta_ads", "Meta Ads settings updated")
	w.WriteHeader(http.StatusNoContent)
}

// TestMetaAdsSettings tests the Meta Ads credentials
func (h *Handlers) TestMetaAdsSettings(w http.ResponseWriter, r *http.Request) {
	svc := newSettingsService(h)

	appID, _ := svc.Get("meta_ads_app_id")
	appSecret, _ := svc.Get("meta_ads_app_secret")

	if appID == "" || appSecret == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"message": "App ID and App Secret are required",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "All Meta Ads credentials are configured",
	})
}

// --- Microsoft Ads Settings Handlers ---

// GetMicrosoftAdsSettings returns the current Microsoft Ads settings with masked secrets
func (h *Handlers) GetMicrosoftAdsSettings(w http.ResponseWriter, r *http.Request) {
	svc := newSettingsService(h)

	clientID, _ := svc.Get("microsoft_ads_client_id")
	clientSecret, _ := svc.Get("microsoft_ads_client_secret")
	devToken, _ := svc.Get("microsoft_ads_developer_token")

	maskedSecret := ""
	if clientSecret != "" {
		maskedSecret = "••••••••"
	}
	maskedToken := ""
	if devToken != "" {
		maskedToken = "••••••••"
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"microsoft_ads_client_id":       clientID,
		"microsoft_ads_client_secret":   maskedSecret,
		"microsoft_ads_developer_token": maskedToken,
	})
}

// UpdateMicrosoftAdsSettings updates the Microsoft Ads settings
func (h *Handlers) UpdateMicrosoftAdsSettings(w http.ResponseWriter, r *http.Request) {
	var input map[string]any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	svc := newSettingsService(h)

	allowedKeys := map[string]bool{
		"microsoft_ads_client_id":       true,
		"microsoft_ads_client_secret":   true,
		"microsoft_ads_developer_token": true,
	}

	for key, val := range input {
		if !allowedKeys[key] {
			continue
		}
		strVal, ok := val.(string)
		if !ok {
			continue
		}
		svc.Set(key, strVal)
	}

	h.logAudit(r, "update", "settings", "microsoft_ads", "Microsoft Ads settings updated")
	w.WriteHeader(http.StatusNoContent)
}

// TestMicrosoftAdsSettings tests the Microsoft Ads credentials
func (h *Handlers) TestMicrosoftAdsSettings(w http.ResponseWriter, r *http.Request) {
	svc := newSettingsService(h)

	clientID, _ := svc.Get("microsoft_ads_client_id")
	clientSecret, _ := svc.Get("microsoft_ads_client_secret")
	devToken, _ := svc.Get("microsoft_ads_developer_token")

	if clientID == "" || clientSecret == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"message": "Client ID and Client Secret are required",
		})
		return
	}

	if devToken == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "Credentials are configured (developer token not set — required for API calls)",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "All Microsoft Ads credentials are configured",
	})
}
