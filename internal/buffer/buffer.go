package buffer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// FlushJob represents a parquet file ready to be loaded into DuckDB.
type FlushJob struct {
	Table    string
	FilePath string

	// done, when non-nil, marks a drain barrier: the writer closes it instead
	// of loading a file, signalling that every job queued earlier is finished.
	done chan struct{}
}

var recoverableBufferTables = []string{
	"visitor_sessions", // longest prefix first
	"performance",
	"events",
	"errors",
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
	writeWG    *sync.WaitGroup
}

func newTableBuffer[T any](name string, threshold int, tempDir string, flushCh chan<- FlushJob, writeWG *sync.WaitGroup) *TableBuffer[T] {
	return &TableBuffer[T]{
		rows:      make([]T, 0, threshold),
		lastFlush: time.Now(),
		tableName: name,
		threshold: threshold,
		tempDir:   tempDir,
		flushCh:   flushCh,
		writeWG:   writeWG,
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

		// Write parquet and send the flush job outside the table lock. The
		// manager waits for these producers before closing flushCh.
		tb.writeWG.Add(1)
		go func() {
			defer tb.writeWG.Done()
			tb.writeAndFlush(rows)
		}()
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

	lifecycleMu sync.Mutex
	accepting   bool
	operationWG sync.WaitGroup
	producerWG  sync.WaitGroup

	stopOnce sync.Once
	stopCh   chan struct{}
	tickerWG sync.WaitGroup
	writerWG sync.WaitGroup
	closeErr error

	writerCtx    context.Context
	writerCancel context.CancelFunc
}

// NewBufferManager creates and starts the buffer manager.
func NewBufferManager(db *sql.DB, cfg BufferConfig) *BufferManager {
	flushCh := make(chan FlushJob, 64)
	writerCtx, writerCancel := context.WithCancel(context.Background())

	bm := &BufferManager{
		flushCh:      flushCh,
		db:           db,
		config:       cfg,
		stopCh:       make(chan struct{}),
		accepting:    true,
		writerCtx:    writerCtx,
		writerCancel: writerCancel,
	}
	bm.events = newTableBuffer[Event]("events", cfg.Threshold, cfg.TempDir, flushCh, &bm.producerWG)
	bm.performance = newTableBuffer[Performance]("performance", cfg.Threshold, cfg.TempDir, flushCh, &bm.producerWG)
	bm.errors = newTableBuffer[ErrorEvent]("errors", cfg.Threshold, cfg.TempDir, flushCh, &bm.producerWG)
	bm.sessions = newTableBuffer[VisitorSession]("visitor_sessions", cfg.Threshold, cfg.TempDir, flushCh, &bm.producerWG)

	// Snapshot leftovers before publishing the manager to callers. Scanning in
	// the recovery goroutine could discover a newly written threshold file and
	// queue it a second time alongside its live producer.
	pendingRecovery := bm.findPendingParquet()

	bm.writerWG.Add(1)
	bm.tickerWG.Add(1)
	go bm.writerLoop()
	bm.producerWG.Add(1)
	go func() {
		defer bm.producerWG.Done()
		bm.queuePendingParquet(pendingRecovery)
	}()
	go bm.tickerLoop()

	return bm
}

// findPendingParquet snapshots complete buffer files left by an interrupted or
// deadline-limited shutdown. Unknown and non-regular files are deliberately
// left untouched for operator inspection.
func (bm *BufferManager) findPendingParquet() []FlushJob {
	entries, err := os.ReadDir(bm.config.TempDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		atomic.AddInt64(&bm.errorCount, 1)
		log.Printf("[buffer] Failed to scan pending parquet files: %v", err)
		return nil
	}

	jobs := make([]FlushJob, 0, len(entries))
	for _, entry := range entries {
		// Truncated leftovers from a crash mid-writeParquet: safe to delete,
		// their rows never reached the flush queue.
		if name, isTemp := strings.CutSuffix(entry.Name(), ".tmp"); isTemp {
			if _, ours := recoverableTableForFile(name); ours {
				tempPath := filepath.Join(bm.config.TempDir, entry.Name())
				if err := os.Remove(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					atomic.AddInt64(&bm.errorCount, 1)
					log.Printf("[buffer] Failed to remove truncated parquet temp %s: %v", entry.Name(), err)
				} else {
					log.Printf("[buffer] Removed truncated parquet temp file: %s", entry.Name())
				}
				continue
			}
		}
		table, ok := recoverableTableForFile(entry.Name())
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			atomic.AddInt64(&bm.errorCount, 1)
			log.Printf("[buffer] Failed to inspect pending file %s: %v", entry.Name(), err)
			continue
		}
		if !info.Mode().IsRegular() {
			log.Printf("[buffer] Refusing non-regular pending file: %s", entry.Name())
			continue
		}
		jobs = append(jobs, FlushJob{Table: table, FilePath: filepath.Join(bm.config.TempDir, entry.Name())})
	}
	return jobs
}

