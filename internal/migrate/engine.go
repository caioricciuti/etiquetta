package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/caioricciuti/etiquetta/internal/buffer"
)

var (
	// ErrJobManagerShuttingDown is returned when a new import is submitted
	// after shutdown has started.
	ErrJobManagerShuttingDown = errors.New("import job manager is shutting down")
	// ErrJobAlreadyRunning is returned if the same job ID is started twice.
	ErrJobAlreadyRunning = errors.New("import job is already running")
)

type runningJob struct {
	cancel context.CancelFunc
}

// JobManager manages import jobs.
type JobManager struct {
	store     *Store
	bufferMgr *buffer.BufferManager
	dataDir   string
	mu        sync.Mutex
	running   map[string]*runningJob
	stopping  bool
	wg        sync.WaitGroup
	stopDone  chan struct{}
	run       func(context.Context, string, string)
}

// NewJobManager creates a new JobManager.
func NewJobManager(store *Store, bufferMgr *buffer.BufferManager, dataDir string) *JobManager {
	jm := &JobManager{
		store:     store,
		bufferMgr: bufferMgr,
		dataDir:   dataDir,
		running:   make(map[string]*runningJob),
		stopDone:  make(chan struct{}),
	}
	jm.run = jm.executeJob
	return jm
}

// Store returns the underlying store for direct queries.
func (jm *JobManager) Store() *Store {
	return jm.store
}

// TempDir returns the path for temporary import files.
func (jm *JobManager) TempDir() string {
	return jm.dataDir + "/migrate_tmp"
}

// RunJob starts an import job in the background. Registration and WaitGroup
// ownership happen before the goroutine starts, so Shutdown cannot miss a job
// racing with it.
func (jm *JobManager) RunJob(jobID, filePath string) error {
	ctx, cancel := context.WithCancel(context.Background())
	running := &runningJob{cancel: cancel}

	jm.mu.Lock()
	if jm.stopping {
		jm.mu.Unlock()
		cancel()
		return ErrJobManagerShuttingDown
	}
	if _, exists := jm.running[jobID]; exists {
		jm.mu.Unlock()
		cancel()
		return fmt.Errorf("%w: %s", ErrJobAlreadyRunning, jobID)
	}
	jm.running[jobID] = running
	jm.wg.Add(1)
	jm.mu.Unlock()

	go func() {
		defer jm.wg.Done()
		defer func() {
			jm.mu.Lock()
			if jm.running[jobID] == running {
				delete(jm.running, jobID)
			}
			jm.mu.Unlock()
			cancel()
		}()

		jm.run(ctx, jobID, filePath)
	}()

	return nil
}

func (jm *JobManager) executeJob(ctx context.Context, jobID, filePath string) {
	job, err := jm.store.Get(jobID)
	if err != nil {
		log.Printf("[migrate] Failed to load job %s: %v", jobID, err)
		return
	}

	// Set running
	if err := jm.store.UpdateStatus(jobID, "running", nil); err != nil {
		log.Printf("[migrate] Failed to update status for job %s: %v", jobID, err)
		return
	}

	// Open file
	f, err := os.Open(filePath)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to open file: %v", err)
		jm.store.UpdateStatus(jobID, "failed", &errMsg)
		return
	}
	defer f.Close()

	// Get parser
	parser := GetParser(job.Source)
	if parser == nil {
		errMsg := fmt.Sprintf("Unsupported source: %s", job.Source)
		jm.store.UpdateStatus(jobID, "failed", &errMsg)
		return
	}

	// Parse column mapping
	var mapping map[string]string
	if err := json.Unmarshal([]byte(job.ColumnMapping), &mapping); err != nil {
		mapping = make(map[string]string)
	}

	opts := ParseOpts{
		Domain:   job.Domain,
		ImportID: jobID,
		Mapping:  mapping,
		Timezone: "UTC",
	}

	var imported, skipped int64
	var warnings []string
	const progressInterval = 5000

	err = parser.Parse(ctx, f, opts, func(events []buffer.Event) error {
		// Check cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		for _, e := range events {
			jm.bufferMgr.AddEvent(ctx, e)
			imported++

			if imported%progressInterval == 0 {
				warningsJSON, _ := json.Marshal(warnings)
				jm.store.UpdateProgress(jobID, imported, skipped, job.RowsTotal, string(warningsJSON))
			}
		}
		return nil
	})

	if err != nil {
		if ctx.Err() != nil {
			jm.store.UpdateStatus(jobID, "cancelled", nil)
		} else {
			errMsg := err.Error()
			jm.store.UpdateStatus(jobID, "failed", &errMsg)
		}
		return
	}

	// Final progress update
	warningsJSON, _ := json.Marshal(warnings)
	jm.store.UpdateProgress(jobID, imported, skipped, imported+skipped, string(warningsJSON))
	jm.store.UpdateStatus(jobID, "completed", nil)

	// Clean up temp file
	os.Remove(filePath)
	log.Printf("[migrate] Job %s completed: %d imported, %d skipped", jobID, imported, skipped)
}

