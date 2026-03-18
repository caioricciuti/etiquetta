package bot

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// BatchAnalyzer performs scheduled analysis of session behavior
type BatchAnalyzer struct {
	db       *sql.DB
	interval time.Duration
	stopCh   chan struct{}
}

// NewBatchAnalyzer creates a new batch analyzer
func NewBatchAnalyzer(db *sql.DB, interval time.Duration) *BatchAnalyzer {
	return &BatchAnalyzer{
		db:       db,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the batch analysis loop
func (b *BatchAnalyzer) Start() {
	log.Printf("Starting bot batch analyzer with %v interval", b.interval)

	// Run immediately on startup
	b.analyze()

	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.analyze()
		case <-b.stopCh:
			log.Println("Stopping bot batch analyzer")
			return
		}
	}
}

// Stop halts the batch analysis
func (b *BatchAnalyzer) Stop() {
	close(b.stopCh)
}

// analyze runs all behavioral analysis patterns
func (b *BatchAnalyzer) analyze() {
	since := time.Now().Add(-15 * time.Minute)
	log.Printf("Running bot batch analysis for sessions since %v", since.Format(time.RFC3339))

	count := 0
	count += b.analyzeZeroInteraction(since)
	count += b.analyzeImpossibleSpeed(since)
	count += b.analyzePerfectTiming(since)
	count += b.analyzeAnomalousVitals(since)
	count += b.analyzeNoInteractionLongSession(since)

	if count > 0 {
		log.Printf("Bot batch analysis: updated %d sessions", count)
	}

	if err := b.MaterializeSessions(since); err != nil {
		log.Printf("Materialize sessions error: %v", err)
	}
}

// appendSignal appends a bot signal to a JSON array string in Go.
func appendSignal(existing string, signalName string, weight int) string {
	var signals []map[string]interface{}
	if err := json.Unmarshal([]byte(existing), &signals); err != nil {
		signals = []map[string]interface{}{}
	}
	signals = append(signals, map[string]interface{}{
		"name":   signalName,
		"weight": weight,
	})
	out, _ := json.Marshal(signals)
	return string(out)
}

// analyzeZeroInteraction detects sessions with no interaction
func (b *BatchAnalyzer) analyzeZeroInteraction(since time.Time) int {
	// Find session IDs matching the pattern
	rows, err := b.db.Query(`
		SELECT session_id
		FROM events
		WHERE timestamp >= ?
		GROUP BY session_id
		HAVING
			SUM(has_scroll) = 0
			AND SUM(has_mouse_move) = 0
			AND SUM(has_click) = 0
			AND COUNT(*) = 1
			AND SUM(CASE WHEN event_type = 'pageview' THEN 1 ELSE 0 END) = 1
			AND COALESCE(MAX(page_duration), 0) < 1000
	`, since.UnixMilli())
	if err != nil {
		log.Printf("Zero interaction analysis error: %v", err)
		return 0
	}
	defer rows.Close()

	var sessionIDs []string
	for rows.Next() {
		var sid string
		rows.Scan(&sid)
		sessionIDs = append(sessionIDs, sid)
	}

	if len(sessionIDs) == 0 {
		return 0
	}

	// Update matching events in Go
	count := 0
	for _, sid := range sessionIDs {
		// Get current values
		var botScore int
		var botSignals, botCategory string
		err := b.db.QueryRow(
			"SELECT bot_score, bot_signals, bot_category FROM events WHERE session_id = ? AND bot_score < 75 AND bot_category != 'good_bot' AND bot_category != 'ai_crawler' AND bot_signals NOT LIKE '%zero_interaction%' LIMIT 1",
			sid,
		).Scan(&botScore, &botSignals, &botCategory)
		if err != nil {
			continue
		}

		newScore := botScore + 25
		if newScore > 100 {
			newScore = 100
		}
		newSignals := appendSignal(botSignals, "zero_interaction", 25)
		newCategory := botCategory
		if newScore > 50 {
			newCategory = "bad_bot"
		} else if newScore > 20 {
			newCategory = "suspicious"
		}

		result, err := b.db.Exec(
			"UPDATE events SET bot_score = ?, bot_signals = ?, bot_category = ? WHERE session_id = ? AND bot_score < 75 AND bot_category != 'good_bot' AND bot_category != 'ai_crawler' AND bot_signals NOT LIKE '%zero_interaction%'",
			newScore, newSignals, newCategory, sid,
		)
		if err != nil {
			continue
		}
		affected, _ := result.RowsAffected()
		count += int(affected)
	}

	return count
}

