package api

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

// GetStatsCompare returns period-over-period comparison data in a single response.
func (h *Handlers) GetStatsCompare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseStatsFilter(r)

	// Parse comparison period or default to previous period
	cf := f.prevPeriod()
	if cs, ce := r.URL.Query().Get("compare_start"), r.URL.Query().Get("compare_end"); cs != "" && ce != "" {
		if st, err := time.Parse(time.RFC3339, cs); err == nil {
			if et, err := time.Parse(time.RFC3339, ce); err == nil {
				cf.startMs = st.UTC().UnixMilli()
				cf.endMs = et.UTC().UnixMilli()
			}
		}
	}

	type kv = map[string]interface{}
	type kvs = []map[string]interface{}

	var (
		curOv, prevOv           kv
		curTs, prevTs           kvs
		curPg, prevPg           kvs
		curRef, prevRef         kvs
		curGeo, prevGeo         kvs
		curDev, prevDev         kvs
		curBr, prevBr           kvs
		curCam, prevCam         kvs
		curEv, prevEv           kvs
		curOut, prevOut         kvs
	)

	var wg sync.WaitGroup
	wg.Add(20)
	run := func(fn func()) { go func() { defer wg.Done(); fn() }() }

	run(func() { curOv = h.queryOverviewStats(ctx, f) })
	run(func() { prevOv = h.queryOverviewStats(ctx, cf) })
	run(func() { curTs = h.compareTimeseries(ctx, f) })
	run(func() { prevTs = h.compareTimeseries(ctx, cf) })
	run(func() { curPg = h.comparePages(ctx, f) })
	run(func() { prevPg = h.comparePages(ctx, cf) })
	run(func() { curRef = h.compareReferrers(ctx, f) })
	run(func() { prevRef = h.compareReferrers(ctx, cf) })
	run(func() { curGeo = h.compareGeo(ctx, f) })
	run(func() { prevGeo = h.compareGeo(ctx, cf) })
	run(func() { curDev = h.compareDevices(ctx, f) })
	run(func() { prevDev = h.compareDevices(ctx, cf) })
	run(func() { curBr = h.compareBrowsers(ctx, f) })
	run(func() { prevBr = h.compareBrowsers(ctx, cf) })
	run(func() { curCam = h.compareCampaigns(ctx, f) })
	run(func() { prevCam = h.compareCampaigns(ctx, cf) })
	run(func() { curEv = h.compareEvents(ctx, f) })
	run(func() { prevEv = h.compareEvents(ctx, cf) })
	run(func() { curOut = h.compareOutbound(ctx, f) })
	run(func() { prevOut = h.compareOutbound(ctx, cf) })

	wg.Wait()

	insights := buildInsights(curOv, prevOv, curPg, prevPg, curRef, prevRef, curGeo, prevGeo)

	writeJSON(w, http.StatusOK, kv{
		"current_period": kv{"start": f.startMs, "end": f.endMs},
		"compare_period": kv{"start": cf.startMs, "end": cf.endMs},
		"overview":       kv{"current": curOv, "previous": prevOv},
		"timeseries":     kv{"current": curTs, "previous": prevTs},
		"pages":          kv{"current": curPg, "previous": prevPg},
		"referrers":      kv{"current": curRef, "previous": prevRef},
		"geo":            kv{"current": curGeo, "previous": prevGeo},
		"devices":        kv{"current": curDev, "previous": prevDev},
		"browsers":       kv{"current": curBr, "previous": prevBr},
		"campaigns":      kv{"current": curCam, "previous": prevCam},
		"events":         kv{"current": curEv, "previous": prevEv},
		"outbound":       kv{"current": curOut, "previous": prevOut},
		"insights":       insights,
	})
}

func (h *Handlers) compareTimeseries(ctx context.Context, f statsFilter) []map[string]interface{} {
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
		return nil
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var period string
		var pageviews, visitors int64
		rows.Scan(&period, &pageviews, &visitors)
		result = append(result, map[string]interface{}{
			"day_index": len(result),
			"date":      period,
			"pageviews": pageviews,
			"visitors":  visitors,
		})
	}
	return result
}

