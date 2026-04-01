package api

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/bot"
	"github.com/caioricciuti/etiquetta/internal/buffer"
	"github.com/caioricciuti/etiquetta/internal/config"
	"github.com/caioricciuti/etiquetta/internal/connections"
	"github.com/caioricciuti/etiquetta/internal/database"
	"github.com/caioricciuti/etiquetta/internal/enrichment"
	"github.com/caioricciuti/etiquetta/internal/identification"
	"github.com/caioricciuti/etiquetta/internal/licensing"
	"github.com/caioricciuti/etiquetta/internal/migrate"
	"github.com/caioricciuti/etiquetta/internal/settings"
)

// Version is set from main.go at startup
var Version = "dev"

type Handlers struct {
	db             *database.DB
	enricher       *enrichment.Enricher
	licenseManager *licensing.Manager
	idGen          *identification.Generator
	cfg            *config.Config
	auth           *auth.Auth
	bufferMgr      *buffer.BufferManager
	connStore      *connections.Store
	syncManager    *connections.SyncManager
	migrateManager *migrate.JobManager

	// SSE subscribers
	sseClients map[chan []byte]bool
	sseMu      sync.RWMutex
}

// logAudit records an admin action to the audit log (fire-and-forget)
func (h *Handlers) logAudit(r *http.Request, action, resourceType, resourceID, detail string) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		return
	}

	clientIP := enrichment.ExtractClientIP(r.RemoteAddr, map[string]string{
		"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
		"X-Real-IP":       r.Header.Get("X-Real-IP"),
	})

	entry := &database.AuditLogEntry{
		ID:           generateID(),
		Timestamp:    time.Now().UnixMilli(),
		UserID:       claims.UserID,
		UserEmail:    claims.Email,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Detail:       detail,
		IPAddress:    clientIP,
	}

	if err := h.db.InsertAuditLog(entry); err != nil {
		fmt.Printf("[audit] Failed to log %s %s: %v\n", action, resourceType, err)
	}
}

// getTrackingBool reads a boolean tracking setting from the DB, falling back to the config value.
func (h *Handlers) getTrackingBool(key string, fallback bool) bool {
	var val string
	err := h.db.Conn().QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return fallback
	}
	return val == "true" || val == "1"
}

// Health check
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetVersion returns the current version
func (h *Handlers) GetVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": Version})
}

// ServeTrackerScript serves the JavaScript tracker
func (h *Handlers) ServeTrackerScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=86400")

	// Read embedded tracker script
	script, err := trackerJS.ReadFile("tracker.js")
	if err != nil {
		http.Error(w, "Script not found", http.StatusNotFound)
		return
	}

	// Global defaults from DB settings, falling back to config file
	trackPerf := h.getTrackingBool("track_performance", h.cfg.TrackPerformance) && h.licenseManager.HasFeature(licensing.FeaturePerformance)
	trackErrors := h.getTrackingBool("track_errors", h.cfg.TrackErrors) && h.licenseManager.HasFeature(licensing.FeatureErrorTracking)
	respectDNT := h.getTrackingBool("respect_dnt", h.cfg.RespectDNT)
	trackingMode := "cookieless"

	// If site_id provided, apply per-domain overrides
	if siteID := r.URL.Query().Get("id"); siteID != "" {
		var domainID string
		err := h.db.Conn().QueryRow("SELECT id FROM domains WHERE site_id = ? AND is_active = 1", siteID).Scan(&domainID)
		if err == nil && domainID != "" {
			svc := settings.New(h.db.Conn())
			if v := svc.GetForDomainWithDefault(domainID, "tracking_mode", "cookieless"); v == "cookie" || v == "localStorage" {
				trackingMode = v
			}
			if v, err := svc.GetForDomain(domainID, "track_performance"); err == nil && v != "" {
				trackPerf = (v == "true" || v == "1") && h.licenseManager.HasFeature(licensing.FeaturePerformance)
			}
			if v, err := svc.GetForDomain(domainID, "track_errors"); err == nil && v != "" {
				trackErrors = (v == "true" || v == "1") && h.licenseManager.HasFeature(licensing.FeatureErrorTracking)
			}
			if v, err := svc.GetForDomain(domainID, "respect_dnt"); err == nil && v != "" {
				respectDNT = v == "true" || v == "1"
			}
		}
	}

	config := fmt.Sprintf(`window.__ETIQUETTA_CONFIG__={endpoint:"%s",trackPerformance:%t,trackErrors:%t,respectDNT:%t,trackingMode:"%s"};`,
		"/i", trackPerf, trackErrors, respectDNT, trackingMode,
	)

	w.Write([]byte(config))
	w.Write(script)
}

