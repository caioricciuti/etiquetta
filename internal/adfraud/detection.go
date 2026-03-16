package adfraud

import (
	"database/sql"
	"time"
)

// FraudSignal represents a detected fraud indicator
type FraudSignal struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Count       int64   `json:"count"`
	Severity    string  `json:"severity"` // low, medium, high
}

// FraudSummary contains overall fraud statistics
type FraudSummary struct {
	TotalClicks       int64         `json:"total_clicks"`
	BotClicks         int64         `json:"bot_clicks"`
	SuspiciousClicks  int64         `json:"suspicious_clicks"`
	HumanClicks       int64         `json:"human_clicks"`
	BotClickRate      float64       `json:"bot_click_rate"`
	Signals           []FraudSignal `json:"signals"`
	EstimatedWaste    float64       `json:"estimated_waste"`
}

// Detector handles fraud detection operations
type Detector struct {
	db *sql.DB
}

// NewDetector creates a new fraud detector
func NewDetector(db *sql.DB) *Detector {
	return &Detector{db: db}
}

// GetFraudSummary returns an overview of detected fraud
func (d *Detector) GetFraudSummary(domain string, days int) (*FraudSummary, error) {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()

	summary := &FraudSummary{
		Signals: make([]FraudSignal, 0),
	}

	// Get click counts by bot category
	query := `
		SELECT
			bot_category,
			COUNT(*) as clicks
		FROM events
		WHERE timestamp >= ?
			AND event_type = 'click'
	`
	args := []interface{}{cutoff}
	if domain != "" {
		query += " AND domain = ?"
		args = append(args, domain)
	}
	query += " GROUP BY bot_category"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var category string
		var clicks int64
		rows.Scan(&category, &clicks)

		summary.TotalClicks += clicks
		switch category {
		case "human":
			summary.HumanClicks = clicks
		case "bad_bot":
			summary.BotClicks += clicks
		case "suspicious":
			summary.SuspiciousClicks = clicks
		case "good_bot":
			// Good bots don't click on ads typically, count as bot
			summary.BotClicks += clicks
		}
	}

	if summary.TotalClicks > 0 {
		summary.BotClickRate = float64(summary.BotClicks+summary.SuspiciousClicks) / float64(summary.TotalClicks) * 100
	}

	// Detect specific fraud patterns
	summary.Signals = append(summary.Signals, d.detectClickWithoutImpression(domain, cutoff)...)
	summary.Signals = append(summary.Signals, d.detectCoordinateClustering(domain, cutoff)...)
	summary.Signals = append(summary.Signals, d.detectEngagementMismatch(domain, cutoff)...)

	// Calculate estimated waste from campaigns
	summary.EstimatedWaste = d.calculateWastedSpend(domain, cutoff)

	return summary, nil
}

// detectClickWithoutImpression finds clicks that don't have a prior pageview in the session
func (d *Detector) detectClickWithoutImpression(domain string, cutoff int64) []FraudSignal {
	query := `
		SELECT COUNT(DISTINCT e.session_id) as orphan_clicks
		FROM events e
		WHERE e.timestamp >= ?
			AND e.event_type = 'click'
			AND e.utm_source IS NOT NULL
			AND NOT EXISTS (
				SELECT 1 FROM events e2
				WHERE e2.session_id = e.session_id
					AND e2.event_type = 'pageview'
					AND e2.timestamp <= e.timestamp
			)
	`
	args := []interface{}{cutoff}
	if domain != "" {
		query = `
			SELECT COUNT(DISTINCT e.session_id) as orphan_clicks
			FROM events e
			WHERE e.timestamp >= ?
				AND e.event_type = 'click'
				AND e.utm_source IS NOT NULL
				AND e.domain = ?
				AND NOT EXISTS (
					SELECT 1 FROM events e2
					WHERE e2.session_id = e.session_id
						AND e2.event_type = 'pageview'
						AND e2.timestamp <= e.timestamp
				)
		`
		args = append(args, domain)
	}

	var count int64
	d.db.QueryRow(query, args...).Scan(&count)

	if count > 0 {
		return []FraudSignal{{
			Type:        "click_without_impression",
			Description: "Clicks from campaign traffic without prior page impression",
			Count:       count,
			Severity:    "high",
		}}
	}
	return nil
}

