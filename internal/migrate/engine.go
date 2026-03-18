package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/caioricciuti/etiquetta/internal/buffer"
)

// JobManager manages import jobs.
type JobManager struct {
	store     *Store
	bufferMgr *buffer.BufferManager
	dataDir   string
	mu        sync.Mutex
	running   map[string]context.CancelFunc
}

// NewJobManager creates a new JobManager.
func NewJobManager(store *Store, bufferMgr *buffer.BufferManager, dataDir string) *JobManager {
	return &JobManager{
		store:     store,
		bufferMgr: bufferMgr,
		dataDir:   dataDir,
		running:   make(map[string]context.CancelFunc),
	}
}

// Store returns the underlying store for direct queries.
func (jm *JobManager) Store() *Store {
	return jm.store
}

// TempDir returns the path for temporary import files.
func (jm *JobManager) TempDir() string {
	return jm.dataDir + "/migrate_tmp"
}

// RunJob starts an import job in the background.
func (jm *JobManager) RunJob(jobID, filePath string) {
	ctx, cancel := context.WithCancel(context.Background())

	jm.mu.Lock()
	jm.running[jobID] = cancel
	jm.mu.Unlock()

	go func() {
		defer func() {
			jm.mu.Lock()
			delete(jm.running, jobID)
			jm.mu.Unlock()
			cancel()
		}()

		jm.executeJob(ctx, jobID, filePath)
	}()
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
	cancel, ok := jm.running[jobID]
	jm.mu.Unlock()
	if ok {
		cancel()
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

	// Delete events - use the store's DB
	_, err = jm.store.db.Exec("DELETE FROM events WHERE import_id = ?", jobID)
	if err != nil {
		return fmt.Errorf("failed to delete imported events: %w", err)
	}

	return jm.store.UpdateStatus(jobID, "rolled_back", nil)
}

// Shutdown cancels all running jobs.
func (jm *JobManager) Shutdown() {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	for id, cancel := range jm.running {
		log.Printf("[migrate] Cancelling job %s on shutdown", id)
		cancel()
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