// Ingest receives tracking events
func (h *Handlers) Ingest(w http.ResponseWriter, r *http.Request) {
	// Respect DNT (Do Not Track) and GPC (Global Privacy Control) headers
	if h.getTrackingBool("respect_dnt", h.cfg.RespectDNT) {
		if r.Header.Get("DNT") == "1" || r.Header.Get("Sec-GPC") == "1" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	// Parse events (NDJSON format - one event per line, possibly encoded)
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to read body")
		return
	}

	// Decode payload based on Content-Type
	contentType := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(contentType, "application/octet-stream"):
		// Deflate-compressed payload from CompressionStream
		reader := flate.NewReader(bytes.NewReader(body))
		decoded, decErr := io.ReadAll(io.LimitReader(reader, 1<<20))
		reader.Close()
		if decErr != nil {
			writeError(w, http.StatusBadRequest, "Failed to decompress body")
			return
		}
		body = decoded
	case strings.HasPrefix(contentType, "text/x-etq"):
		// XOR + base64 encoded payload (fallback for older browsers)
		decoded, decErr := base64.StdEncoding.DecodeString(string(body))
		if decErr != nil {
			writeError(w, http.StatusBadRequest, "Failed to decode body")
			return
		}
		xorKey := []byte{0x4e, 0x71, 0x63, 0x58}
		for i := range decoded {
			decoded[i] ^= xorKey[i%len(xorKey)]
		}
		body = decoded
	}
	// default: raw NDJSON (text/plain) — existing behavior, no decoding needed

	// Get Origin/Referer for domain validation
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	var requestHost string
	if origin != "" {
		if parsedOrigin, err := url.Parse(origin); err == nil {
			requestHost = parsedOrigin.Host
		}
	}

	// Get client info for enrichment
	clientIP := enrichment.ExtractClientIP(r.RemoteAddr, map[string]string{
		"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
		"X-Real-IP":       r.Header.Get("X-Real-IP"),
	})
	userAgent := r.Header.Get("User-Agent")

	// Collect headers for bot detection
	headers := map[string]string{
		"Accept-Language": r.Header.Get("Accept-Language"),
		"Accept-Encoding": r.Header.Get("Accept-Encoding"),
		"Accept":          r.Header.Get("Accept"),
	}

	// Enrich with geo, device, bot detection
	enriched := h.enricher.EnrichWithHeaders(clientIP, userAgent, "", headers)

	// Generate IP hash for tracking (privacy-preserving)
	ipHash := hashIP(clientIP)

	// Parse each line as a separate event
	var events []*database.Event
	var perfs []*database.Performance
	var errs []*database.Error

	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		// Validate site_id and domain match
		siteID, _ := raw["site_id"].(string)
		if siteID == "" {
			// No site_id provided - reject unless we have no domains registered (backwards compat)
			var domainCount int
			h.db.Conn().QueryRow("SELECT COUNT(*) FROM domains").Scan(&domainCount)
			if domainCount > 0 {
				continue // Skip events without site_id when domains are configured
			}
		} else {
			// Validate site_id exists and matches the request origin
			var registeredDomain string
			err := h.db.Conn().QueryRow("SELECT domain FROM domains WHERE site_id = ? AND is_active = 1", siteID).Scan(&registeredDomain)
			if err != nil {
				continue // Invalid or inactive site_id
			}

			// Verify the request origin matches the registered domain
			// Allow localhost for development
			if requestHost != "" && requestHost != registeredDomain {
				// Check if it's localhost/127.0.0.1 (development mode)
				if !strings.HasPrefix(requestHost, "localhost") && !strings.HasPrefix(requestHost, "127.0.0.1") {
					continue // Origin doesn't match registered domain
				}
			}
		}

		// Extract visitor hash for session ID generation (differentiates users sharing IP+UA)
		visitorHash, _ := raw["visitor_hash"].(string)
		sessionID := h.idGen.GenerateSessionID(clientIP, userAgent, visitorHash)

		eventType, _ := raw["type"].(string)

		switch eventType {
		case "performance":
			if !h.licenseManager.HasFeature(licensing.FeaturePerformance) {
				continue
			}
			perf := h.parsePerformance(raw, sessionID, enriched, enriched.BotScore, enriched.BotCategory)
			if perf != nil {
				perfs = append(perfs, perf)
			}

		case "error":
			if !h.licenseManager.HasFeature(licensing.FeatureErrorTracking) {
				continue
			}
			errEvent := h.parseError(raw, sessionID, enriched)
			if errEvent != nil {
				errs = append(errs, errEvent)
			}

		default:
			event := h.parseEvent(raw, sessionID, enriched, userAgent, ipHash, headers)
			if event != nil {
				events = append(events, event)
			}
		}
	}

	// Buffer events for batch loading via parquet
	ctx := r.Context()
	for _, e := range events {
		h.bufferMgr.AddEvent(ctx, buffer.ConvertEvent(e))
	}
	for _, p := range perfs {
		h.bufferMgr.AddPerformance(ctx, buffer.ConvertPerformance(p))
	}
	for _, e := range errs {
		h.bufferMgr.AddError(ctx, buffer.ConvertError(e))
	}

	// Notify SSE clients
	h.notifyClients(events, perfs, errs)

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) parseEvent(raw map[string]interface{}, sessionID string, enriched *enrichment.EnrichmentResult, userAgent string, ipHash string, headers map[string]string) *database.Event {
	urlStr, _ := raw["url"].(string)
	parsedURL, _ := url.Parse(urlStr)

	visitorHash, _ := raw["visitor_hash"].(string)
	if !identification.ValidateClientFingerprint(visitorHash) {
		// Use server-generated fallback
		visitorHash = h.idGen.GenerateVisitorHash("", userAgent)
	}

	// Extract client-side bot signals if provided
	var clientSignals *bot.ClientSignals
	if botSignalsRaw, ok := raw["bot_signals"].(map[string]interface{}); ok {
		clientSignals = &bot.ClientSignals{
			Webdriver:       getBoolFromFloat(botSignalsRaw, "webdriver"),
			Phantom:         getBoolFromFloat(botSignalsRaw, "phantom"),
			Selenium:        getBoolFromFloat(botSignalsRaw, "selenium"),
			Headless:        getBoolFromFloat(botSignalsRaw, "headless"),
			ScreenValid:     getBoolFromFloat(botSignalsRaw, "screen_valid"),
			Plugins:         int(getFloatOr(botSignalsRaw, "plugins", 0)),
			Languages:       int(getFloatOr(botSignalsRaw, "languages", 0)),
			ScreenWidth:     int(getFloatOr(botSignalsRaw, "screen_width", 0)),
			ScreenHeight:    int(getFloatOr(botSignalsRaw, "screen_height", 0)),
			CDPDetected:     getBoolFromFloat(botSignalsRaw, "cdp_detected"),
			DocHiddenAtLoad: getBoolFromFloat(botSignalsRaw, "doc_hidden_at_load"),
		}
	}

	// Recalculate bot score with client signals
	botResult := enriched.BotScore
	botCategory := enriched.BotCategory
	botSignals := enriched.BotSignals

	// Don't recalculate for known AI crawlers or good bots — server-side
	// UA detection already classified them correctly. Client signals would
	// overwrite the category (e.g., GPTBot running JS gets reclassified as bad_bot).
	if clientSignals != nil && botCategory != bot.CategoryAICrawler && botCategory != bot.CategoryGoodBot {
		// Merge server and client bot detection, preserving header checks
		result := bot.CalculateScore(userAgent, clientSignals, enriched.DatacenterIP, enriched.AICrawlerIP, enriched.AICrawlerIPName, headers)
		botResult = result.Score
		botCategory = result.Category
		botSignals = bot.SignalsToJSON(result.Signals)
	}

	// Check for suspicious path patterns (attack scanners, exploit probes)
	if pathSignal := bot.ScoreSuspiciousPath(parsedURL.Path); pathSignal != nil {
		botResult += pathSignal.Weight
		if botResult > 100 {
			botResult = 100
		}
		botCategory = bot.ScoreToCategory(botResult)
		// Re-serialize signals with the path signal added
		var signals []bot.Signal
		json.Unmarshal([]byte(botSignals), &signals)
		signals = append(signals, *pathSignal)
		botSignals = bot.SignalsToJSON(signals)
	}

	// Set geo coordinates if available
	var geoLat, geoLon *float64
	if enriched.GeoLatitude != 0 {
		geoLat = &enriched.GeoLatitude
	}
	if enriched.GeoLongitude != 0 {
		geoLon = &enriched.GeoLongitude
	}

	event := &database.Event{
		ID:           generateID(),
		Timestamp:    time.Now(),
		EventType:    getStringOr(raw, "event_type", "pageview"),
		SessionID:    sessionID,
		VisitorHash:  visitorHash,
		Domain:       parsedURL.Host,
		URL:          urlStr,
		Path:         parsedURL.Path,
		GeoCountry:   &enriched.GeoCountry,
		GeoCity:      &enriched.GeoCity,
		GeoRegion:    &enriched.GeoRegion,
		GeoLatitude:  geoLat,
		GeoLongitude: geoLon,
		BrowserName:  &enriched.BrowserName,
		OSName:       &enriched.OSName,
		DeviceType:   &enriched.DeviceType,
		IsBot:        botResult > 50,

		// Bot detection fields
		BotScore:     botResult,
		BotCategory:  botCategory,
		BotSignals:   botSignals,
		DatacenterIP: enriched.DatacenterIP,
		IPHash:       &ipHash,
	}

	// Extract behavioral flags from client
	event.HasScroll = getBoolFromFloat(raw, "has_scroll")
	event.HasMouseMove = getBoolFromFloat(raw, "has_mouse_move")
	event.HasClick = getBoolFromFloat(raw, "has_click")
	event.HasTouch = getBoolFromFloat(raw, "has_touch")

	// Extract click coordinates
	if clickX, ok := raw["click_x"].(float64); ok {
		x := int(clickX)
		event.ClickX = &x
	}
	if clickY, ok := raw["click_y"].(float64); ok {
		y := int(clickY)
		event.ClickY = &y
	}

	// Extract page duration
	if duration, ok := raw["page_duration"].(float64); ok {
		d := int(duration)
		event.PageDuration = &d
	}

	if title, ok := raw["page_title"].(string); ok {
		event.PageTitle = &title
	}
	if name, ok := raw["event_name"].(string); ok {
		event.EventName = &name
	}
	if ref, ok := raw["referrer_url"].(string); ok && ref != "" {
		event.ReferrerURL = &ref
		refType := enrichment.ClassifyReferrer(ref)
		event.ReferrerType = &refType
	}
	if utm, ok := raw["utm_source"].(string); ok {
		event.UTMSource = &utm
	}
	if utm, ok := raw["utm_medium"].(string); ok {
		event.UTMMedium = &utm
	}
	if utm, ok := raw["utm_campaign"].(string); ok {
		event.UTMCampaign = &utm
	}
	// Handle props - tracker sends as JSON string, but could also be a map
	if propsStr, ok := raw["props"].(string); ok && propsStr != "" {
		event.Props = json.RawMessage(propsStr)
	} else if propsMap, ok := raw["props"].(map[string]interface{}); ok {
		propsJSON, _ := json.Marshal(propsMap)
		event.Props = propsJSON
	}

	return event
}

