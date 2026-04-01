package buffer

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// FlushJob represents a parquet file ready to be loaded into DuckDB.
type FlushJob struct {
	Table    string
	FilePath string
}

// TableBuffer is a generic per-table in-memory buffer.
type TableBuffer[T any] struct {
	mu         sync.Mutex
	rows       []T
	lastFlush  time.Time
	flushCount int64
	tableName  string
	threshold  int
	tempDir    string
	flushCh    chan<- FlushJob
}

func newTableBuffer[T any](name string, threshold int, tempDir string, flushCh chan<- FlushJob) *TableBuffer[T] {
	return &TableBuffer[T]{
		rows:      make([]T, 0, threshold),
		lastFlush: time.Now(),
		tableName: name,
		threshold: threshold,
		tempDir:   tempDir,
		flushCh:   flushCh,
	}
}

// Add appends a row. If the buffer exceeds threshold, it flushes asynchronously.
func (tb *TableBuffer[T]) Add(row T) {
	tb.mu.Lock()
	tb.rows = append(tb.rows, row)
	if len(tb.rows) >= tb.threshold {
		// Swap out the slice under lock — fast path
		rows := tb.rows
		tb.rows = make([]T, 0, tb.threshold)
		tb.lastFlush = time.Now()
		tb.mu.Unlock()

		// Write parquet and send flush job outside lock
		go tb.writeAndFlush(rows)
		return
	}
	tb.mu.Unlock()
}

// ForceFlush flushes whatever is in the buffer, regardless of threshold.
func (tb *TableBuffer[T]) ForceFlush() {
	tb.mu.Lock()
	if len(tb.rows) == 0 {
		tb.mu.Unlock()
		return
	}
	rows := tb.rows
	tb.rows = make([]T, 0, tb.threshold)
	tb.lastFlush = time.Now()
	tb.mu.Unlock()

	tb.writeAndFlush(rows)
}

// Len returns the current buffer size.
func (tb *TableBuffer[T]) Len() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return len(tb.rows)
}

// LastFlush returns the last flush time.
func (tb *TableBuffer[T]) LastFlush() time.Time {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.lastFlush
}

// FlushCount returns total flushes.
func (tb *TableBuffer[T]) FlushCount() int64 {
	return atomic.LoadInt64(&tb.flushCount)
}

func (tb *TableBuffer[T]) writeAndFlush(rows []T) {
	path, err := writeParquet[T](tb.tableName, rows, tb.tempDir)
	if err != nil {
		log.Printf("[buffer] Failed to write parquet for %s (%d rows): %v", tb.tableName, len(rows), err)
		return
	}

	atomic.AddInt64(&tb.flushCount, 1)

	tb.flushCh <- FlushJob{
		Table:    tb.tableName,
		FilePath: path,
	}
}

// BufferStats holds stats for monitoring.
type BufferStats struct {
	Events      TableStats `json:"events"`
	Performance TableStats `json:"performance"`
	Errors      TableStats `json:"errors"`
	Sessions    TableStats `json:"sessions"`
	ErrorCount  int64      `json:"error_count"`
}

// TableStats holds per-table stats.
type TableStats struct {
	BufferedRows int       `json:"buffered_rows"`
	FlushCount   int64     `json:"flush_count"`
	LastFlush    time.Time `json:"last_flush"`
}

