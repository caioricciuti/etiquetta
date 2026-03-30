package buffer

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// tableIndexes maps table names to their index creation statements.
var tableIndexes = map[string][]string{
	"events": {
		"CREATE INDEX idx_events_timestamp ON events(timestamp)",
		"CREATE INDEX idx_events_session ON events(session_id)",
		"CREATE INDEX idx_events_visitor ON events(visitor_hash)",
		"CREATE INDEX idx_events_domain ON events(domain)",
		"CREATE INDEX idx_events_path ON events(path)",
		"CREATE INDEX idx_events_type ON events(event_type)",
		"CREATE INDEX idx_events_country ON events(geo_country)",
		"CREATE INDEX idx_events_bot_category ON events(bot_category)",
		"CREATE INDEX idx_events_bot_score ON events(bot_score)",
		"CREATE INDEX idx_events_geo ON events(geo_latitude, geo_longitude)",
		"CREATE INDEX idx_events_ts_domain_bot ON events(timestamp, domain, is_bot)",
		"CREATE INDEX idx_events_ts_type_bot ON events(timestamp, event_type, is_bot)",
	},
	"performance": {
		"CREATE INDEX idx_perf_timestamp ON performance(timestamp)",
		"CREATE INDEX idx_perf_session ON performance(session_id)",
		"CREATE INDEX idx_perf_path ON performance(path)",
	},
	"errors": {
		"CREATE INDEX idx_errors_timestamp ON errors(timestamp)",
		"CREATE INDEX idx_errors_hash ON errors(error_hash)",
		"CREATE INDEX idx_errors_type ON errors(error_type)",
		"CREATE INDEX idx_errors_session ON errors(session_id)",
	},
	"visitor_sessions": {
		"CREATE INDEX idx_visitor_sessions_session ON visitor_sessions(session_id)",
		"CREATE INDEX idx_visitor_sessions_domain ON visitor_sessions(domain)",
		"CREATE INDEX idx_visitor_sessions_bot ON visitor_sessions(bot_category)",
		"CREATE INDEX idx_vsessions_domain_start ON visitor_sessions(domain, start_time)",
	},
}

// Compactor handles periodic table compaction for DuckDB.
type Compactor struct {
	db        *sql.DB
	bufferMgr *BufferManager
	tables    []string
}

// NewCompactor creates a new compactor for the given tables.
func NewCompactor(db *sql.DB, bufferMgr *BufferManager) *Compactor {
	return &Compactor{
		db:        db,
		bufferMgr: bufferMgr,
		tables:    []string{"events", "performance", "errors", "visitor_sessions"},
	}
}

// RunCompaction compacts all high-write tables by recreating them without fragmentation.
// It flushes all buffers and pauses the writer loop to get exclusive DuckDB access,
// preventing concurrent INSERT + DDL races that cause heap corruption via CGO.
func (c *Compactor) RunCompaction(ctx context.Context) error {
	// Flush all buffered data so nothing is lost during compaction
	log.Println("[compaction] Flushing buffers before compaction...")
	c.bufferMgr.Flush(ctx)

	// Block the writer loop from executing INSERTs during DDL
	c.bufferMgr.PauseWrites()
	defer c.bufferMgr.ResumeWrites()

	for _, table := range c.tables {
		start := time.Now()
		if err := c.compactTable(ctx, table); err != nil {
			log.Printf("[compaction] Failed to compact %s: %v", table, err)
			continue
		}
		log.Printf("[compaction] Compacted %s in %v", table, time.Since(start))
	}
	return nil
}

func (c *Compactor) compactTable(ctx context.Context, table string) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	compacted := table + "_compacted"

	// Create compacted copy
	_, err = tx.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s AS SELECT * FROM %s", compacted, table))
	if err != nil {
		return fmt.Errorf("create compacted table: %w", err)
	}

	// Drop original
	_, err = tx.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s", table))
	if err != nil {
		return fmt.Errorf("drop original: %w", err)
	}

	// Rename compacted to original
	_, err = tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s", compacted, table))
	if err != nil {
		return fmt.Errorf("rename compacted: %w", err)
	}

	// Recreate indexes
	if indexes, ok := tableIndexes[table]; ok {
		for _, idx := range indexes {
			if _, err := tx.ExecContext(ctx, idx); err != nil {
				log.Printf("[compaction] Warning: failed to create index on %s: %v", table, err)
			}
		}
	}

	return tx.Commit()
}

// StartSchedule runs compaction daily at the specified hour.
func (c *Compactor) StartSchedule(ctx context.Context, hour int) {
	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}
			timer := time.NewTimer(time.Until(next))

			select {
			case <-timer.C:
				log.Println("[compaction] Starting scheduled compaction...")
				if err := c.RunCompaction(ctx); err != nil {
					log.Printf("[compaction] Scheduled compaction failed: %v", err)
				}
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}()
}