// CancelJob cancels a running job.
func (jm *JobManager) CancelJob(jobID string) bool {
	jm.mu.Lock()
	running, ok := jm.running[jobID]
	jm.mu.Unlock()
	if ok {
		running.cancel()
		return true
	}
	return false
}

// Rollback deletes all events associated with an import job.
func (jm *JobManager) Rollback(jobID string) error {
	job, err := jm.store.Get(jobID)
	if err != nil {
		return err
	}
	if job.Status == "running" {
		return fmt.Errorf("cannot rollback a running job, cancel it first")
	}

	// Imported rows may still sit in the in-memory buffer or the parquet write
	// queue; flush and drain so the DELETE below actually sees them.
	if jm.bufferMgr != nil {
		if err := jm.bufferMgr.FlushAndWait(context.Background()); err != nil {
			return fmt.Errorf("failed to flush buffered events before rollback: %w", err)
		}
	}

	// Delete events - use the store's DB
	_, err = jm.store.db.Exec("DELETE FROM events WHERE import_id = ?", jobID)
	if err != nil {
		return fmt.Errorf("failed to delete imported events: %w", err)
	}

	return jm.store.UpdateStatus(jobID, "rolled_back", nil)
}

// Shutdown prevents new jobs, cancels every registered job, and waits for all
// job goroutines to stop. A deadline only bounds this call; jobs remain owned
// by the manager and a later Shutdown call can continue waiting for stopDone.
func (jm *JobManager) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	jm.mu.Lock()
	firstShutdown := !jm.stopping
	jm.stopping = true
	running := make(map[string]*runningJob, len(jm.running))
	for id, job := range jm.running {
		running[id] = job
	}
	jm.mu.Unlock()

	for id, job := range running {
		if firstShutdown {
			log.Printf("[migrate] Cancelling job %s on shutdown", id)
		}
		job.cancel()
	}

	if firstShutdown {
		go func() {
			jm.wg.Wait()
			close(jm.stopDone)
		}()
	}

	select {
	case <-jm.stopDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("waiting for import jobs to stop: %w", ctx.Err())
	}
}

// AnalyzeFile auto-detects the file format and returns a Detection.
func (jm *JobManager) AnalyzeFile(filePath, source string) (*Detection, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	// If source is specified, use that parser
	if source != "" && source != "auto" {
		p := GetParser(source)
		if p == nil {
			return nil, fmt.Errorf("unsupported source: %s", source)
		}
		det, err := p.Detect(f)
		if err != nil {
			return nil, err
		}
		det.Source = source
		return det, nil
	}

	// Auto-detect: try each parser
	parsers := []struct {
		name   string
		parser Parser
	}{
		{SourcePlausible, &PlausibleParser{}},
		{SourceGA4CSV, &GA4CSVParser{}},
		{SourceMatomo, &MatomoParser{}},
		{SourceUmami, &UmamiParser{}},
		{SourceGA4BigQuery, &GA4BigQueryParser{}},
		{SourceCSV, &GenericCSVParser{}}, // generic last
	}

	for _, p := range parsers {
		f.Seek(0, 0)
		det, err := p.parser.Detect(f)
		if err == nil && det != nil {
			det.Source = p.name
			return det, nil
		}
	}

	return nil, fmt.Errorf("could not detect file format")
}
