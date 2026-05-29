package api

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

// Install debugger (idea #8): a read-only view over the existing events table so
// a user who just pasted the snippet can confirm data is flowing and see exactly
// what Etiquetta collects + how it enriches/classifies each hit. No schema or
// tracker changes; it only reads events.

// GetInstallStatus reports, for one domain: total events, events in the last
// 30 minutes, the most recent event time, and a sample of recent events with
// their enrichment (geo, device, bot classification). The UI polls this to flip
// from "waiting for first event" to "receiving data".
func (h *Handlers) GetInstallStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	domain := getDomainParam(r)
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain parameter required")
		return
	}

	conn := h.db.Conn()

	var total int64
	conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE domain = ?", domain).Scan(&total)

	recentCutoff := time.Now().Add(-30 * time.Minute).UnixMilli()
	var last30m int64
	conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE domain = ? AND timestamp >= ?", domain, recentCutoff).Scan(&last30m)

	var lastEventAt sql.NullInt64
	conn.QueryRowContext(ctx, "SELECT MAX(timestamp) FROM events WHERE domain = ?", domain).Scan(&lastEventAt)

	// Sample of the most recent events, showing the fields the tracker collects
	// and the enrichment the server adds.
	rows, err := conn.QueryContext(ctx, `
		SELECT
			timestamp,
			event_type,
			COALESCE(event_name, ''),
			COALESCE(path, ''),
			COALESCE(geo_country, ''),
			COALESCE(geo_city, ''),
			COALESCE(device_type, ''),
			COALESCE(browser_name, ''),
			COALESCE(os_name, ''),
			COALESCE(referrer_type, ''),
			COALESCE(bot_category, 'human'),
			COALESCE(bot_score, 0),
			COALESCE(is_bot, 0),
			COALESCE(has_click, 0),
			COALESCE(has_scroll, 0)
		FROM events
		WHERE domain = ?
		ORDER BY timestamp DESC
		LIMIT 25
	`, domain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	recent := make([]map[string]interface{}, 0, 25)
	for rows.Next() {
		var ts, botScore, isBot, hasClick, hasScroll int64
		var etype, ename, path, country, city, device, browser, os, refType, botCat string
		if err := rows.Scan(&ts, &etype, &ename, &path, &country, &city, &device, &browser, &os, &refType, &botCat, &botScore, &isBot, &hasClick, &hasScroll); err != nil {
			continue
		}
		recent = append(recent, map[string]interface{}{
			"timestamp":     ts,
			"event_type":    etype,
			"event_name":    ename,
			"path":          path,
			"geo_country":   country,
			"geo_city":      city,
			"device_type":   device,
			"browser_name":  browser,
			"os_name":       os,
			"referrer_type": refType,
			"bot_category":  botCat,
			"bot_score":     botScore,
			"is_bot":        isBot == 1,
			"has_click":     hasClick == 1,
			"has_scroll":    hasScroll == 1,
		})
	}

	resp := map[string]interface{}{
		"domain":          domain,
		"total_events":    total,
		"events_last_30m": last30m,
		"installed":       total > 0,
		"receiving":       last30m > 0,
		"recent":          recent,
	}
	if lastEventAt.Valid {
		resp["last_event_at"] = lastEventAt.Int64
	} else {
		resp["last_event_at"] = nil
	}

	writeJSON(w, http.StatusOK, resp)
}