func (h *Handlers) parsePerformance(raw map[string]interface{}, sessionID string, enriched *enrichment.EnrichmentResult, botScore int, botCategory string) *database.Performance {
	urlStr, _ := raw["url"].(string)
	parsedURL, _ := url.Parse(urlStr)

	if botCategory == "" {
		botCategory = "human"
	}

	perf := &database.Performance{
		ID:          generateID(),
		Timestamp:   time.Now(),
		SessionID:   sessionID,
		VisitorHash: getStringOr(raw, "visitor_hash", ""),
		Domain:      parsedURL.Host,
		URL:         urlStr,
		Path:        parsedURL.Path,
		DeviceType:  &enriched.DeviceType,
		GeoCountry:  &enriched.GeoCountry,
		BotScore:    botScore,
		BotCategory: botCategory,
	}

	if v, ok := raw["lcp"].(float64); ok {
		perf.LCP = &v
	}
	if v, ok := raw["cls"].(float64); ok {
		perf.CLS = &v
	}
	if v, ok := raw["fcp"].(float64); ok {
		perf.FCP = &v
	}
	if v, ok := raw["ttfb"].(float64); ok {
		perf.TTFB = &v
	}
	if v, ok := raw["inp"].(float64); ok {
		perf.INP = &v
	}
	if v, ok := raw["page_load_time"].(float64); ok {
		perf.PageLoadTime = &v
	}
	if v, ok := raw["connection_type"].(string); ok {
		perf.ConnectionType = &v
	}

	return perf
}

