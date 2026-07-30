package buffer

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/caioricciuti/etiquetta/internal/database"
)

type bufferTestDB struct {
	started   chan struct{}
	release   chan struct{}
	startOnce sync.Once
	execCount atomic.Int64
}

type bufferTestConnector struct {
	state *bufferTestDB
}

func (c *bufferTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &bufferTestConn{state: c.state}, nil
}

func (c *bufferTestConnector) Driver() driver.Driver {
	return bufferTestDriver{state: c.state}
}

type bufferTestDriver struct {
	state *bufferTestDB
}

func (d bufferTestDriver) Open(string) (driver.Conn, error) {
	return &bufferTestConn{state: d.state}, nil
}

type bufferTestConn struct {
	state *bufferTestDB
}

func (c *bufferTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}

func (c *bufferTestConn) Close() error { return nil }

func (c *bufferTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions not supported")
}

func (c *bufferTestConn) ExecContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.startOnce.Do(func() {
		if c.state.started != nil {
			close(c.state.started)
		}
	})

	if c.state.release != nil {
		select {
		case <-c.state.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	c.state.execCount.Add(1)
	return driver.RowsAffected(1), nil
}

func newBufferTestManager(t *testing.T, state *bufferTestDB, threshold int) (*BufferManager, string) {
	t.Helper()
	db := sql.OpenDB(&bufferTestConnector{state: state})
	t.Cleanup(func() { _ = db.Close() })
	tempDir := t.TempDir()
	mgr := NewBufferManager(db, BufferConfig{
		Threshold:    threshold,
		FlushTimeout: time.Hour,
		TempDir:      tempDir,
	})
	return mgr, tempDir
}

func testEvent(id string) Event {
	return Event{
		ID:          id,
		Timestamp:   1,
		EventType:   "pageview",
		SessionID:   "session-1",
		VisitorHash: "visitor-1",
		Domain:      "example.com",
		URL:         "https://example.com/",
		Path:        "/",
		Props:       "{}",
		BotSignals:  "[]",
		BotCategory: "human",
	}
}

func TestPendingParquetIsRecoveredIdempotentlyOnStartup(t *testing.T) {
	root := t.TempDir()
	db, err := database.New(filepath.Join(root, "etiquetta.duckdb"))
	if err != nil {
		t.Fatalf("open DuckDB: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate DuckDB: %v", err)
	}

	tempDir := filepath.Join(root, "buffer_tmp")
	event := testEvent("recovered-event")
	firstPath, err := writeParquet("events", []Event{event}, tempDir)
	if err != nil {
		t.Fatalf("write first pending parquet: %v", err)
	}

	newManager := func() *BufferManager {
		return NewBufferManager(db.Conn(), BufferConfig{
			Threshold:    100,
			FlushTimeout: time.Hour,
			TempDir:      tempDir,
		})
	}
	waitRecovered := func(filePath string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var count int
			queryErr := db.Conn().QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", event.ID).Scan(&count)
			_, statErr := os.Stat(filePath)
			if queryErr == nil && count == 1 && errors.Is(statErr, os.ErrNotExist) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("pending parquet %s was not recovered and removed", filePath)
	}

	mgr := newManager()
	waitRecovered(firstPath)
	if err := mgr.Close(context.Background()); err != nil {
		t.Fatalf("close first manager: %v", err)
	}

	// Simulate a crash after a successful insert but before removal. Recovery
	// must treat the primary-key conflict as already applied and remove the file.
	duplicatePath, err := writeParquet("events", []Event{event}, tempDir)
	if err != nil {
		t.Fatalf("write duplicate pending parquet: %v", err)
	}
	mgr = newManager()
	waitRecovered(duplicatePath)
	if err := mgr.Close(context.Background()); err != nil {
		t.Fatalf("close second manager: %v", err)
	}

	var count int
	if err := db.Conn().QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", event.ID).Scan(&count); err != nil {
		t.Fatalf("count recovered event: %v", err)
	}
	if count != 1 {
		t.Fatalf("recovered event count = %d, want 1", count)
	}
}