// analyzeImpossibleSpeed detects sessions with inhuman speed
func (b *BatchAnalyzer) analyzeImpossibleSpeed(since time.Time) int {
	rows, err := b.db.Query(`
		SELECT session_id
		FROM events
		WHERE timestamp >= ?
			AND event_type = 'pageview'
		GROUP BY session_id
		HAVING
			COUNT(*) > 50
			AND (MAX(timestamp) - MIN(timestamp)) < 10000
	`, since.UnixMilli())
	if err != nil {
		log.Printf("Impossible speed analysis error: %v", err)
		return 0
	}
	defer rows.Close()

	var sessionIDs []string
	for rows.Next() {
		var sid string
		rows.Scan(&sid)
		sessionIDs = append(sessionIDs, sid)
	}

	count := 0
	for _, sid := range sessionIDs {
		var botScore int
		var botSignals string
		err := b.db.QueryRow(
			"SELECT bot_score, bot_signals FROM events WHERE session_id = ? AND bot_category != 'good_bot' AND bot_category != 'ai_crawler' AND bot_signals NOT LIKE '%impossible_speed%' LIMIT 1",
			sid,
		).Scan(&botScore, &botSignals)
		if err != nil {
			continue
		}

		newScore := botScore + 30
		if newScore > 100 {
			newScore = 100
		}
		newSignals := appendSignal(botSignals, "impossible_speed", 30)

		result, err := b.db.Exec(
			"UPDATE events SET bot_score = ?, bot_signals = ?, bot_category = 'bad_bot' WHERE session_id = ? AND bot_category != 'good_bot' AND bot_category != 'ai_crawler' AND bot_signals NOT LIKE '%impossible_speed%'",
			newScore, newSignals, sid,
		)
		if err != nil {
			continue
		}
		affected, _ := result.RowsAffected()
		count += int(affected)
	}

	return count
}

// analyzePerfectTiming detects sessions with robotic click patterns
func (b *BatchAnalyzer) analyzePerfectTiming(since time.Time) int {
	rows, err := b.db.Query(`
		SELECT e.session_id
		FROM events e
		WHERE e.timestamp >= ?
			AND e.event_type = 'click'
		GROUP BY e.session_id
		HAVING
			COUNT(*) >= 10
			AND (MAX(e.timestamp) - MIN(e.timestamp)) / COUNT(*) < 100
	`, since.UnixMilli())
	if err != nil {
		log.Printf("Perfect timing analysis error: %v", err)
		return 0
	}
	defer rows.Close()

	var sessionIDs []string
	for rows.Next() {
		var sid string
		rows.Scan(&sid)
		sessionIDs = append(sessionIDs, sid)
	}

	count := 0
	for _, sid := range sessionIDs {
		var botScore int
		var botSignals string
		err := b.db.QueryRow(
			"SELECT bot_score, bot_signals FROM events WHERE session_id = ? AND bot_category != 'good_bot' AND bot_category != 'ai_crawler' AND bot_signals NOT LIKE '%perfect_timing%' LIMIT 1",
			sid,
		).Scan(&botScore, &botSignals)
		if err != nil {
			continue
		}

		newScore := botScore + 20
		if newScore > 100 {
			newScore = 100
		}
		newSignals := appendSignal(botSignals, "perfect_timing", 20)
		newCategory := "suspicious"
		if newScore > 50 {
			newCategory = "bad_bot"
		}

		result, err := b.db.Exec(
			"UPDATE events SET bot_score = ?, bot_signals = ?, bot_category = ? WHERE session_id = ? AND bot_category != 'good_bot' AND bot_category != 'ai_crawler' AND bot_signals NOT LIKE '%perfect_timing%'",
			newScore, newSignals, newCategory, sid,
		)
		if err != nil {
			continue
		}
		affected, _ := result.RowsAffected()
		count += int(affected)
	}

	return count
}