// detectCoordinateClustering finds suspiciously clustered click coordinates
func (d *Detector) detectCoordinateClustering(domain string, cutoff int64) []FraudSignal {
	// Look for >10% of clicks at the exact same coordinates
	query := `
		SELECT click_x, click_y, COUNT(*) as click_count,
			CAST(COUNT(*) AS DOUBLE) / (SELECT COUNT(*) FROM events WHERE timestamp >= ? AND event_type = 'click' AND click_x IS NOT NULL) * 100 as pct
		FROM events
		WHERE timestamp >= ?
			AND event_type = 'click'
			AND click_x IS NOT NULL
	`
	args := []interface{}{cutoff, cutoff}
	if domain != "" {
		query = `
			SELECT click_x, click_y, COUNT(*) as click_count,
				CAST(COUNT(*) AS DOUBLE) / (SELECT COUNT(*) FROM events WHERE timestamp >= ? AND event_type = 'click' AND click_x IS NOT NULL AND domain = ?) * 100 as pct
			FROM events
			WHERE timestamp >= ?
				AND event_type = 'click'
				AND click_x IS NOT NULL
				AND domain = ?
		`
		args = []interface{}{cutoff, domain, cutoff, domain}
	}
	query += " GROUP BY click_x, click_y HAVING pct > 10 LIMIT 5"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var signals []FraudSignal
	for rows.Next() {
		var x, y int
		var count int64
		var pct float64
		rows.Scan(&x, &y, &count, &pct)

		signals = append(signals, FraudSignal{
			Type:        "coordinate_clustering",
			Description: "High concentration of clicks at specific coordinates",
			Count:       count,
			Severity:    "medium",
		})
	}
	return signals
}

// detectEngagementMismatch finds sessions with clicks but no engagement
func (d *Detector) detectEngagementMismatch(domain string, cutoff int64) []FraudSignal {
	query := `
		SELECT COUNT(DISTINCT session_id) as count
		FROM events
		WHERE timestamp >= ?
			AND event_type = 'click'
			AND utm_source IS NOT NULL
			AND session_id NOT IN (
				SELECT session_id FROM events
				WHERE timestamp >= ? AND has_scroll = 1
				UNION
				SELECT session_id FROM events
				WHERE timestamp >= ? AND page_duration > 5000
			)
	`
	args := []interface{}{cutoff, cutoff, cutoff}
	if domain != "" {
		query = `
			SELECT COUNT(DISTINCT session_id) as count
			FROM events
			WHERE timestamp >= ?
				AND event_type = 'click'
				AND utm_source IS NOT NULL
				AND domain = ?
				AND session_id NOT IN (
					SELECT session_id FROM events
					WHERE timestamp >= ? AND has_scroll = 1 AND domain = ?
					UNION
					SELECT session_id FROM events
					WHERE timestamp >= ? AND page_duration > 5000 AND domain = ?
				)
		`
		args = []interface{}{cutoff, domain, cutoff, domain, cutoff, domain}
	}

	var count int64
	d.db.QueryRow(query, args...).Scan(&count)

	if count > 0 {
		return []FraudSignal{{
			Type:        "engagement_mismatch",
			Description: "Campaign clicks with no scroll or meaningful time on site",
			Count:       count,
			Severity:    "medium",
		}}
	}
	return nil
}

// calculateWastedSpend estimates money wasted on bot/fraudulent clicks
func (d *Detector) calculateWastedSpend(domain string, cutoff int64) float64 {
	query := `
		SELECT COALESCE(SUM(c.cpc), 0) as waste
		FROM events e
		JOIN campaigns c ON (
			(c.utm_source IS NULL OR c.utm_source = e.utm_source)
			AND (c.utm_medium IS NULL OR c.utm_medium = e.utm_medium)
			AND (c.utm_campaign IS NULL OR c.utm_campaign = e.utm_campaign)
		)
		WHERE e.timestamp >= ?
			AND e.event_type = 'click'
			AND e.bot_score >= 50
	`
	args := []interface{}{cutoff}
	if domain != "" {
		query += " AND e.domain = ?"
		args = append(args, domain)
	}

	var waste float64
	d.db.QueryRow(query, args...).Scan(&waste)
	return waste / 100 // Convert cents to dollars
}
