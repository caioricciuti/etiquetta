package migrate

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/caioricciuti/etiquetta/internal/buffer"
	"github.com/caioricciuti/etiquetta/internal/database"
)

func newLifecycleTestManager(t *testing.T, run func(context.Context, string, string)) *JobManager {
	t.Helper()
	jm := NewJobManager(nil, nil, t.TempDir())
	jm.run = run
	return jm
}

func TestShutdownCancelsAndJoinsRunningJobs(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	jm := newLifecycleTestManager(t, func(ctx context.Context, _, _ string) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
	})

	if err := jm.RunJob("job-1", "unused"); err != nil {
		t.Fatalf("RunJob() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("job did not start")
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- jm.Shutdown(context.Background()) }()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not cancel the job")
	}
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned before job cleanup completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not join the job")
	}

	if err := jm.RunJob("job-after-stop", "unused"); !errors.Is(err, ErrJobManagerShuttingDown) {
		t.Fatalf("RunJob() after shutdown error = %v, want ErrJobManagerShuttingDown", err)
	}
}

func TestShutdownDeadlineCanBeFollowedByFullJoin(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	jm := newLifecycleTestManager(t, func(context.Context, string, string) {
		close(started)
		<-release
	})

	if err := jm.RunJob("job-1", "unused"); err != nil {
		t.Fatalf("RunJob() error = %v", err)
	}
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := jm.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want context.DeadlineExceeded", err)
	}

	close(release)
	if err := jm.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}

func TestRollbackDeletesEventsStillInBuffer(t *testing.T) {
	root := t.TempDir()
	db, err := database.New(filepath.Join(root, "etiquetta.duckdb"))
	if err != nil {
		t.Fatalf("open DuckDB: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate DuckDB: %v", err)
	}

	bm := buffer.NewBufferManager(db.Conn(), buffer.BufferConfig{
		Threshold:    100,
		FlushTimeout: time.Hour,
		TempDir:      filepath.Join(root, "buffer_tmp"),
	})
	defer bm.Close(context.Background())

	store := NewStore(db.Conn())
	jm := NewJobManager(store, bm, root)

	const jobID = "rollback-job"
	if err := store.Create(&Job{
		ID:            jobID,
		Source:        SourceCSV,
		Status:        "completed",
		Domain:        "example.com",
		ColumnMapping: "{}",
		Warnings:      "[]",
		CreatedAt:     time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("create job: %v", err)
	}

	// The imported row sits in the in-memory buffer, well below the flush
	// threshold; a prompt rollback must still delete it from DuckDB.
	bm.AddEvent(context.Background(), buffer.Event{
		ID:          "imported-event",
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
		ImportID:    jobID,
	})

	if err := jm.Rollback(jobID); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	var count int
	if err := db.Conn().QueryRow("SELECT COUNT(*) FROM events WHERE import_id = ?", jobID).Scan(&count); err != nil {
		t.Fatalf("count imported events: %v", err)
	}
	if count != 0 {
		t.Fatalf("imported events after rollback = %d, want 0", count)
	}
	job, err := store.Get(jobID)
	if err != nil {
		t.Fatalf("get job after rollback: %v", err)
	}
	if job.Status != "rolled_back" {
		t.Fatalf("job status = %q, want rolled_back", job.Status)
	}
}

func TestRunJobRejectsDuplicateRunningID(t *testing.T) {
	started := make(chan struct{})
	jm := newLifecycleTestManager(t, func(ctx context.Context, _, _ string) {
		close(started)
		<-ctx.Done()
	})

	if err := jm.RunJob("job-1", "unused"); err != nil {
		t.Fatalf("first RunJob() error = %v", err)
	}
	<-started
	if err := jm.RunJob("job-1", "unused"); !errors.Is(err, ErrJobAlreadyRunning) {
		t.Fatalf("duplicate RunJob() error = %v, want ErrJobAlreadyRunning", err)
	}
	if !jm.CancelJob("job-1") {
		t.Fatal("CancelJob returned false for running job")
	}
	if !jm.CancelJob("job-1") {
		t.Fatal("repeated CancelJob should remain idempotently successful until job exits")
	}
	if err := jm.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestRunJobRacingWithShutdownIsAlwaysOwned(t *testing.T) {
	jm := newLifecycleTestManager(t, func(ctx context.Context, _, _ string) {
		<-ctx.Done()
	})

	const attempts = 32
	start := make(chan struct{})
	var callers sync.WaitGroup
	callers.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(id int) {
			defer callers.Done()
			<-start
			_ = jm.RunJob("job-"+strconv.Itoa(id), "unused")
		}(i)
	}
	close(start)
	if err := jm.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	callers.Wait()

	jm.mu.Lock()
	running := len(jm.running)
	jm.mu.Unlock()
	if running != 0 {
		t.Fatalf("running jobs after Shutdown = %d, want 0", running)
	}
}