func (h *Handlers) comparePages(ctx context.Context, f statsFilter) []map[string]interface{} {
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'pageview'", f.startMs, f.endMs)
	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT path, COUNT(*) as views, COUNT(DISTINCT visitor_hash) as visitors
		FROM events WHERE `+where+`
		GROUP BY path ORDER BY views DESC LIMIT 20
	`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var path string
		var views, visitors int64
		rows.Scan(&path, &views, &visitors)
		result = append(result, map[string]interface{}{"path": path, "views": views, "visitors": visitors})
	}
	return result
}

func (h *Handlers) compareReferrers(ctx context.Context, f statsFilter) []map[string]interface{} {
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'pageview'", f.startMs, f.endMs)
	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT
			CASE WHEN referrer_url IS NULL OR referrer_url = '' THEN 'Direct / None'
			ELSE replace(regexp_extract(referrer_url, '://([^/]+)', 1), 'www.', '') END as source,
			COUNT(*) as visits, COUNT(DISTINCT visitor_hash) as visitors
		FROM events WHERE `+where+`
		GROUP BY source ORDER BY visits DESC LIMIT 20
	`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var source string
		var visits, visitors int64
		rows.Scan(&source, &visits, &visitors)
		result = append(result, map[string]interface{}{"source": source, "visits": visits, "visitors": visitors})
	}
	return result
}

func (h *Handlers) compareGeo(ctx context.Context, f statsFilter) []map[string]interface{} {
	where, args := f.where("timestamp >= ? AND timestamp <= ?", f.startMs, f.endMs)
	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT COALESCE(geo_country, 'Unknown') as country, COUNT(DISTINCT visitor_hash) as visitors
		FROM events WHERE `+where+`
		GROUP BY geo_country ORDER BY visitors DESC LIMIT 20
	`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var country string
		var visitors int64
		rows.Scan(&country, &visitors)
		result = append(result, map[string]interface{}{"country": country, "visitors": visitors})
	}
	return result
}

func (h *Handlers) compareDevices(ctx context.Context, f statsFilter) []map[string]interface{} {
	where, args := f.where("timestamp >= ? AND timestamp <= ?", f.startMs, f.endMs)
	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT COALESCE(device_type, 'Unknown') as device, COUNT(DISTINCT visitor_hash) as visitors
		FROM events WHERE `+where+`
		GROUP BY device_type ORDER BY visitors DESC
	`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var device string
		var visitors int64
		rows.Scan(&device, &visitors)
		result = append(result, map[string]interface{}{"device": device, "visitors": visitors})
	}
	return result
}

func (h *Handlers) compareBrowsers(ctx context.Context, f statsFilter) []map[string]interface{} {
	where, args := f.where("timestamp >= ? AND timestamp <= ?", f.startMs, f.endMs)
	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT COALESCE(browser_name, 'Unknown') as browser, COUNT(DISTINCT visitor_hash) as visitors
		FROM events WHERE `+where+`
		GROUP BY browser_name ORDER BY visitors DESC LIMIT 10
	`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var browser string
		var visitors int64
		rows.Scan(&browser, &visitors)
		result = append(result, map[string]interface{}{"browser": browser, "visitors": visitors})
	}
	return result
}

func (h *Handlers) compareCampaigns(ctx context.Context, f statsFilter) []map[string]interface{} {
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'pageview'", f.startMs, f.endMs)
	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT
			COALESCE(utm_source, '(direct)') as source,
			COALESCE(utm_medium, '(none)') as medium,
			COALESCE(utm_campaign, '(none)') as campaign,
			COUNT(*) as sessions, COUNT(DISTINCT visitor_hash) as visitors
		FROM events WHERE `+where+`
		GROUP BY utm_source, utm_medium, utm_campaign ORDER BY sessions DESC LIMIT 20
	`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var source, medium, campaign string
		var sessions, visitors int64
		rows.Scan(&source, &medium, &campaign, &sessions, &visitors)
		result = append(result, map[string]interface{}{
			"utm_source": source, "utm_medium": medium, "utm_campaign": campaign,
			"sessions": sessions, "visitors": visitors,
		})
	}
	return result
}

