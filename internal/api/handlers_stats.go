package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// statsFilter holds all filter parameters for stat queries
type statsFilter struct {
	startMs   int64
	endMs     int64
	domain    string
	country   string
	browser   string
	device    string
	page      string
	referrer  string
	botFilter string // "all", "humans", "good_bots", "bad_bots", "suspicious", or "" (default = exclude bots)
}

// parseStatsFilter extracts filter params from request
func parseStatsFilter(r *http.Request) statsFilter {
	f := statsFilter{}
	f.startMs, f.endMs = getDateRangeParams(r, 7)
	f.domain = r.URL.Query().Get("domain")
	f.country = r.URL.Query().Get("country")
	f.browser = r.URL.Query().Get("browser")
	f.device = r.URL.Query().Get("device")
	f.page = r.URL.Query().Get("page")
	f.referrer = r.URL.Query().Get("referrer")
	f.botFilter = r.URL.Query().Get("bot_filter")
	return f
}

// where builds a WHERE clause from a base condition plus all active filters.
// Bot filtering is automatically applied (default: exclude bots).
func (f statsFilter) where(base string, baseArgs ...interface{}) (string, []interface{}) {
	where := base
	args := append([]interface{}{}, baseArgs...)

	// Bot filtering — replaces hardcoded "is_bot = 0" in all callers
	where += " AND " + getBotFilterCondition(f.botFilter)

	if f.domain != "" {
		where += " AND domain = ?"
		args = append(args, f.domain)
	}
	if f.country != "" {
		where += " AND geo_country = ?"
		args = append(args, f.country)
	}
	if f.browser != "" {
		where += " AND browser_name = ?"
		args = append(args, f.browser)
	}
	if f.device != "" {
		where += " AND device_type = ?"
		args = append(args, f.device)
	}
	if f.page != "" {
		where += " AND path = ?"
		args = append(args, f.page)
	}
	if f.referrer != "" {
		where += " AND referrer_url LIKE ?"
		args = append(args, "%"+f.referrer+"%")
	}
	return where, args
}

// prevPeriod returns a filter shifted back by the same duration
func (f statsFilter) prevPeriod() statsFilter {
	duration := f.endMs - f.startMs
	prev := f
	prev.startMs = f.startMs - duration
	prev.endMs = f.startMs
	return prev
}

// queryOverviewStats fetches overview stats for a given filter
func (h *Handlers) queryOverviewStats(ctx context.Context, f statsFilter) map[string]interface{} {
	var totalEvents, uniqueVisitors, sessions, pageviews int64
	var bounceRate, avgDuration float64

	w1, a1 := f.where("timestamp >= ? AND timestamp <= ?", f.startMs, f.endMs)
	h.db.Conn().QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE "+w1, a1...).Scan(&totalEvents)
	h.db.Conn().QueryRowContext(ctx, "SELECT COUNT(DISTINCT visitor_hash) FROM events WHERE "+w1, a1...).Scan(&uniqueVisitors)
	h.db.Conn().QueryRowContext(ctx, "SELECT COUNT(DISTINCT session_id) FROM events WHERE "+w1, a1...).Scan(&sessions)

	w2, a2 := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'pageview'", f.startMs, f.endMs)
	h.db.Conn().QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE "+w2, a2...).Scan(&pageviews)

	h.db.Conn().QueryRowContext(ctx, `
		SELECT COALESCE(
			CAST(SUM(CASE WHEN pv_count = 1 THEN 1 ELSE 0 END) AS DOUBLE) / NULLIF(COUNT(*), 0) * 100,
			0
		) FROM (
			SELECT session_id, COUNT(*) as pv_count
			FROM events
			WHERE `+w2+`
			GROUP BY session_id
		)
	`, a2...).Scan(&bounceRate)

	w3, a3 := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'engagement'", f.startMs, f.endMs)
	h.db.Conn().QueryRowContext(ctx, `
		SELECT COALESCE(AVG(
			CAST(json_extract_string(props, '$.visible_time_ms') AS INTEGER)
		), 0) / 1000.0
		FROM events
		WHERE `+w3,
		a3...).Scan(&avgDuration)

	return map[string]interface{}{
		"total_events":        totalEvents,
		"unique_visitors":     uniqueVisitors,
		"sessions":            sessions,
		"pageviews":           pageviews,
		"bounce_rate":         bounceRate,
		"avg_session_seconds": avgDuration,
	}
}