// analyzeAnomalousVitals detects sessions with extreme web vitals (AI-driven browsers)
func (b *BatchAnalyzer) analyzeAnomalousVitals(since time.Time) int {
	rows, err := b.db.Query(`
		SELECT DISTINCT p.session_id
		FROM performance p
		WHERE p.timestamp >= ?
			AND (p.lcp > 30000 OR p.fcp > 20000)
			AND COALESCE(p.connection_type, '4g') NOT IN ('2g', 'slow-2g')
	`, since.UnixMilli())
	if err != nil {
		log.Printf("Anomalous vitals analysis error: %v", err)
		return 0
	}
	defer rows.Close()

	var sessionIDs []string
	for rows.Next() {
		var sid string
		rows.Scan(&sid)
		sessionIDs = append(sessionIDs, sid)
	}

	count := 0
	for _, sid := range sessionIDs {
		var botScore int
		var botSignals, botCategory string
		err := b.db.QueryRow(
			"SELECT bot_score, bot_signals, bot_category FROM events WHERE session_id = ? AND bot_category != 'good_bot' AND bot_category != 'ai_crawler' AND bot_signals NOT LIKE '%anomalous_vitals%' LIMIT 1",
			sid,
		).Scan(&botScore, &botSignals, &botCategory)
		if err != nil {
			continue
		}

		newScore := botScore + 20
		if newScore > 100 {
			newScore = 100
		}
		newSignals := appendSignal(botSignals, "anomalous_vitals", 20)
		newCategory := botCategory
		if newScore > 50 {
			newCategory = "bad_bot"
			// Check if automation signals are present
			if hasAutomationSignal(botSignals) {
				newCategory = CategoryAutomation
			}
		} else if newScore > 20 {
			newCategory = "suspicious"
		}

		result, err := b.db.Exec(
			"UPDATE events SET bot_score = ?, bot_signals = ?, bot_category = ? WHERE session_id = ? AND bot_category != 'good_bot' AND bot_category != 'ai_crawler' AND bot_signals NOT LIKE '%anomalous_vitals%'",
			newScore, newSignals, newCategory, sid,
		)
		if err != nil {
			continue
		}
		affected, _ := result.RowsAffected()
		count += int(affected)

		// Also update performance table
		b.db.Exec(
			"UPDATE performance SET bot_score = ?, bot_category = ? WHERE session_id = ?",
			newScore, newCategory, sid,
		)
	}

	return count
}

