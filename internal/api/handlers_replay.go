package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/caioricciuti/etiquetta/internal/replay"
)

// replayStore is set during router creation
var replayStore *replay.Store

// IngestReplay receives rrweb event chunks from the recorder script.
// POST /r
func (h *Handlers) IngestReplay(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		SessionID   string             `json:"session_id"`
		Domain      string             `json:"domain"`
		VisitorHash string             `json:"visitor_hash"`
		Events      []json.RawMessage  `json:"events"`
		Meta        *replayMeta        `json:"meta,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	if payload.SessionID == "" || payload.Domain == "" || len(payload.Events) == 0 {
		http.Error(w, `{"error":"missing session_id, domain, or events"}`, http.StatusBadRequest)
		return
	}

	// Check if replay is enabled
	if !h.getTrackingBool("replay_enabled", false) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Check storage quota
	quotaMB := h.getTrackingInt("replay_storage_quota_mb", 5120)
	if quotaMB > 0 {
		used, _ := replayStore.DiskUsageBytes()
		if used > int64(quotaMB)*1024*1024 {
			w.WriteHeader(http.StatusNoContent) // silently drop — don't error on client
			return
		}
	}

	// Append events to disk
	sizeBytes, err := replayStore.AppendEvents(payload.Domain, payload.SessionID, payload.Events)
	if err != nil {
		fmt.Printf("[replay] Failed to append events: %v\n", err)
		http.Error(w, `{"error":"storage error"}`, http.StatusInternalServerError)
		return
	}

	// Upsert metadata in DB
	now := time.Now().UnixMilli()
	if payload.Meta != nil {
		h.db.Conn().Exec(`
			INSERT INTO session_recordings (session_id, domain, visitor_hash, start_time, first_url, device_type, browser_name, os_name, geo_country, screen_width, screen_height, size_bytes, events_count, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'recording', ?, ?)
			ON CONFLICT (session_id) DO UPDATE SET
				size_bytes = ?,
				events_count = session_recordings.events_count + ?,
				duration = ? - session_recordings.start_time,
				updated_at = ?
		`,
			payload.SessionID, payload.Domain, payload.VisitorHash, now, payload.Meta.URL,
			payload.Meta.DeviceType, payload.Meta.BrowserName, payload.Meta.OSName,
			payload.Meta.GeoCountry, payload.Meta.ScreenWidth, payload.Meta.ScreenHeight,
			sizeBytes, len(payload.Events), now, now,
			sizeBytes, len(payload.Events), now, now,
		)
	} else {
		h.db.Conn().Exec(`
			INSERT INTO session_recordings (session_id, domain, visitor_hash, start_time, size_bytes, events_count, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 'recording', ?, ?)
			ON CONFLICT (session_id) DO UPDATE SET
				size_bytes = ?,
				events_count = session_recordings.events_count + ?,
				duration = ? - session_recordings.start_time,
				updated_at = ?
		`,
			payload.SessionID, payload.Domain, payload.VisitorHash, now,
			sizeBytes, len(payload.Events), now, now,
			sizeBytes, len(payload.Events), now, now,
		)
	}

	w.WriteHeader(http.StatusNoContent)
}

type replayMeta struct {
	URL          string `json:"url"`
	DeviceType   string `json:"device_type"`
	BrowserName  string `json:"browser_name"`
	OSName       string `json:"os_name"`
	GeoCountry   string `json:"geo_country"`
	ScreenWidth  int    `json:"screen_width"`
	ScreenHeight int    `json:"screen_height"`
}

// ListReplays returns paginated session recordings.
// GET /api/replays
func (h *Handlers) ListReplays(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `SELECT session_id, domain, visitor_hash, start_time, duration, pages,
		first_url, device_type, browser_name, os_name, geo_country,
		screen_width, screen_height, size_bytes, events_count, status, created_at
		FROM session_recordings WHERE 1=1`
	var args []interface{}

	if domain != "" {
		query += " AND domain = ?"
		args = append(args, domain)
	}
	if from != "" {
		if ts, err := strconv.ParseInt(from, 10, 64); err == nil {
			query += " AND start_time >= ?"
			args = append(args, ts)
		}
	}
	if to != "" {
		if ts, err := strconv.ParseInt(to, 10, 64); err == nil {
			query += " AND start_time <= ?"
			args = append(args, ts)
		}
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM session_recordings WHERE 1=1"
	if domain != "" {
		countQuery += " AND domain = ?"
	}
	if from != "" {
		if _, err := strconv.ParseInt(from, 10, 64); err == nil {
			countQuery += " AND start_time >= ?"
		}
	}
	if to != "" {
		if _, err := strconv.ParseInt(to, 10, 64); err == nil {
			countQuery += " AND start_time <= ?"
		}
	}

	var total int
	h.db.Conn().QueryRow(countQuery, args...).Scan(&total)

	query += " ORDER BY start_time DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := h.db.Conn().Query(query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var recordings []sessionRecording
	for rows.Next() {
		var rec sessionRecording
		rows.Scan(
			&rec.SessionID, &rec.Domain, &rec.VisitorHash, &rec.StartTime,
			&rec.Duration, &rec.Pages, &rec.FirstURL, &rec.DeviceType,
			&rec.BrowserName, &rec.OSName, &rec.GeoCountry,
			&rec.ScreenWidth, &rec.ScreenHeight, &rec.SizeBytes,
			&rec.EventsCount, &rec.Status, &rec.CreatedAt,
		)
		recordings = append(recordings, rec)
	}

	if recordings == nil {
		recordings = []sessionRecording{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"recordings": recordings,
		"total":      total,
		"limit":      limit,
		"offset":     offset,
	})
}

type sessionRecording struct {
	SessionID   string `json:"session_id"`
	Domain      string `json:"domain"`
	VisitorHash string `json:"visitor_hash"`
	StartTime   int64  `json:"start_time"`
	Duration    int64  `json:"duration"`
	Pages       int    `json:"pages"`
	FirstURL    string `json:"first_url"`
	DeviceType  string `json:"device_type"`
	BrowserName string `json:"browser_name"`
	OSName      string `json:"os_name"`
	GeoCountry  string `json:"geo_country"`
	ScreenWidth int    `json:"screen_width"`
	ScreenHeight int   `json:"screen_height"`
	SizeBytes   int64  `json:"size_bytes"`
	EventsCount int    `json:"events_count"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"created_at"`
}

// GetReplay returns rrweb events for playback.
// GET /api/replays/{sessionId}
func (h *Handlers) GetReplay(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")

	// Look up domain from metadata
	var domain string
	err := h.db.Conn().QueryRow("SELECT domain FROM session_recordings WHERE session_id = ?", sessionID).Scan(&domain)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording not found"})
		return
	}

	events, err := replayStore.ReadEvents(domain, sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording data not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": sessionID,
		"events":     events,
	})
}

// DeleteReplay removes a session recording.
// DELETE /api/replays/{sessionId}
func (h *Handlers) DeleteReplay(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")

	var domain string
	err := h.db.Conn().QueryRow("SELECT domain FROM session_recordings WHERE session_id = ?", sessionID).Scan(&domain)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording not found"})
		return
	}

	replayStore.Delete(domain, sessionID)
	h.db.Conn().Exec("DELETE FROM session_recordings WHERE session_id = ?", sessionID)

	h.logAudit(r, "delete", "session_recording", sessionID, "Deleted session recording")
	w.WriteHeader(http.StatusNoContent)
}

// GetReplayStats returns storage usage info.
// GET /api/replays/stats
func (h *Handlers) GetReplayStats(w http.ResponseWriter, r *http.Request) {
	var totalRecordings int
	var totalSizeBytes int64
	h.db.Conn().QueryRow("SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM session_recordings").Scan(&totalRecordings, &totalSizeBytes)

	diskUsage, _ := replayStore.DiskUsageBytes()
	quotaMB := h.getTrackingInt("replay_storage_quota_mb", 5120)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_recordings": totalRecordings,
		"total_size_bytes": totalSizeBytes,
		"disk_usage_bytes": diskUsage,
		"quota_bytes":      int64(quotaMB) * 1024 * 1024,
		"quota_mb":         quotaMB,
	})
}

// GetReplaySettings returns current replay configuration.
// GET /api/replays/settings
func (h *Handlers) GetReplaySettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":          h.getTrackingBool("replay_enabled", false),
		"sample_rate":      h.getTrackingInt("replay_sample_rate", 10),
		"mask_text":        h.getTrackingBool("replay_mask_text", true),
		"mask_inputs":      h.getTrackingBool("replay_mask_inputs", true),
		"max_duration_sec": h.getTrackingInt("replay_max_duration_sec", 1800),
		"storage_quota_mb": h.getTrackingInt("replay_storage_quota_mb", 5120),
	})
}

// UpdateReplaySettings updates replay configuration.
// PUT /api/replays/settings
func (h *Handlers) UpdateReplaySettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled        *bool `json:"enabled"`
		SampleRate     *int  `json:"sample_rate"`
		MaskText       *bool `json:"mask_text"`
		MaskInputs     *bool `json:"mask_inputs"`
		MaxDurationSec *int  `json:"max_duration_sec"`
		StorageQuotaMB *int  `json:"storage_quota_mb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	now := time.Now().UnixMilli()

	if req.Enabled != nil {
		h.db.Conn().Exec("UPDATE settings SET value = ?, updated_at = ? WHERE key = 'replay_enabled'",
			fmt.Sprintf("%v", *req.Enabled), now)
	}
	if req.SampleRate != nil {
		rate := *req.SampleRate
		if rate < 0 {
			rate = 0
		}
		if rate > 100 {
			rate = 100
		}
		h.db.Conn().Exec("UPDATE settings SET value = ?, updated_at = ? WHERE key = 'replay_sample_rate'",
			strconv.Itoa(rate), now)
	}
	if req.MaskText != nil {
		h.db.Conn().Exec("UPDATE settings SET value = ?, updated_at = ? WHERE key = 'replay_mask_text'",
			fmt.Sprintf("%v", *req.MaskText), now)
	}
	if req.MaskInputs != nil {
		h.db.Conn().Exec("UPDATE settings SET value = ?, updated_at = ? WHERE key = 'replay_mask_inputs'",
			fmt.Sprintf("%v", *req.MaskInputs), now)
	}
	if req.MaxDurationSec != nil {
		h.db.Conn().Exec("UPDATE settings SET value = ?, updated_at = ? WHERE key = 'replay_max_duration_sec'",
			strconv.Itoa(*req.MaxDurationSec), now)
	}
	if req.StorageQuotaMB != nil {
		h.db.Conn().Exec("UPDATE settings SET value = ?, updated_at = ? WHERE key = 'replay_storage_quota_mb'",
			strconv.Itoa(*req.StorageQuotaMB), now)
	}

	h.logAudit(r, "update", "replay_settings", "", "Updated session replay settings")

	// Return updated settings
	h.GetReplaySettings(w, r)
}

// getTrackingInt reads an integer setting from the DB.
func (h *Handlers) getTrackingInt(key string, fallback int) int {
	var val string
	err := h.db.Conn().QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return fallback
	}
	v, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return v
}

// ServeRecorderScript serves the session recorder JavaScript.
// GET /r.js
func (h *Handlers) ServeRecorderScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=86400")

	script, err := recorderJS.ReadFile("recorder.js")
	if err != nil {
		http.Error(w, "Script not found", http.StatusNotFound)
		return
	}

	w.Write(script)
}

// ServeReplayConfig serves replay configuration for the tracker script.
// GET /r/config (public)
func (h *Handlers) ServeReplayConfig(w http.ResponseWriter, r *http.Request) {
	enabled := h.getTrackingBool("replay_enabled", false)
	if !enabled {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled": false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":          true,
		"sample_rate":      h.getTrackingInt("replay_sample_rate", 10),
		"mask_text":        h.getTrackingBool("replay_mask_text", true),
		"mask_inputs":      h.getTrackingBool("replay_mask_inputs", true),
		"max_duration_sec": h.getTrackingInt("replay_max_duration_sec", 1800),
	})
}