func (h *Handlers) compareEvents(ctx context.Context, f statsFilter) []map[string]interface{} {
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'custom'", f.startMs, f.endMs)
	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT event_name, COUNT(*) as count, COUNT(DISTINCT visitor_hash) as unique_visitors
		FROM events WHERE `+where+`
		GROUP BY event_name ORDER BY count DESC LIMIT 20
	`, args...)
	if err != nil {
		return nil
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
		result = append(result, map[string]interface{}{"event_name": eventName, "count": count, "unique_visitors": visitors})
	}
	return result
}

func (h *Handlers) compareOutbound(ctx context.Context, f statsFilter) []map[string]interface{} {
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'click' AND event_name = 'outbound'", f.startMs, f.endMs)
	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT json_extract_string(props, '$.target') as target, COUNT(*) as clicks, COUNT(DISTINCT visitor_hash) as unique_visitors
		FROM events WHERE `+where+`
		GROUP BY target ORDER BY clicks DESC LIMIT 20
	`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var target *string
		var clicks, visitors int64
		rows.Scan(&target, &clicks, &visitors)
		url := "(unknown)"
		if target != nil {
			url = *target
		}
		result = append(result, map[string]interface{}{"url": url, "clicks": clicks, "unique_visitors": visitors})
	}
	return result
}

// buildInsights generates auto-insights from comparison data.
func buildInsights(
	curOv, prevOv map[string]interface{},
	curPages, prevPages []map[string]interface{},
	curRefs, prevRefs []map[string]interface{},
	curGeo, prevGeo []map[string]interface{},
) []map[string]interface{} {
	type candidate struct {
		typ    string
		metric string
		text   string
		score  float64
	}

	var candidates []candidate

	// Overview KPI deltas
	kpis := []struct {
		key    string
		label  string
		invert bool // true = lower is better (e.g. bounce rate)
	}{
		{"total_events", "Total events", false},
		{"unique_visitors", "Unique visitors", false},
		{"pageviews", "Pageviews", false},
		{"sessions", "Sessions", false},
		{"bounce_rate", "Bounce rate", true},
		{"avg_session_seconds", "Avg session duration", false},
	}

	for _, kpi := range kpis {
		cur := toFloat(curOv[kpi.key])
		prev := toFloat(prevOv[kpi.key])
		if prev == 0 && cur == 0 {
			continue
		}
		var pct float64
		if prev != 0 {
			pct = ((cur - prev) / prev) * 100
		}
		if math.Abs(pct) < 15 {
			continue
		}
		positive := pct > 0
		if kpi.invert {
			positive = !positive
		}
		typ := "positive"
		if !positive {
			typ = "negative"
		}
		direction := "increased"
		if pct < 0 {
			direction = "decreased"
		}
		candidates = append(candidates, candidate{
			typ:    typ,
			metric: "overview",
			text:   fmt.Sprintf("%s %s %.1f%% (%.0f -> %.0f)", kpi.label, direction, math.Abs(pct), prev, cur),
			score:  math.Abs(pct) * math.Sqrt(math.Max(cur, prev)),
		})
	}

	// Dimension movers (pages, referrers, geo)
	dimInsights := func(metric, keyField, valField string, cur, prev []map[string]interface{}) {
		prevMap := make(map[string]float64)
		for _, item := range prev {
			k, _ := item[keyField].(string)
			prevMap[k] = toFloat(item[valField])
		}
		for _, item := range cur {
			k, _ := item[keyField].(string)
			curVal := toFloat(item[valField])
			prevVal, existed := prevMap[k]
			if !existed {
				candidates = append(candidates, candidate{
					typ: "neutral", metric: metric,
					text:  fmt.Sprintf("%s is new in this period (%.0f %s)", k, curVal, valField),
					score: curVal,
				})
				continue
			}
			if prevVal == 0 {
				continue
			}
			pct := ((curVal - prevVal) / prevVal) * 100
			if math.Abs(pct) < 20 {
				continue
			}
			typ := "positive"
			if pct < 0 {
				typ = "negative"
			}
			candidates = append(candidates, candidate{
				typ: typ, metric: metric,
				text:  fmt.Sprintf("%s changed %+.1f%% (%.0f -> %.0f %s)", k, pct, prevVal, curVal, valField),
				score: math.Abs(pct) * math.Sqrt(curVal),
			})
		}
	}

	dimInsights("pages", "path", "views", curPages, prevPages)
	dimInsights("referrers", "source", "visits", curRefs, prevRefs)
	dimInsights("geo", "country", "visitors", curGeo, prevGeo)

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}

	result := make([]map[string]interface{}, len(candidates))
	for i, c := range candidates {
		result[i] = map[string]interface{}{"type": c.typ, "metric": c.metric, "text": c.text}
	}
	return result
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}