// analyzeNoInteractionLongSession detects sessions with zero interaction but long page views
func (b *BatchAnalyzer) analyzeNoInteractionLongSession(since time.Time) int {
	rows, err := b.db.Query(`
		SELECT session_id FROM events
		WHERE timestamp >= ?
		GROUP BY session_id
		HAVING SUM(has_scroll) = 0 AND SUM(has_mouse_move) = 0 AND SUM(has_click) = 0
			AND SUM(has_touch) = 0 AND MAX(page_duration) > 10000 AND COUNT(*) >= 2
	`, since.UnixMilli())
	if err != nil {
		log.Printf("No interaction long session analysis error: %v", err)
		return 0
	}
	defer rows.Close()

	var sessionIDs []string
	for rows.Next() {
		var sid string
		rows.Scan(&sid)
		sessionIDs = append(sessionIDs, sid)
	}

	count := 0
	for _, sid := range sessionIDs {
		var botScore int
		var botSignals, botCategory string
		err := b.db.QueryRow(
			"SELECT bot_score, bot_signals, bot_category FROM events WHERE session_id = ? AND bot_category != 'good_bot' AND bot_category != 'ai_crawler' AND bot_signals NOT LIKE '%no_interaction_long_session%' LIMIT 1",
			sid,
		).Scan(&botScore, &botSignals, &botCategory)
		if err != nil {
			continue
		}

		newScore := botScore + 15
		if newScore > 100 {
			newScore = 100
		}
		newSignals := appendSignal(botSignals, "no_interaction_long_session", 15)
		newCategory := botCategory
		if newScore > 50 {
			newCategory = "bad_bot"
			if hasAutomationSignal(botSignals) {
				newCategory = CategoryAutomation
			}
		} else if newScore > 20 {
			newCategory = "suspicious"
		}

		result, err := b.db.Exec(
			"UPDATE events SET bot_score = ?, bot_signals = ?, bot_category = ? WHERE session_id = ? AND bot_category != 'good_bot' AND bot_category != 'ai_crawler' AND bot_signals NOT LIKE '%no_interaction_long_session%'",
			newScore, newSignals, newCategory, sid,
		)
		if err != nil {
			continue
		}
		affected, _ := result.RowsAffected()
		count += int(affected)
	}

	return count
}

// hasAutomationSignal checks if bot signals JSON contains automation-specific signals
func hasAutomationSignal(signals string) bool {
	for _, name := range []string{"cdp_detected", "webdriver", "selenium"} {
		if strings.Contains(signals, name) {
			return true
		}
	}
	return false
}

// MaterializeSessions creates/updates the visitor_sessions table.
// Uses DELETE + INSERT instead of ON CONFLICT because DuckDB does not allow
// updating columns referenced by an index via ON CONFLICT.
func (b *BatchAnalyzer) MaterializeSessions(since time.Time) error {
	sinceMs := since.UnixMilli()

	// Delete existing sessions that will be recomputed
	_, err := b.db.Exec(`
		DELETE FROM visitor_sessions
		WHERE id IN (
			SELECT DISTINCT session_id || '_' || domain
			FROM events
			WHERE timestamp >= ?
		)
	`, sinceMs)
	if err != nil {
		return fmt.Errorf("delete existing sessions: %w", err)
	}

	// Insert fresh session data
	_, err = b.db.Exec(`
		INSERT INTO visitor_sessions (
			id, session_id, visitor_hash, domain,
			start_time, end_time, duration, pageviews,
			entry_url, exit_url, is_bounce,
			bot_score, bot_category
		)
		WITH session_data AS (
			SELECT
				session_id, domain, visitor_hash, timestamp, url,
				event_type, bot_score, bot_category,
				FIRST_VALUE(url) OVER (PARTITION BY session_id, domain ORDER BY timestamp) as entry_url,
				LAST_VALUE(url) OVER (PARTITION BY session_id, domain ORDER BY timestamp ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING) as exit_url
			FROM events
			WHERE timestamp >= ?
		)
		SELECT
			session_id || '_' || domain as id,
			session_id,
			MAX(visitor_hash) as visitor_hash,
			domain,
			MIN(timestamp) as start_time,
			MAX(timestamp) as end_time,
			MAX(timestamp) - MIN(timestamp) as duration,
			SUM(CASE WHEN event_type = 'pageview' THEN 1 ELSE 0 END) as pageviews,
			ANY_VALUE(entry_url) as entry_url,
			ANY_VALUE(exit_url) as exit_url,
			CASE WHEN SUM(CASE WHEN event_type = 'pageview' THEN 1 ELSE 0 END) = 1 THEN 1 ELSE 0 END as is_bounce,
			MAX(bot_score) as bot_score,
			MAX(bot_category) as bot_category
		FROM session_data
		GROUP BY session_id, domain
	`, sinceMs)
	if err != nil {
		return fmt.Errorf("insert sessions: %w", err)
	}

	return nil
}