// BufferManager coordinates all table buffers and the DuckDB writer.
type BufferManager struct {
	events      *TableBuffer[Event]
	performance *TableBuffer[Performance]
	errors      *TableBuffer[ErrorEvent]
	sessions    *TableBuffer[VisitorSession]

	flushCh    chan FlushJob
	db         *sql.DB
	dbMu       sync.RWMutex // Guards DuckDB access; compaction takes write lock, writer takes read lock
	config     BufferConfig
	errorCount int64

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewBufferManager creates and starts the buffer manager.
func NewBufferManager(db *sql.DB, cfg BufferConfig) *BufferManager {
	flushCh := make(chan FlushJob, 64)

	bm := &BufferManager{
		events:      newTableBuffer[Event]("events", cfg.Threshold, cfg.TempDir, flushCh),
		performance: newTableBuffer[Performance]("performance", cfg.Threshold, cfg.TempDir, flushCh),
		errors:      newTableBuffer[ErrorEvent]("errors", cfg.Threshold, cfg.TempDir, flushCh),
		sessions:    newTableBuffer[VisitorSession]("visitor_sessions", cfg.Threshold, cfg.TempDir, flushCh),
		flushCh:     flushCh,
		db:          db,
		config:      cfg,
		stopCh:      make(chan struct{}),
	}

	bm.wg.Add(2)
	go bm.writerLoop()
	go bm.tickerLoop()

	return bm
}

// AddEvent buffers a tracking event.
func (bm *BufferManager) AddEvent(_ context.Context, e Event) {
	bm.events.Add(e)
}

// AddPerformance buffers a web vitals record.
func (bm *BufferManager) AddPerformance(_ context.Context, p Performance) {
	bm.performance.Add(p)
}

// AddError buffers a JS error event.
func (bm *BufferManager) AddError(_ context.Context, e ErrorEvent) {
	bm.errors.Add(e)
}

// AddSession buffers a materialized session.
func (bm *BufferManager) AddSession(_ context.Context, s VisitorSession) {
	bm.sessions.Add(s)
}

// Flush forces all buffers to flush immediately.
func (bm *BufferManager) Flush(_ context.Context) {
	bm.events.ForceFlush()
	bm.performance.ForceFlush()
	bm.errors.ForceFlush()
	bm.sessions.ForceFlush()
}

// Stats returns current buffer statistics.
func (bm *BufferManager) Stats() BufferStats {
	return BufferStats{
		Events: TableStats{
			BufferedRows: bm.events.Len(),
			FlushCount:   bm.events.FlushCount(),
			LastFlush:    bm.events.LastFlush(),
		},
		Performance: TableStats{
			BufferedRows: bm.performance.Len(),
			FlushCount:   bm.performance.FlushCount(),
			LastFlush:    bm.performance.LastFlush(),
		},
		Errors: TableStats{
			BufferedRows: bm.errors.Len(),
			FlushCount:   bm.errors.FlushCount(),
			LastFlush:    bm.errors.LastFlush(),
		},
		Sessions: TableStats{
			BufferedRows: bm.sessions.Len(),
			FlushCount:   bm.sessions.FlushCount(),
			LastFlush:    bm.sessions.LastFlush(),
		},
		ErrorCount: atomic.LoadInt64(&bm.errorCount),
	}
}

// Close gracefully shuts down: stops ticker, flushes all buffers, drains flushCh, waits for writer.
// Times out after 30 seconds to prevent hanging on DuckDB deadlocks.
func (bm *BufferManager) Close(_ context.Context) {
	bm.stopOnce.Do(func() {
		log.Println("[buffer] Shutting down buffer manager...")
		close(bm.stopCh)

		// Force flush all remaining data
		bm.events.ForceFlush()
		bm.performance.ForceFlush()
		bm.errors.ForceFlush()
		bm.sessions.ForceFlush()

		// Close the flush channel to signal writer to drain and exit
		close(bm.flushCh)

		// Wait for writer and ticker goroutines with timeout
		done := make(chan struct{})
		go func() {
			bm.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Println("[buffer] Buffer manager stopped")
		case <-time.After(30 * time.Second):
			log.Println("[buffer] Buffer manager shutdown timed out after 30s, forcing exit")
		}
	})
}

// DBMu returns the database access mutex. Background jobs that write to compacted
// tables (events, performance, errors, visitor_sessions) should hold RLock during writes.
func (bm *BufferManager) DBMu() *sync.RWMutex {
	return &bm.dbMu
}

// PauseWrites acquires an exclusive lock, blocking the writer loop.
// Must be paired with ResumeWrites. Used by compaction to get exclusive DB access.
func (bm *BufferManager) PauseWrites() {
	bm.dbMu.Lock()
}

// ResumeWrites releases the exclusive lock, unblocking the writer loop.
func (bm *BufferManager) ResumeWrites() {
	bm.dbMu.Unlock()
}

// writerLoop reads FlushJobs and loads parquet files into DuckDB.
// Failed loads are retried up to 3 times with exponential backoff.
func (bm *BufferManager) writerLoop() {
	defer bm.wg.Done()

	const maxRetries = 3
	backoffs := [3]time.Duration{1 * time.Second, 5 * time.Second, 15 * time.Second}

	for job := range bm.flushCh {
		var err error
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				log.Printf("[buffer] Retrying load into %s (attempt %d/%d)", job.Table, attempt, maxRetries)
				time.Sleep(backoffs[attempt-1])
			}

			bm.dbMu.RLock()
			err = bm.loadParquet(job)
			bm.dbMu.RUnlock()

			if err == nil {
				break
			}
		}

		if err != nil {
			atomic.AddInt64(&bm.errorCount, 1)
			log.Printf("[buffer] Failed to load parquet into %s after %d retries: %v (file kept: %s)", job.Table, maxRetries, err, job.FilePath)
			continue
		}
		os.Remove(job.FilePath)
	}
}

// tickerLoop checks buffer ages and triggers flushes when timeout expires.
func (bm *BufferManager) tickerLoop() {
	defer bm.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			timeout := bm.config.FlushTimeout

			if bm.events.Len() > 0 && now.Sub(bm.events.LastFlush()) > timeout {
				bm.events.ForceFlush()
			}
			if bm.performance.Len() > 0 && now.Sub(bm.performance.LastFlush()) > timeout {
				bm.performance.ForceFlush()
			}
			if bm.errors.Len() > 0 && now.Sub(bm.errors.LastFlush()) > timeout {
				bm.errors.ForceFlush()
			}
			if bm.sessions.Len() > 0 && now.Sub(bm.sessions.LastFlush()) > timeout {
				bm.sessions.ForceFlush()
			}
		case <-bm.stopCh:
			return
		}
	}
}

// loadParquet loads a parquet file into DuckDB.
func (bm *BufferManager) loadParquet(job FlushJob) error {
	query := fmt.Sprintf("INSERT INTO %s BY NAME SELECT * FROM read_parquet('%s')", job.Table, job.FilePath)
	_, err := bm.db.Exec(query)
	return err
}
