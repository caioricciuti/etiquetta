package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Heatmaps are built entirely from data Etiquetta already collects:
//   - click_x / click_y (viewport coords captured on pageviews with has_click = 1)
//   - scroll milestone events (event_type = 'scroll', event_name = 'scroll_25'..'scroll_100')
//   - engagement events carrying props.scroll_depth (max scroll % reached)
// No schema change or tracker change is required.

// GetStatsHeatmapPages lists pages that have interaction data, for the page selector.
// Ordered by pageviews so the busiest pages surface first.
func (h *Handlers) GetStatsHeatmapPages(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	f := parseStatsFilter(r)
	where, args := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'pageview'", f.startMs, f.endMs)

	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT
			path,
			COUNT(*) AS pageviews,
			SUM(CASE WHEN has_click = 1 THEN 1 ELSE 0 END) AS clicks
		FROM events
		WHERE `+where+`
		GROUP BY path
		ORDER BY pageviews DESC
		LIMIT 200
	`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var path string
		var pageviews, clicks int64
		rows.Scan(&path, &pageviews, &clicks)
		result = append(result, map[string]interface{}{
			"path":      path,
			"pageviews": pageviews,
			"clicks":    clicks,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatsHeatmapClicks returns click coordinates bucketed into a grid so the UI
// can render a density overlay. Coordinates are viewport-relative (clientX/clientY).
// Pass ?page= to scope to a single page (strongly recommended for a usable overlay).
func (h *Handlers) GetStatsHeatmapClicks(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	f := parseStatsFilter(r)

	// Grid bucket size in px (clamped). Smaller = finer-grained, more points.
	bucket := 16
	if b := r.URL.Query().Get("bucket"); b != "" {
		if n, err := strconv.Atoi(b); err == nil && n >= 4 && n <= 100 {
			bucket = n
		}
	}

	where, args := f.where("timestamp >= ? AND timestamp <= ? AND has_click = 1 AND click_x >= 0 AND click_y >= 0", f.startMs, f.endMs)

	// DuckDB uses // for integer (floor) division.
	query := fmt.Sprintf(`
		SELECT
			(click_x // %d) * %d AS bx,
			(click_y // %d) * %d AS by,
			COUNT(*) AS c
		FROM events
		WHERE %s
		GROUP BY bx, by
		ORDER BY c DESC
		LIMIT 5000
	`, bucket, bucket, bucket, bucket, where)

	rows, err := h.db.Conn().QueryContext(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	points := make([]map[string]interface{}, 0)
	var maxX, maxY, maxCount, total int64
	for rows.Next() {
		var bx, by, c int64
		rows.Scan(&bx, &by, &c)
		if bx > maxX {
			maxX = bx
		}
		if by > maxY {
			maxY = by
		}
		if c > maxCount {
			maxCount = c
		}
		total += c
		points = append(points, map[string]interface{}{
			"x":     bx,
			"y":     by,
			"count": c,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"points":       points,
		"max_x":        maxX,
		"max_y":        maxY,
		"max_count":    maxCount,
		"total_clicks": total,
		"bucket":       bucket,
	})
}

// GetStatsHeatmapScroll returns scroll-depth reach for a page: of all pageviews,
// what fraction scrolled to each milestone, plus the average max scroll depth.
func (h *Handlers) GetStatsHeatmapScroll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	f := parseStatsFilter(r)

	// Denominator: total pageviews matching the filter.
	pvWhere, pvArgs := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'pageview'", f.startMs, f.endMs)
	var pageviews int64
	h.db.Conn().QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE "+pvWhere, pvArgs...).Scan(&pageviews)

	// Scroll milestone events.
	scWhere, scArgs := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'scroll'", f.startMs, f.endMs)
	rows, err := h.db.Conn().QueryContext(ctx, `
		SELECT event_name, COUNT(*) AS c
		FROM events
		WHERE `+scWhere+`
		GROUP BY event_name
	`, scArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	reached := map[string]int64{}
	for rows.Next() {
		var name string
		var c int64
		rows.Scan(&name, &c)
		reached[name] = c
	}
	rows.Close()

	// Average max scroll depth from engagement events.
	enWhere, enArgs := f.where("timestamp >= ? AND timestamp <= ? AND event_type = 'engagement'", f.startMs, f.endMs)
	var avgDepth float64
	h.db.Conn().QueryRowContext(ctx, `
		SELECT COALESCE(AVG(TRY_CAST(json_extract_string(props, '$.scroll_depth') AS INTEGER)), 0)
		FROM events
		WHERE `+enWhere+` AND json_extract_string(props, '$.scroll_depth') IS NOT NULL
	`, enArgs...).Scan(&avgDepth)

	milestones := make([]map[string]interface{}, 0, 4)
	for _, m := range []int{25, 50, 75, 100} {
		c := reached[fmt.Sprintf("scroll_%d", m)]
		var pct float64
		if pageviews > 0 {
			pct = float64(c) / float64(pageviews) * 100
		}
		milestones = append(milestones, map[string]interface{}{
			"depth":   m,
			"reached": c,
			"pct":     pct,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pageviews":      pageviews,
		"avg_max_scroll": avgDepth,
		"milestones":     milestones,
	})
}