// GetStatsOverview returns main dashboard stats with period comparison
func (h *Handlers) GetStatsOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	live := time.Now().Add(-5 * time.Minute).UnixMilli()

	// Current period stats
	result := h.queryOverviewStats(ctx, f)

	// Live visitors (not affected by filters other than domain)
	var liveVisitors int64
	liveWhere, liveArgs := f.where("timestamp >= ?", live)
	h.db.Conn().QueryRowContext(ctx, "SELECT COUNT(DISTINCT session_id) FROM events WHERE "+liveWhere, liveArgs...).Scan(&liveVisitors)
	result["live_visitors"] = liveVisitors

	// Previous period comparison
	pf := f.prevPeriod()
	prev := h.queryOverviewStats(ctx, pf)
	result["prev_total_events"] = prev["total_events"]
	result["prev_unique_visitors"] = prev["unique_visitors"]
	result["prev_sessions"] = prev["sessions"]
	result["prev_pageviews"] = prev["pageviews"]
	result["prev_bounce_rate"] = prev["bounce_rate"]
	result["prev_avg_session_seconds"] = prev["avg_session_seconds"]

	writeJSON(w, http.StatusOK, result)
}

// GetStatsTimeseries returns traffic over time
func (h *Handlers) GetStatsTimeseries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'pageview'", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT
			strftime('%Y-%m-%d', to_timestamp(timestamp / 1000)::TIMESTAMP) as period,
			COUNT(*) as pageviews,
			COUNT(DISTINCT visitor_hash) as visitors
		FROM events
		WHERE `+where+`
		GROUP BY period
		ORDER BY period
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var period string
		var pageviews, visitors int64
		rows.Scan(&period, &pageviews, &visitors)
		result = append(result, map[string]interface{}{
			"period":    period,
			"pageviews": pageviews,
			"visitors":  visitors,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsPages returns top pages
func (h *Handlers) GetStatsPages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'pageview'", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT path, COUNT(*) as views, COUNT(DISTINCT visitor_hash) as visitors
		FROM events
		WHERE `+where+`
		GROUP BY path
		ORDER BY views DESC
		LIMIT 10
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var path string
		var views, visitors int64
		rows.Scan(&path, &views, &visitors)
		result = append(result, map[string]interface{}{
			"path":     path,
			"views":    views,
			"visitors": visitors,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsReferrers returns traffic sources with actual domains
func (h *Handlers) GetStatsReferrers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'pageview'", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT
			CASE
				WHEN referrer_url IS NULL OR referrer_url = '' THEN 'Direct / None'
				ELSE replace(regexp_extract(referrer_url, '://([^/]+)', 1), 'www.', '')
			END as source,
			ANY_VALUE(COALESCE(NULLIF(referrer_type, ''), 'direct')) as referrer_type,
			COUNT(*) as visits,
			COUNT(DISTINCT visitor_hash) as visitors
		FROM events
		WHERE `+where+`
		GROUP BY source
		ORDER BY visits DESC
		LIMIT 20
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var source, refType string
		var visits, visitors int64
		rows.Scan(&source, &refType, &visits, &visitors)
		result = append(result, map[string]interface{}{
			"source":        source,
			"referrer_type": refType,
			"visits":        visits,
			"visitors":      visitors,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsGeo returns geographic distribution
func (h *Handlers) GetStatsGeo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ?", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT COALESCE(geo_country, 'Unknown') as country, COUNT(DISTINCT visitor_hash) as visitors
		FROM events
		WHERE `+where+`
		GROUP BY geo_country
		ORDER BY visitors DESC
		LIMIT 20
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var country string
		var visitors int64
		rows.Scan(&country, &visitors)
		result = append(result, map[string]interface{}{
			"country":  country,
			"visitors": visitors,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsMapData returns geographic data with coordinates for map visualization
func (h *Handlers) GetStatsMapData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND geo_latitude IS NOT NULL AND geo_latitude != 0", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT
			geo_city,
			geo_country,
			geo_latitude,
			geo_longitude,
			COUNT(DISTINCT visitor_hash) as visitors,
			COUNT(*) as pageviews
		FROM events
		WHERE `+where+`
		GROUP BY geo_city, geo_country, geo_latitude, geo_longitude
		ORDER BY visitors DESC
		LIMIT 500
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var city, country sql.NullString
		var lat, lng float64
		var visitors, pageviews int64
		rows.Scan(&city, &country, &lat, &lng, &visitors, &pageviews)
		result = append(result, map[string]interface{}{
			"city":      city.String,
			"country":   country.String,
			"lat":       lat,
			"lng":       lng,
			"visitors":  visitors,
			"pageviews": pageviews,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsDevices returns device breakdown
func (h *Handlers) GetStatsDevices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ?", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT COALESCE(device_type, 'Unknown') as device, COUNT(DISTINCT visitor_hash) as visitors
		FROM events
		WHERE `+where+`
		GROUP BY device_type
		ORDER BY visitors DESC
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var device string
		var visitors int64
		rows.Scan(&device, &visitors)
		result = append(result, map[string]interface{}{
			"device":   device,
			"visitors": visitors,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsBrowsers returns browser breakdown
func (h *Handlers) GetStatsBrowsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ?", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT COALESCE(browser_name, 'Unknown') as browser, COUNT(DISTINCT visitor_hash) as visitors
		FROM events
		WHERE `+where+`
		GROUP BY browser_name
		ORDER BY visitors DESC
		LIMIT 10
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var browser string
		var visitors int64
		rows.Scan(&browser, &visitors)
		result = append(result, map[string]interface{}{
			"browser":  browser,
			"visitors": visitors,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsCampaigns returns UTM campaign breakdown
func (h *Handlers) GetStatsCampaigns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'pageview'", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT
			COALESCE(utm_source, '(direct)') as source,
			COALESCE(utm_medium, '(none)') as medium,
			COALESCE(utm_campaign, '(none)') as campaign,
			COUNT(*) as visits,
			COUNT(DISTINCT visitor_hash) as visitors
		FROM events
		WHERE `+where+`
		GROUP BY utm_source, utm_medium, utm_campaign
		ORDER BY visits DESC
		LIMIT 20
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var source, medium, campaign string
		var visits, visitors int64
		rows.Scan(&source, &medium, &campaign, &visits, &visitors)
		result = append(result, map[string]interface{}{
			"utm_source":   source,
			"utm_medium":   medium,
			"utm_campaign": campaign,
			"sessions":     visits,
			"visitors":     visitors,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsCustomEvents returns custom event breakdown
func (h *Handlers) GetStatsCustomEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'custom'", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT
			event_name,
			COUNT(*) as count,
			COUNT(DISTINCT visitor_hash) as visitors
		FROM events
		WHERE `+where+`
		GROUP BY event_name
		ORDER BY count DESC
		LIMIT 20
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var name *string
		var count, visitors int64
		rows.Scan(&name, &count, &visitors)
		eventName := "(unnamed)"
		if name != nil {
			eventName = *name
		}
		result = append(result, map[string]interface{}{
			"event_name":      eventName,
			"count":           count,
			"unique_visitors": visitors,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsOutbound returns outbound link clicks
func (h *Handlers) GetStatsOutbound(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'click' AND event_name = 'outbound'", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT
			json_extract_string(props, '$.target') as target,
			COUNT(*) as clicks,
			COUNT(DISTINCT visitor_hash) as visitors
		FROM events
		WHERE `+where+`
		GROUP BY target
		ORDER BY clicks DESC
		LIMIT 20
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var target *string
		var clicks, visitors int64
		rows.Scan(&target, &clicks, &visitors)
		targetURL := "(unknown)"
		if target != nil {
			targetURL = *target
		}
		result = append(result, map[string]interface{}{
			"url":             targetURL,
			"clicks":          clicks,
			"unique_visitors": visitors,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsBots returns bot traffic breakdown (intentionally shows ALL traffic including bots)
func (h *Handlers) GetStatsBots(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	startMs, endMs := getDateRangeParams(r, 7)
	domain := getDomainParam(r)

	// Category distribution
	var categoryRows *sql.Rows
	var err error
	if domain != "" {
		categoryRows, err = h.db.Conn().QueryContext(ctx, `
			SELECT bot_category, COUNT(*) as count, COUNT(DISTINCT visitor_hash) as visitors
			FROM events
			WHERE timestamp >= ? AND timestamp <= ? AND domain = ?
			GROUP BY bot_category
		`, startMs, endMs, domain)
	} else {
		categoryRows, err = h.db.Conn().QueryContext(ctx, `
			SELECT bot_category, COUNT(*) as count, COUNT(DISTINCT visitor_hash) as visitors
			FROM events
			WHERE timestamp >= ? AND timestamp <= ?
			GROUP BY bot_category
		`, startMs, endMs)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	categories := make([]map[string]interface{}, 0)
	for categoryRows.Next() {
		var category string
		var count, visitors int64
		categoryRows.Scan(&category, &count, &visitors)
		categories = append(categories, map[string]interface{}{
			"category": category,
			"events":   count,
			"visitors": visitors,
		})
	}
	categoryRows.Close()

	// Score distribution (histogram)
	var scoreRows *sql.Rows
	if domain != "" {
		scoreRows, err = h.db.Conn().QueryContext(ctx, `
			SELECT
				CASE
					WHEN bot_score <= 10 THEN '0-10'
					WHEN bot_score <= 20 THEN '11-20'
					WHEN bot_score <= 30 THEN '21-30'
					WHEN bot_score <= 40 THEN '31-40'
					WHEN bot_score <= 50 THEN '41-50'
					WHEN bot_score <= 60 THEN '51-60'
					WHEN bot_score <= 70 THEN '61-70'
					WHEN bot_score <= 80 THEN '71-80'
					WHEN bot_score <= 90 THEN '81-90'
					ELSE '91-100'
				END as score_range,
				COUNT(*) as count
			FROM events
			WHERE timestamp >= ? AND timestamp <= ? AND domain = ?
			GROUP BY score_range
			ORDER BY score_range
		`, startMs, endMs, domain)
	} else {
		scoreRows, err = h.db.Conn().QueryContext(ctx, `
			SELECT
				CASE
					WHEN bot_score <= 10 THEN '0-10'
					WHEN bot_score <= 20 THEN '11-20'
					WHEN bot_score <= 30 THEN '21-30'
					WHEN bot_score <= 40 THEN '31-40'
					WHEN bot_score <= 50 THEN '41-50'
					WHEN bot_score <= 60 THEN '51-60'
					WHEN bot_score <= 70 THEN '61-70'
					WHEN bot_score <= 80 THEN '71-80'
					WHEN bot_score <= 90 THEN '81-90'
					ELSE '91-100'
				END as score_range,
				COUNT(*) as count
			FROM events
			WHERE timestamp >= ? AND timestamp <= ?
			GROUP BY score_range
			ORDER BY score_range
		`, startMs, endMs)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	scoreDistribution := make([]map[string]interface{}, 0)
	for scoreRows.Next() {
		var scoreRange string
		var count int64
		scoreRows.Scan(&scoreRange, &count)
		scoreDistribution = append(scoreDistribution, map[string]interface{}{
			"range": scoreRange,
			"count": count,
		})
	}
	scoreRows.Close()

	// Bot traffic over time
	var timeRows *sql.Rows
	if domain != "" {
		timeRows, err = h.db.Conn().QueryContext(ctx, `
			SELECT
				strftime('%Y-%m-%d', to_timestamp(timestamp / 1000)::TIMESTAMP) as period,
				SUM(CASE WHEN bot_category = 'human' THEN 1 ELSE 0 END) as humans,
				SUM(CASE WHEN bot_category = 'suspicious' THEN 1 ELSE 0 END) as suspicious,
				SUM(CASE WHEN bot_category = 'bad_bot' THEN 1 ELSE 0 END) as bad_bots,
				SUM(CASE WHEN bot_category = 'good_bot' THEN 1 ELSE 0 END) as good_bots
			FROM events
			WHERE timestamp >= ? AND timestamp <= ? AND domain = ?
			GROUP BY period
			ORDER BY period
		`, startMs, endMs, domain)
	} else {
		timeRows, err = h.db.Conn().QueryContext(ctx, `
			SELECT
				strftime('%Y-%m-%d', to_timestamp(timestamp / 1000)::TIMESTAMP) as period,
				SUM(CASE WHEN bot_category = 'human' THEN 1 ELSE 0 END) as humans,
				SUM(CASE WHEN bot_category = 'suspicious' THEN 1 ELSE 0 END) as suspicious,
				SUM(CASE WHEN bot_category = 'bad_bot' THEN 1 ELSE 0 END) as bad_bots,
				SUM(CASE WHEN bot_category = 'good_bot' THEN 1 ELSE 0 END) as good_bots
			FROM events
			WHERE timestamp >= ? AND timestamp <= ?
			GROUP BY period
			ORDER BY period
		`, startMs, endMs)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	timeseries := make([]map[string]interface{}, 0)
	for timeRows.Next() {
		var period string
		var humans, suspicious, badBots, goodBots int64
		timeRows.Scan(&period, &humans, &suspicious, &badBots, &goodBots)
		timeseries = append(timeseries, map[string]interface{}{
			"period":     period,
			"humans":     humans,
			"suspicious": suspicious,
			"bad_bots":   badBots,
			"good_bots":  goodBots,
		})
	}
	timeRows.Close()

	// Top bots detail list
	var botRows *sql.Rows
	if domain != "" {
		botRows, err = h.db.Conn().QueryContext(ctx, `
			SELECT
				COALESCE(browser_name, 'Unknown') as browser_name,
				bot_category,
				bot_score,
				ANY_VALUE(bot_signals) as bot_signals,
				COUNT(*) as hits,
				COUNT(DISTINCT visitor_hash) as visitors,
				COUNT(DISTINCT session_id) as sessions,
				MAX(timestamp) as last_seen
			FROM events
			WHERE timestamp >= ? AND timestamp <= ? AND domain = ? AND bot_category != 'human'
			GROUP BY browser_name, bot_category, bot_score
			ORDER BY hits DESC
			LIMIT 50
		`, startMs, endMs, domain)
	} else {
		botRows, err = h.db.Conn().QueryContext(ctx, `
			SELECT
				COALESCE(browser_name, 'Unknown') as browser_name,
				bot_category,
				bot_score,
				ANY_VALUE(bot_signals) as bot_signals,
				COUNT(*) as hits,
				COUNT(DISTINCT visitor_hash) as visitors,
				COUNT(DISTINCT session_id) as sessions,
				MAX(timestamp) as last_seen
			FROM events
			WHERE timestamp >= ? AND timestamp <= ? AND bot_category != 'human'
			GROUP BY browser_name, bot_category, bot_score
			ORDER BY hits DESC
			LIMIT 50
		`, startMs, endMs)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	topBots := make([]map[string]interface{}, 0)
	for botRows.Next() {
		var browserName, botCat, botSigs string
		var score int
		var hits, visitors, sessions, lastSeen int64
		botRows.Scan(&browserName, &botCat, &score, &botSigs, &hits, &visitors, &sessions, &lastSeen)

		// Extract signal names from JSON
		var rawSignals []struct {
			Name  string `json:"name"`
			Value string `json:"value,omitempty"`
		}
		signalNames := make([]string, 0)
		if json.Unmarshal([]byte(botSigs), &rawSignals) == nil {
			for _, s := range rawSignals {
				signalNames = append(signalNames, s.Name)
			}
		}

		topBots = append(topBots, map[string]interface{}{
			"browser_name": browserName,
			"category":     botCat,
			"score":        score,
			"signals":      signalNames,
			"hits":         hits,
			"visitors":     visitors,
			"sessions":     sessions,
			"last_seen":    lastSeen,
		})
	}
	botRows.Close()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"categories":         categories,
		"score_distribution": scoreDistribution,
		"timeseries":         timeseries,
		"top_bots":           topBots,
	})
}