func (bm *BufferManager) queuePendingParquet(jobs []FlushJob) {
	queued := 0
	for _, job := range jobs {
		select {
		case bm.flushCh <- job:
			queued++
		case <-bm.stopCh:
			return
		}
	}
	if queued > 0 {
		log.Printf("[buffer] Queued %d pending parquet file(s) for recovery", queued)
	}
}

func recoverableTableForFile(name string) (string, bool) {
	if !strings.HasSuffix(name, ".parquet") {
		return "", false
	}
	for _, table := range recoverableBufferTables {
		if strings.HasPrefix(name, table+"_") {
			return table, true
		}
	}
	return "", false
}

// AddEvent buffers a tracking event.
func (bm *BufferManager) AddEvent(_ context.Context, e Event) {
	if !bm.beginOperation() {
		return
	}
	defer bm.operationWG.Done()
	bm.events.Add(e)
}

// AddPerformance buffers a web vitals record.
func (bm *BufferManager) AddPerformance(_ context.Context, p Performance) {
	if !bm.beginOperation() {
		return
	}
	defer bm.operationWG.Done()
	bm.performance.Add(p)
}

// AddError buffers a JS error event.
func (bm *BufferManager) AddError(_ context.Context, e ErrorEvent) {
	if !bm.beginOperation() {
		return
	}
	defer bm.operationWG.Done()
	bm.errors.Add(e)
}

// AddSession buffers a materialized session.
func (bm *BufferManager) AddSession(_ context.Context, s VisitorSession) {
	if !bm.beginOperation() {
		return
	}
	defer bm.operationWG.Done()
	bm.sessions.Add(s)
}

// Flush forces all buffers to flush immediately.
func (bm *BufferManager) Flush(ctx context.Context) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	if !bm.beginOperation() {
		return
	}
	defer bm.operationWG.Done()
	bm.flushAll()
}