func (h *Handlers) parseError(raw map[string]interface{}, sessionID string, enriched *enrichment.EnrichmentResult) *database.Error {
	urlStr, _ := raw["url"].(string)
	parsedURL, _ := url.Parse(urlStr)

	errorType := getStringOr(raw, "error_type", "javascript")
	errorMessage := getStringOr(raw, "message", "Unknown error")
	scriptURL := getStringOr(raw, "script_url", "")
	lineNumber := int(getFloatOr(raw, "line_number", 0))

	errEvent := &database.Error{
		ID:           generateID(),
		Timestamp:    time.Now(),
		SessionID:    sessionID,
		VisitorHash:  getStringOr(raw, "visitor_hash", ""),
		Domain:       parsedURL.Host,
		URL:          urlStr,
		Path:         parsedURL.Path,
		ErrorType:    errorType,
		ErrorMessage: errorMessage,
		ErrorHash:    enrichment.HashError(errorType, errorMessage, scriptURL, lineNumber),
		BrowserName:  &enriched.BrowserName,
		GeoCountry:   &enriched.GeoCountry,
	}

	if v, ok := raw["stack"].(string); ok {
		errEvent.ErrorStack = &v
	}
	if scriptURL != "" {
		errEvent.ScriptURL = &scriptURL
	}
	if lineNumber > 0 {
		errEvent.LineNumber = &lineNumber
	}
	if v, ok := raw["column_number"].(float64); ok {
		col := int(v)
		errEvent.ColumnNumber = &col
	}

	return errEvent
}

