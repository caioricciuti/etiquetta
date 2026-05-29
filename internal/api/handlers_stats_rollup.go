package api

import (
	"context"
)

// rollupEligible reports whether a stats filter can be served EXACTLY from
// rollup_daily. Rollups are keyed by (date_key, domain, bot_category) and carry
// no per-dimension columns, so the fast path only applies when:
//   - the rollup feature flag is on,
//   - no per-dimension filter is active (country/browser/device/page/referrer),
//   - the bot filter maps to exactly one bot_category bucket.
//
// The default ("") and "bots"/"all" filters use is_bot semantics that don't map
// 1:1 to a single category bucket, so they deliberately fall through to raw
// events to keep numbers byte-identical to the pre-rollup behavior.
func (h *Handlers) rollupEligible(f statsFilter) (category string, ok bool) {
	if !h.useRollups {
		return "", false
	}
	if f.country != "" || f.browser != "" || f.device != "" || f.page != "" || f.referrer != "" {
		return "", false
	}
	switch f.botFilter {
	case "humans":
		return "human", true
	case "good_bots":
		return "good_bot", true
	case "bad_bots":
		return "bad_bot", true
	case "suspicious":
		return "suspicious", true
	case "automation":
		return "automation", true
	case "ai_crawlers":
		return "ai_crawler", true
	default:
		// "", "all", "bots" -> is_bot-based semantics; not a single category.
		return "", false
	}
}

// rollupTimeseries serves per-day pageviews+visitors from rollup_daily. Both are
// exact per day for a single bot_category bucket. Returns ok=false on any error
// so the caller can fall back to the raw-events query.
func (h *Handlers) rollupTimeseries(ctx context.Context, f statsFilter, category string) ([]map[string]interface{}, bool) {
	startKey := msToDateKey(f.startMs)
	endKey := msToDateKey(f.endMs)

	args := []interface{}{startKey, endKey, category}
	q := `
		SELECT date_key AS period, SUM(pageviews) AS pageviews, SUM(visitors) AS visitors
		FROM rollup_daily
		WHERE date_key >= ? AND date_key <= ? AND bot_category = ?`
	if f.domain != "" {
		q += " AND domain = ?"
		args = append(args, f.domain)
	}
	q += " GROUP BY date_key ORDER BY date_key"

	rows, err := h.db.Conn().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var period string
		var pageviews, visitors int64
		if err := rows.Scan(&period, &pageviews, &visitors); err != nil {
			return nil, false
		}
		result = append(result, map[string]interface{}{
			"period":    period,
			"pageviews": pageviews,
			"visitors":  visitors,
		})
	}
	return result, true
}
