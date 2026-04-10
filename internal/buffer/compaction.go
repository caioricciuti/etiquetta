package buffer

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Compactor handles periodic table compaction for DuckDB.
type Compactor struct {
	db        *sql.DB
	bufferMgr *BufferManager
}

// NewCompactor creates a new compactor.
func NewCompactor(db *sql.DB, bufferMgr *BufferManager) *Compactor {
	return &Compactor{
		db:        db,
		bufferMgr: bufferMgr,
	}
}

// RunCompaction flushes buffers and runs CHECKPOINT to compact the DuckDB storage.
// This replaces the previous CREATE/DROP/ALTER approach which caused heap corruption
// when HTTP handlers executed concurrent queries during DDL operations.
func (c *Compactor) RunCompaction(ctx context.Context) error {
	log.Println("[compaction] Flushing buffers before compaction...")
	c.bufferMgr.Flush(ctx)

	c.bufferMgr.PauseWrites()
	defer c.bufferMgr.ResumeWrites()

	start := time.Now()
	if _, err := c.db.ExecContext(ctx, "CHECKPOINT"); err != nil {
		return fmt.Errorf("checkpoint failed: %w", err)
	}
	log.Printf("[compaction] Checkpoint completed in %v", time.Since(start))
	return nil
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
