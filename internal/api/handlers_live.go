package api

import (
	"context"
	"net/http"
	"time"
)

// Live view (idea #6): a real-time snapshot of who's on the site right now.
// Read-only over the events table, scoped by domain + bot filter (default
// excludes bots, same as every other stats endpoint). The UI polls this on a
// short interval; the existing SSE stream (EventStream) signals when new data
// has landed so the poll can refresh promptly.

// GetStatsLive returns a real-time snapshot: current visitors, active pages,
// live traffic sources, a per-minute pageview sparkline, and recent events.
func (h *Handlers) GetStatsLive(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	domain := getDomainParam(r)
	botFilter := r.URL.Query().Get("bot_filter")

	now := time.Now()
	// "Current" window: visitors active in the last 5 minutes.
	currentMs := now.Add(-5 * time.Minute).UnixMilli()
	// Sparkline window: last 30 minutes.
	sparkMs := now.Add(-30 * time.Minute).UnixMilli()

	// Build a filter that reuses the shared bot + domain handling.
	f := statsFilter{startMs: currentMs, endMs: now.UnixMilli(), domain: domain, botFilter: botFilter}

	conn := h.db.Conn()

	// Current visitors + active sessions (last 5 min).
	var currentVisitors, activeSessions, pageviews5m int64
	w1, a1 := f.where("timestamp >= ?", currentMs)
	conn.QueryRowContext(ctx, "SELECT COUNT(DISTINCT visitor_hash) FROM events WHERE "+w1, a1...).Scan(&currentVisitors)
	conn.QueryRowContext(ctx, "SELECT COUNT(DISTINCT session_id) FROM events WHERE "+w1, a1...).Scan(&activeSessions)
	w1pv, a1pv := f.where("timestamp >= ? AND event_type = 'pageview'", currentMs)
	conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE "+w1pv, a1pv...).Scan(&pageviews5m)

	// Active pages right now (last 5 min, by visitors).
	activePages := make([]map[string]interface{}, 0)
	wp, ap := f.where("timestamp >= ? AND event_type = 'pageview'", currentMs)
	if rows, err := conn.QueryContext(ctx, `
		SELECT path, COUNT(DISTINCT visitor_hash) AS visitors, COUNT(*) AS views
		FROM events WHERE `+wp+`
		GROUP BY path ORDER BY visitors DESC, views DESC LIMIT 10
	`, ap...); err == nil {
		for rows.Next() {
			var path string
			var visitors, views int64
			rows.Scan(&path, &visitors, &views)
			activePages = append(activePages, map[string]interface{}{
				"path": path, "visitors": visitors, "views": views,
			})
		}
		rows.Close()
	}

	// Live traffic sources (last 5 min).
	liveSources := make([]map[string]interface{}, 0)
	ws, as := f.where("timestamp >= ? AND event_type = 'pageview'", currentMs)
	if rows, err := conn.QueryContext(ctx, `
		SELECT
			CASE
				WHEN referrer_url IS NULL OR referrer_url = '' THEN 'Direct / None'
				ELSE replace(regexp_extract(referrer_url, '://([^/]+)', 1), 'www.', '')
			END AS source,
			COUNT(DISTINCT visitor_hash) AS visitors
		FROM events WHERE `+ws+`
		GROUP BY source ORDER BY visitors DESC LIMIT 8
	`, as...); err == nil {
		for rows.Next() {
			var source string
			var visitors int64
			rows.Scan(&source, &visitors)
			liveSources = append(liveSources, map[string]interface{}{
				"source": source, "visitors": visitors,
			})
		}
		rows.Close()
	}

	// Top countries right now (last 5 min).
	liveCountries := make([]map[string]interface{}, 0)
	wc, ac := f.where("timestamp >= ?", currentMs)
	if rows, err := conn.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(geo_country, ''), 'Unknown') AS country, COUNT(DISTINCT visitor_hash) AS visitors
		FROM events WHERE `+wc+`
		GROUP BY country ORDER BY visitors DESC LIMIT 8
	`, ac...); err == nil {
		for rows.Next() {
			var country string
			var visitors int64
			rows.Scan(&country, &visitors)
			liveCountries = append(liveCountries, map[string]interface{}{
				"country": country, "visitors": visitors,
			})
		}
		rows.Close()
	}

	// Per-minute pageview sparkline for the last 30 minutes.
	sparkline := make([]map[string]interface{}, 0)
	fs := statsFilter{startMs: sparkMs, endMs: now.UnixMilli(), domain: domain, botFilter: botFilter}
	wsl, asl := fs.where("timestamp >= ? AND event_type = 'pageview'", sparkMs)
	if rows, err := conn.QueryContext(ctx, `
		SELECT strftime('%Y-%m-%d %H:%M', to_timestamp(timestamp / 1000)::TIMESTAMP) AS minute,
			COUNT(*) AS views, COUNT(DISTINCT visitor_hash) AS visitors
		FROM events WHERE `+wsl+`
		GROUP BY minute ORDER BY minute
	`, asl...); err == nil {
		for rows.Next() {
			var minute string
			var views, visitors int64
			rows.Scan(&minute, &views, &visitors)
			sparkline = append(sparkline, map[string]interface{}{
				"minute": minute, "views": views, "visitors": visitors,
			})
		}
		rows.Close()
	}

	// Most recent events (the live feed).
	recent := make([]map[string]interface{}, 0)
	wr, ar := f.where("timestamp >= ?", currentMs)
	if rows, err := conn.QueryContext(ctx, `
		SELECT timestamp, event_type, COALESCE(event_name, ''), COALESCE(path, ''),
			COALESCE(geo_country, ''), COALESCE(geo_city, ''), COALESCE(device_type, ''),
			COALESCE(browser_name, ''), COALESCE(referrer_type, '')
		FROM events WHERE `+wr+`
		ORDER BY timestamp DESC LIMIT 30
	`, ar...); err == nil {
		for rows.Next() {
			var ts int64
			var etype, ename, path, country, city, device, browser, refType string
			rows.Scan(&ts, &etype, &ename, &path, &country, &city, &device, &browser, &refType)
			recent = append(recent, map[string]interface{}{
				"timestamp": ts, "event_type": etype, "event_name": ename, "path": path,
				"geo_country": country, "geo_city": city, "device_type": device,
				"browser_name": browser, "referrer_type": refType,
			})
		}
		rows.Close()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current_visitors": currentVisitors,
		"active_sessions":  activeSessions,
		"pageviews_5m":     pageviews5m,
		"active_pages":     activePages,
		"live_sources":     liveSources,
		"live_countries":   liveCountries,
		"sparkline":        sparkline,
		"recent":           recent,
		"window_minutes":   5,
		"server_time":      now.UnixMilli(),
	})
}