// License handlers
func (h *Handlers) GetLicense(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.licenseManager.GetInfo())
}

func (h *Handlers) UploadLicense(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to read body")
		return
	}

	if err := h.licenseManager.SaveLicense(body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.logAudit(r, "upload", "license", "", "License uploaded")
	writeJSON(w, http.StatusOK, h.licenseManager.GetInfo())
}

func (h *Handlers) RemoveLicense(w http.ResponseWriter, r *http.Request) {
	if err := h.licenseManager.RemoveLicense(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.logAudit(r, "remove", "license", "", "License removed")
	writeJSON(w, http.StatusOK, h.licenseManager.GetInfo())
}

// Settings handlers
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Conn().Query("SELECT key, value FROM settings")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		rows.Scan(&key, &value)
		settings[key] = value
	}

	writeJSON(w, http.StatusOK, settings)
}

func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]string
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	tx, _ := h.db.Conn().Begin()
	changedKeys := make([]string, 0, len(settings))
	for key, value := range settings {
		tx.Exec("INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at",
			key, value, time.Now().UnixMilli())
		changedKeys = append(changedKeys, key)
	}
	tx.Commit()

	h.logAudit(r, "update", "settings", "", "Updated keys: "+strings.Join(changedKeys, ", "))
	w.WriteHeader(http.StatusNoContent)
}