// TestFlushLoadsIntoConstraintlessTable reproduces the v2.0.0 regression where
// databases whose tables were created by an older schema (no PRIMARY KEY) could
// not ingest: DuckDB rejects "ON CONFLICT DO NOTHING" without a constraint, so
// every flush failed and was quarantined. The loader must fall back to a plain
// insert on such tables.
func TestFlushLoadsIntoConstraintlessTable(t *testing.T) {
	root := t.TempDir()
	db, err := database.New(filepath.Join(root, "etiquetta.duckdb"))
	if err != nil {
		t.Fatalf("open DuckDB: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate DuckDB: %v", err)
	}
	conn := db.Conn()

	// Recreate `events` with the same columns but no constraints (CTAS drops
	// PRIMARY KEY), simulating a database created by an older version.
	for _, stmt := range []string{
		`CREATE TABLE events_nopk AS SELECT * FROM events WHERE 1=0`,
		`DROP TABLE events`,
		`ALTER TABLE events_nopk RENAME TO events`,
	} {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("rebuild constraintless events (%q): %v", stmt, err)
		}
	}
	var constraints int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM information_schema.table_constraints
		 WHERE table_name = 'events' AND constraint_type IN ('PRIMARY KEY', 'UNIQUE')`,
	).Scan(&constraints); err != nil {
		t.Fatalf("check constraints: %v", err)
	}
	if constraints != 0 {
		t.Fatalf("test setup: events still has %d constraints, want 0", constraints)
	}

	tempDir := filepath.Join(root, "buffer_tmp")
	event := testEvent("constraintless-event")
	path, err := writeParquet("events", []Event{event}, tempDir)
	if err != nil {
		t.Fatalf("write pending parquet: %v", err)
	}

	mgr := NewBufferManager(conn, BufferConfig{
		Threshold:    100,
		FlushTimeout: time.Hour,
		TempDir:      tempDir,
	})
	defer mgr.Close(context.Background())

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		qErr := conn.QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", event.ID).Scan(&count)
		_, statErr := os.Stat(path)
		if qErr == nil && count == 1 && errors.Is(statErr, os.ErrNotExist) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("event never loaded into constraintless events table (the v2.0.0 ON CONFLICT regression)")
}

func TestStartupRemovesTruncatedTempAndIgnoresQuarantined(t *testing.T) {
	state := &bufferTestDB{}
	db := sql.OpenDB(&bufferTestConnector{state: state})
	t.Cleanup(func() { _ = db.Close() })
	tempDir := t.TempDir()

	// A crash mid-writeParquet leaves a truncated .tmp file; a permanently
	// failed load leaves a quarantined .failed file. Neither may be re-queued.
	truncated := filepath.Join(tempDir, "events_1.parquet.tmp")
	if err := os.WriteFile(truncated, []byte("partial"), 0644); err != nil {
		t.Fatalf("write truncated temp: %v", err)
	}
	quarantined := filepath.Join(tempDir, "events_2.parquet.failed")
	if err := os.WriteFile(quarantined, []byte("bad"), 0644); err != nil {
		t.Fatalf("write quarantined file: %v", err)
	}

	mgr := NewBufferManager(db, BufferConfig{
		Threshold:    100,
		FlushTimeout: time.Hour,
		TempDir:      tempDir,
	})
	if err := mgr.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := os.Stat(truncated); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("truncated temp file was not removed: %v", err)
	}
	if _, err := os.Stat(quarantined); err != nil {
		t.Fatalf("quarantined file should be left untouched: %v", err)
	}
	if got := state.execCount.Load(); got != 0 {
		t.Fatalf("startup loaded leftover files into DB: %d loads", got)
	}
}

func TestFlushAndWaitDrainsQueuedLoads(t *testing.T) {
	state := &bufferTestDB{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	mgr, _ := newBufferTestManager(t, state, 100)
	mgr.AddEvent(context.Background(), testEvent("event-1"))

	done := make(chan error, 1)
	go func() { done <- mgr.FlushAndWait(context.Background()) }()

	select {
	case <-state.started:
	case <-time.After(5 * time.Second):
		t.Fatal("FlushAndWait did not reach the writer")
	}
	select {
	case err := <-done:
		t.Fatalf("FlushAndWait returned before writer drained: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(state.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("FlushAndWait() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("FlushAndWait did not return after writer drained")
	}
	if got := state.execCount.Load(); got != 1 {
		t.Fatalf("DuckDB loads = %d, want 1", got)
	}
	if err := mgr.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestCloseFlushesBufferedRowsBeforeReturning(t *testing.T) {
	state := &bufferTestDB{}
	mgr, tempDir := newBufferTestManager(t, state, 100)
	mgr.AddEvent(context.Background(), testEvent("event-1"))

	if err := mgr.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := state.execCount.Load(); got != 1 {
		t.Fatalf("DuckDB loads = %d, want 1", got)
	}
	files, err := filepath.Glob(filepath.Join(tempDir, "*.parquet"))
	if err != nil {
		t.Fatalf("glob parquet files: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("successful shutdown left parquet files: %v", files)
	}

	// Calls that arrive after shutdown are safely rejected instead of sending
	// to the closed writer channel.
	mgr.AddEvent(context.Background(), testEvent("event-after-close"))
	if got := mgr.Stats().Events.BufferedRows; got != 0 {
		t.Fatalf("buffered rows after Close = %d, want 0", got)
	}
}

func TestCloseWaitsForWriterDrain(t *testing.T) {
	state := &bufferTestDB{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	mgr, _ := newBufferTestManager(t, state, 1)
	mgr.AddEvent(context.Background(), testEvent("event-1"))

	select {
	case <-state.started:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not start")
	}

	closed := make(chan error, 1)
	go func() { closed <- mgr.Close(context.Background()) }()

	select {
	case err := <-closed:
		t.Fatalf("Close returned before writer drained: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(state.release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after writer drained")
	}
}

func TestCloseRetainsParquetWhenContextAlreadyCanceled(t *testing.T) {
	state := &bufferTestDB{}
	mgr, tempDir := newBufferTestManager(t, state, 100)
	mgr.AddEvent(context.Background(), testEvent("event-1"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := mgr.Close(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Close() error = %v, want context.Canceled", err)
	}
	if got := state.execCount.Load(); got != 0 {
		t.Fatalf("DuckDB loads after cancellation = %d, want 0", got)
	}
	files, globErr := filepath.Glob(filepath.Join(tempDir, "*.parquet"))
	if globErr != nil {
		t.Fatalf("glob parquet files: %v", globErr)
	}
	if len(files) != 1 {
		t.Fatalf("retained parquet files = %v, want exactly one", files)
	}
}

func TestConcurrentAddsAreRejectedSafelyDuringClose(t *testing.T) {
	state := &bufferTestDB{}
	mgr, _ := newBufferTestManager(t, state, 5)

	const producers = 4
	started := make(chan struct{}, producers)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for producer := 0; producer < producers; producer++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			mgr.AddEvent(context.Background(), testEvent("initial"))
			started <- struct{}{}
			for {
				select {
				case <-stop:
					return
				default:
					mgr.AddEvent(context.Background(), testEvent("concurrent"))
					runtime.Gosched()
				}
			}
		}(producer)
	}
	for producer := 0; producer < producers; producer++ {
		<-started
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.Close(ctx); err != nil {
		close(stop)
		wg.Wait()
		t.Fatalf("Close() error = %v", err)
	}
	close(stop)
	wg.Wait()

	if got := mgr.Stats().Events.BufferedRows; got != 0 {
		t.Fatalf("buffered rows after Close = %d, want 0", got)
	}
}

var (
	_ driver.Connector     = (*bufferTestConnector)(nil)
	_ driver.Driver        = bufferTestDriver{}
	_ driver.Conn          = (*bufferTestConn)(nil)
	_ driver.ExecerContext = (*bufferTestConn)(nil)
	_ io.Closer            = (*bufferTestConn)(nil)
)
