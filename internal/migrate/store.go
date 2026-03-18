package migrate

import (
	"database/sql"
	"fmt"
	"time"
)

// Job represents an import job record.
type Job struct {
	ID            string  `json:"id"`
	Source        string  `json:"source"`
	Status        string  `json:"status"`
	Domain        string  `json:"domain"`
	FileName      string  `json:"file_name"`
	FileSize      int64   `json:"file_size"`
	RowsTotal     int64   `json:"rows_total"`
	RowsImported  int64   `json:"rows_imported"`
	RowsSkipped   int64   `json:"rows_skipped"`
	ErrorMessage  *string `json:"error_message"`
	ColumnMapping string  `json:"column_mapping"`
	Warnings      string  `json:"warnings"`
	StartedAt     *int64  `json:"started_at"`
	CompletedAt   *int64  `json:"completed_at"`
	CreatedBy     string  `json:"created_by"`
	CreatedAt     int64   `json:"created_at"`
}

// Store handles DB operations for import jobs.
type Store struct {
	db *sql.DB
}

// NewStore creates a new Store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new job.
func (s *Store) Create(j *Job) error {
	_, err := s.db.Exec(`
		INSERT INTO import_jobs (id, source, status, domain, file_name, file_size, rows_total, rows_imported, rows_skipped, error_message, column_mapping, warnings, started_at, completed_at, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.Source, j.Status, j.Domain, j.FileName, j.FileSize,
		j.RowsTotal, j.RowsImported, j.RowsSkipped, j.ErrorMessage,
		j.ColumnMapping, j.Warnings, j.StartedAt, j.CompletedAt, j.CreatedBy, j.CreatedAt,
	)
	return err
}

// Get retrieves a job by ID.
func (s *Store) Get(id string) (*Job, error) {
	j := &Job{}
	err := s.db.QueryRow(`
		SELECT id, source, status, domain, file_name, file_size, rows_total, rows_imported, rows_skipped, error_message, column_mapping, warnings, started_at, completed_at, created_by, created_at
		FROM import_jobs WHERE id = ?`, id,
	).Scan(&j.ID, &j.Source, &j.Status, &j.Domain, &j.FileName, &j.FileSize,
		&j.RowsTotal, &j.RowsImported, &j.RowsSkipped, &j.ErrorMessage,
		&j.ColumnMapping, &j.Warnings, &j.StartedAt, &j.CompletedAt, &j.CreatedBy, &j.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job not found")
	}
	return j, err
}

// List retrieves all jobs, most recent first.
func (s *Store) List() ([]*Job, error) {
	rows, err := s.db.Query(`
		SELECT id, source, status, domain, file_name, file_size, rows_total, rows_imported, rows_skipped, error_message, column_mapping, warnings, started_at, completed_at, created_by, created_at
		FROM import_jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		j := &Job{}
		if err := rows.Scan(&j.ID, &j.Source, &j.Status, &j.Domain, &j.FileName, &j.FileSize,
			&j.RowsTotal, &j.RowsImported, &j.RowsSkipped, &j.ErrorMessage,
			&j.ColumnMapping, &j.Warnings, &j.StartedAt, &j.CompletedAt, &j.CreatedBy, &j.CreatedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateStatus updates the status and optionally error/completion.
func (s *Store) UpdateStatus(id, status string, errMsg *string) error {
	now := time.Now().UnixMilli()
	if status == "running" {
		_, err := s.db.Exec("UPDATE import_jobs SET status = ?, started_at = ? WHERE id = ?", status, now, id)
		return err
	}
	if status == "completed" || status == "failed" || status == "cancelled" || status == "rolled_back" {
		_, err := s.db.Exec("UPDATE import_jobs SET status = ?, error_message = ?, completed_at = ? WHERE id = ?",
			status, errMsg, now, id)
		return err
	}
	_, err := s.db.Exec("UPDATE import_jobs SET status = ? WHERE id = ?", status, id)
	return err
}

// UpdateProgress updates row counters and optionally warnings.
func (s *Store) UpdateProgress(id string, imported, skipped, total int64, warnings string) error {
	_, err := s.db.Exec("UPDATE import_jobs SET rows_imported = ?, rows_skipped = ?, rows_total = ?, warnings = ? WHERE id = ?",
		imported, skipped, total, warnings, id)
	return err
}

// Delete removes a job record.
func (s *Store) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM import_jobs WHERE id = ?", id)
	return err
}
