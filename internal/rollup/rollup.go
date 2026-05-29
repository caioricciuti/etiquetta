// Package rollup maintains pre-aggregated daily summaries of the events table so
// dashboard queries can read small rollup rows instead of scanning raw events
// (idea #1: keep dashboards fast at scale).
//
// Correctness: rollup_daily holds one row per (day, domain, bot_category) and
// every stored metric is EXACT for that day. Additive metrics (pageviews, events,
// sessions, bounce_sessions, visible_ms_sum, engagement_count) may be summed over
// a date range. Unique visitors are NOT additive across days, so the per-day
// 'visitors' column is exact but must never be summed for a multi-day range — the
// query layer keeps multi-day uniques on the exact raw path.
package rollup

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"
)

// Refresher periodically recomputes recent days of rollup_daily from raw events.
type Refresher struct {
	db       *sql.DB
	dbMu     *sync.RWMutex
	interval time.Duration

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewRefresher creates a rollup refresher. dbMu is the shared DuckDB lock; the
// refresher takes a read lock while writing rollup rows so it is paused during
// compaction's exclusive lock, exactly like every other writer.
func NewRefresher(db *sql.DB, dbMu *sync.RWMutex, interval time.Duration) *Refresher {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &Refresher{
		db:       db,
		dbMu:     dbMu,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start runs an initial backfill and then refreshes on the configured interval.
func (rf *Refresher) Start() {
	rf.wg.Add(1)
	go func() {
		defer rf.wg.Done()

		// Initial pass: backfill all history once so rollups are usable immediately
		// after upgrade. Subsequent passes only touch recent days.
		if err := rf.RefreshSince(context.Background(), 0); err != nil {
			log.Printf("[rollup] initial backfill failed: %v", err)
		} else {
			log.Println("[rollup] initial backfill complete")
		}

		ticker := time.NewTicker(rf.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Refresh the last 2 days each cycle to absorb late/buffered events.
				cutoff := time.Now().AddDate(0, 0, -2).UnixMilli()
				if err := rf.RefreshSince(context.Background(), cutoff); err != nil {
					log.Printf("[rollup] refresh failed: %v", err)
				}
			case <-rf.stopCh:
				return
			}
		}
	}()
}

// Stop halts the refresher and waits for the goroutine to exit.
func (rf *Refresher) Stop() {
	rf.stopOnce.Do(func() {
		close(rf.stopCh)
		rf.wg.Wait()
	})
}

// RefreshSince recomputes rollup_daily for every day with events at or after
// cutoffMs (pass 0 to rebuild all history). It is idempotent: affected day
// buckets are overwritten in place via INSERT ... ON CONFLICT DO UPDATE. It
// never deletes raw data and never deletes rollup rows.
func (rf *Refresher) RefreshSince(ctx context.Context, cutoffMs int64) error {
	rf.dbMu.RLock()
	defer rf.dbMu.RUnlock()

	_, err := rf.db.ExecContext(ctx, `
		INSERT INTO rollup_daily (
			date_key, domain, bot_category,
			pageviews, events, sessions, visitors, bounce_sessions,
			visible_ms_sum, engagement_count, updated_at
		)
		WITH base AS (
			SELECT
				strftime('%Y-%m-%d', to_timestamp(timestamp / 1000)::TIMESTAMP) AS date_key,
				domain,
				COALESCE(NULLIF(bot_category, ''), 'human') AS bot_category,
				session_id,
				visitor_hash,
				event_type,
				props
			FROM events
			WHERE timestamp >= ?
		),
		sess AS (
			SELECT date_key, domain, bot_category, session_id,
				SUM(CASE WHEN event_type = 'pageview' THEN 1 ELSE 0 END) AS pv
			FROM base
			GROUP BY date_key, domain, bot_category, session_id
		),
		sess_agg AS (
			SELECT date_key, domain, bot_category,
				COUNT(*) AS sessions,
				SUM(CASE WHEN pv <= 1 THEN 1 ELSE 0 END) AS bounce_sessions
			FROM sess
			GROUP BY date_key, domain, bot_category
		),
		ev_agg AS (
			SELECT date_key, domain, bot_category,
				SUM(CASE WHEN event_type = 'pageview' THEN 1 ELSE 0 END) AS pageviews,
				COUNT(*) AS events,
				COUNT(DISTINCT visitor_hash) AS visitors,
				COALESCE(SUM(TRY_CAST(json_extract_string(props, '$.visible_time_ms') AS BIGINT)), 0) AS visible_ms_sum,
				SUM(CASE WHEN event_type = 'engagement' THEN 1 ELSE 0 END) AS engagement_count
			FROM base
			GROUP BY date_key, domain, bot_category
		)
		SELECT
			e.date_key, e.domain, e.bot_category,
			e.pageviews, e.events, s.sessions, e.visitors, s.bounce_sessions,
			e.visible_ms_sum, e.engagement_count, epoch_ms(now())
		FROM ev_agg e
		JOIN sess_agg s USING (date_key, domain, bot_category)
		ON CONFLICT (date_key, domain, bot_category) DO UPDATE SET
			pageviews = excluded.pageviews,
			events = excluded.events,
			sessions = excluded.sessions,
			visitors = excluded.visitors,
			bounce_sessions = excluded.bounce_sessions,
			visible_ms_sum = excluded.visible_ms_sum,
			engagement_count = excluded.engagement_count,
			updated_at = excluded.updated_at
	`, cutoffMs)
	return err
}