// Database access
func (h *Handlers) ServeDatabase(w http.ResponseWriter, r *http.Request) {
	dbPath := h.cfg.DataDir + "/etiquetta.duckdb"

	// Check if client wants partial content (Range request)
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		w.Header().Set("Accept-Ranges", "bytes")
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache")

	http.ServeFile(w, r, dbPath)
}

func (h *Handlers) GetDatabaseInfo(w http.ResponseWriter, r *http.Request) {
	dbPath := h.cfg.DataDir + "/etiquetta.duckdb"
	info, err := os.Stat(dbPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "Database not found")
		return
	}

	eventCount, _ := h.db.GetEventCount()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"size_bytes":  info.Size(),
		"modified_at": info.ModTime(),
		"event_count": eventCount,
		"engine":      "duckdb",
	})
}

// ExplorerQuery executes a read-only SQL query (admin only)
func (h *Handlers) ExplorerQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "Query is required")
		return
	}

	result, err := h.db.ExecuteExplorerQuery(req.Query)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// ExplorerSchema returns the database schema for autocomplete
func (h *Handlers) ExplorerSchema(w http.ResponseWriter, r *http.Request) {
	schema, err := h.db.GetTableSchema()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, schema)
}

// SSE for real-time events
func (h *Handlers) EventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Create client channel
	client := make(chan []byte, 100)

	h.sseMu.Lock()
	if h.sseClients == nil {
		h.sseClients = make(map[chan []byte]bool)
	}
	h.sseClients[client] = true
	h.sseMu.Unlock()

	defer func() {
		h.sseMu.Lock()
		delete(h.sseClients, client)
		h.sseMu.Unlock()
		close(client)
	}()

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	// Listen for events with keepalive to prevent WriteTimeout
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case msg := <-client:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (h *Handlers) notifyClients(events []*database.Event, perfs []*database.Performance, errs []*database.Error) {
	h.sseMu.RLock()
	defer h.sseMu.RUnlock()

	if len(h.sseClients) == 0 {
		return
	}

	// Build notification
	notification := map[string]interface{}{
		"type":        "batch",
		"events":      len(events),
		"performance": len(perfs),
		"errors":      len(errs),
		"timestamp":   time.Now().UnixMilli(),
	}

	// Add last event details
	if len(events) > 0 {
		last := events[len(events)-1]
		notification["last_event"] = map[string]interface{}{
			"type":    last.EventType,
			"path":    last.Path,
			"country": last.GeoCountry,
		}
	}

	data, _ := json.Marshal(notification)

	for client := range h.sseClients {
		select {
		case client <- data:
		default:
			// Client buffer full, skip
		}
	}
}
