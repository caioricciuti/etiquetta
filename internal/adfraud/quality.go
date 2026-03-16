package adfraud

import (
	"database/sql"
	"time"
)

// SourceQuality represents traffic quality metrics for a UTM source
type SourceQuality struct {
	UTMSource     string  `json:"utm_source"`
	UTMMedium     string  `json:"utm_medium"`
	UTMCampaign   string  `json:"utm_campaign"`
	TotalVisits   int64   `json:"total_visits"`
	BotVisits     int64   `json:"bot_visits"`
	HumanVisits   int64   `json:"human_visits"`
	BotRate       float64 `json:"bot_rate"`
	AvgBotScore   float64 `json:"avg_bot_score"`
	BounceRate    float64 `json:"bounce_rate"`
	AvgDuration   float64 `json:"avg_duration_seconds"`
	QualityScore  int     `json:"quality_score"` // 0-100, higher is better
}

// GetSourceQuality returns traffic quality metrics per UTM source
func (d *Detector) GetSourceQuality(domain string, days int) ([]SourceQuality, error) {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()

	query := `
		SELECT
			COALESCE(utm_source, '(direct)') as utm_source,
			COALESCE(utm_medium, '(none)') as utm_medium,
			COALESCE(utm_campaign, '(none)') as utm_campaign,
			COUNT(DISTINCT session_id) as total_visits,
			SUM(CASE WHEN bot_category IN ('bad_bot', 'good_bot') THEN 1 ELSE 0 END) as bot_visits,
			SUM(CASE WHEN bot_category = 'human' THEN 1 ELSE 0 END) as human_visits,
			AVG(bot_score) as avg_bot_score,
			AVG(CASE WHEN page_duration IS NOT NULL THEN page_duration / 1000.0 ELSE NULL END) as avg_duration
		FROM events
		WHERE timestamp >= ?
			AND event_type = 'pageview'
	`
	args := []interface{}{cutoff}
	if domain != "" {
		query += " AND domain = ?"
		args = append(args, domain)
	}
	query += `
		GROUP BY utm_source, utm_medium, utm_campaign
		HAVING total_visits >= 10
		ORDER BY total_visits DESC
		LIMIT 50
	`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]SourceQuality, 0)
	for rows.Next() {
		var sq SourceQuality
		var avgDuration sql.NullFloat64
		err := rows.Scan(
			&sq.UTMSource,
			&sq.UTMMedium,
			&sq.UTMCampaign,
			&sq.TotalVisits,
			&sq.BotVisits,
			&sq.HumanVisits,
			&sq.AvgBotScore,
			&avgDuration,
		)
		if err != nil {
			continue
		}

		if avgDuration.Valid {
			sq.AvgDuration = avgDuration.Float64
		}

		// Calculate bot rate
		if sq.TotalVisits > 0 {
			sq.BotRate = float64(sq.BotVisits) / float64(sq.TotalVisits) * 100
		}

		// Calculate quality score (inverse of bot rate + engagement factors)
		sq.QualityScore = calculateQualityScore(sq)

		results = append(results, sq)
	}

	// Get bounce rates separately (requires aggregation)
	d.populateBounceRates(results, domain, cutoff)

	return results, nil
}

// calculateQualityScore computes a 0-100 quality score
func calculateQualityScore(sq SourceQuality) int {
	score := 100.0

	// Penalize for bot traffic (-0.5 points per % bot rate)
	score -= sq.BotRate * 0.5

	// Penalize for high average bot score
	if sq.AvgBotScore > 20 {
		score -= (sq.AvgBotScore - 20) * 0.3
	}

	// Bonus for engagement (time on site)
	if sq.AvgDuration > 30 {
		score += 5
	} else if sq.AvgDuration < 5 {
		score -= 10
	}

	// Clamp to 0-100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return int(score)
}

// populateBounceRates adds bounce rate data to source quality results
func (d *Detector) populateBounceRates(results []SourceQuality, domain string, cutoff int64) {
	for i := range results {
		sq := &results[i]

		query := `
			SELECT
				CAST(SUM(CASE WHEN pv_count = 1 THEN 1 ELSE 0 END) AS DOUBLE) / NULLIF(COUNT(*), 0) * 100
			FROM (
				SELECT session_id, COUNT(*) as pv_count
				FROM events
				WHERE timestamp >= ?
					AND event_type = 'pageview'
					AND COALESCE(utm_source, '(direct)') = ?
					AND COALESCE(utm_medium, '(none)') = ?
					AND COALESCE(utm_campaign, '(none)') = ?
		`
		args := []interface{}{cutoff, sq.UTMSource, sq.UTMMedium, sq.UTMCampaign}
		if domain != "" {
			query += " AND domain = ?"
			args = append(args, domain)
		}
		query += " GROUP BY session_id)"

		var bounceRate sql.NullFloat64
		d.db.QueryRow(query, args...).Scan(&bounceRate)
		if bounceRate.Valid {
			sq.BounceRate = bounceRate.Float64
		}
	}
}