// FlushAndWait flushes all buffers and blocks until every parquet load queued
// so far has been applied to DuckDB (or failed permanently). Callers that need
// read-your-writes semantics, such as import rollback, use this before running
// queries against previously buffered rows.
func (bm *BufferManager) FlushAndWait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !bm.beginOperation() {
		return errors.New("buffer manager is shutting down")
	}
	defer bm.operationWG.Done()

	// ForceFlush queues parquet jobs synchronously, so the barrier below is
	// guaranteed to sit behind every row that was buffered before this call.
	bm.flushAll()

	done := make(chan struct{})
	select {
	case bm.flushCh <- FlushJob{done: done}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (bm *BufferManager) flushAll() {
	bm.events.ForceFlush()
	bm.performance.ForceFlush()
	bm.errors.ForceFlush()
	bm.sessions.ForceFlush()
}

// beginOperation prevents new producers from starting once shutdown begins.
// Holding lifecycleMu while incrementing operationWG makes its later Wait safe:
// once accepting is false, no goroutine can add to the wait group.
func (bm *BufferManager) beginOperation() bool {
	bm.lifecycleMu.Lock()
	defer bm.lifecycleMu.Unlock()
	if !bm.accepting {
		return false
	}
	bm.operationWG.Add(1)
	return true
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

// Close gracefully shuts down the buffer manager. It stops new producers,
// waits for in-flight parquet writers, drains flushCh, and only then returns so
// the caller can safely close DuckDB. If ctx expires, pending parquet files are
// retained instead of being loaded, but lifecycle shutdown still completes.
func (bm *BufferManager) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	bm.stopOnce.Do(func() {
		log.Println("[buffer] Shutting down buffer manager...")

		bm.lifecycleMu.Lock()
		bm.accepting = false
		close(bm.stopCh)
		bm.lifecycleMu.Unlock()

		// If the graceful-shutdown budget expires, cancel DuckDB loads and
		// retries. The writer will still drain the channel, leaving those
		// parquet files on disk for recovery.
		if ctx.Err() != nil {
			bm.writerCancel()
		}
		stopDeadlineWatch := context.AfterFunc(ctx, bm.writerCancel)
		defer stopDeadlineWatch()

		// Stop the periodic flusher and wait for all Add/Flush calls that
		// started before accepting was disabled.
		bm.tickerWG.Wait()
		bm.operationWG.Wait()

		// Capture rows that did not reach a threshold, then wait for every
		// threshold-triggered parquet producer before closing its channel.
		bm.flushAll()
		bm.producerWG.Wait()

		// Close the flush channel to signal writer to drain and exit
		close(bm.flushCh)
		bm.writerWG.Wait()
		bm.writerCancel()

		if err := ctx.Err(); err != nil {
			bm.closeErr = fmt.Errorf("buffer shutdown exceeded graceful deadline; pending parquet files retained: %w", err)
		}
		log.Println("[buffer] Buffer manager stopped")
	})
	return bm.closeErr
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
	defer bm.writerWG.Done()

	const maxRetries = 3
	backoffs := [3]time.Duration{1 * time.Second, 5 * time.Second, 15 * time.Second}

	for job := range bm.flushCh {
		if job.done != nil {
			close(job.done)
			continue
		}
		if err := bm.writerCtx.Err(); err != nil {
			atomic.AddInt64(&bm.errorCount, 1)
			log.Printf("[buffer] Shutdown deadline reached; keeping parquet for %s: %s", job.Table, job.FilePath)
			continue
		}

		var err error
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				log.Printf("[buffer] Retrying load into %s (attempt %d/%d)", job.Table, attempt, maxRetries)
				timer := time.NewTimer(backoffs[attempt-1])
				select {
				case <-timer.C:
				case <-bm.writerCtx.Done():
					timer.Stop()
					err = bm.writerCtx.Err()
				}
				if err != nil {
					break
				}
			}

			bm.dbMu.RLock()
			err = bm.loadParquet(bm.writerCtx, job)
			bm.dbMu.RUnlock()

			if err == nil {
				break
			}
		}

		if err != nil {
			atomic.AddInt64(&bm.errorCount, 1)
			if bm.writerCtx.Err() != nil {
				// Shutdown interrupted the load; keep the file for startup recovery.
				log.Printf("[buffer] Shutdown interrupted load into %s; parquet kept for recovery: %s", job.Table, job.FilePath)
				continue
			}
			// All attempts exhausted on a live database: quarantine the file so it
			// does not poison the recovery scan on every startup.
			failedPath := job.FilePath + ".failed"
			if renameErr := os.Rename(job.FilePath, failedPath); renameErr != nil {
				log.Printf("[buffer] Failed to load parquet into %s after %d retries: %v (could not quarantine %s: %v)", job.Table, maxRetries, err, job.FilePath, renameErr)
			} else {
				log.Printf("[buffer] Failed to load parquet into %s after %d retries: %v (quarantined: %s)", job.Table, maxRetries, err, failedPath)
			}
			continue
		}
		if err := os.Remove(job.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			atomic.AddInt64(&bm.errorCount, 1)
			log.Printf("[buffer] Loaded %s but could not remove parquet %s: %v", job.Table, job.FilePath, err)
		}
	}
}

// tickerLoop checks buffer ages and triggers flushes when timeout expires.
func (bm *BufferManager) tickerLoop() {
	defer bm.tickerWG.Done()

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
func (bm *BufferManager) loadParquet(ctx context.Context, job FlushJob) error {
	if _, ok := recoverableTableForFile(job.Table + "_0.parquet"); !ok {
		return fmt.Errorf("unsupported buffer table %q", job.Table)
	}
	escapedPath := strings.ReplaceAll(job.FilePath, "'", "''")
	query := fmt.Sprintf("INSERT INTO %s BY NAME SELECT * FROM read_parquet('%s') ON CONFLICT DO NOTHING", job.Table, escapedPath)
	_, err := bm.db.ExecContext(ctx, query)
	return err
}
