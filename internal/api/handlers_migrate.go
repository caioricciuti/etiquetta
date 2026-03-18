package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/migrate"
)

// MigrateAnalyze handles POST /api/migrate/analyze
// Accepts multipart form with "file" field and optional "source" field.
func (h *Handlers) MigrateAnalyze(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Failed to parse multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing file field")
		return
	}
	defer file.Close()

	source := r.FormValue("source")

	// Ensure temp dir exists
	tmpDir := h.migrateManager.TempDir()
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create temp directory")
		return
	}

	analysisID := generateID()
	filePath := tmpDir + "/" + analysisID

	dst, err := os.Create(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save uploaded file")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		os.Remove(filePath)
		writeError(w, http.StatusInternalServerError, "Failed to write file")
		return
	}
	dst.Close()

	detection, err := h.migrateManager.AnalyzeFile(filePath, source)
	if err != nil {
		os.Remove(filePath)
		writeError(w, http.StatusBadRequest, "Analysis failed: "+err.Error())
		return
	}

	log.Printf("[migrate] Analyzed file %s (%d bytes), detected source: %s", header.Filename, header.Size, detection.Source)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"analysis_id":      analysisID,
		"source":           detection.Source,
		"columns":          detection.Columns,
		"sample_rows":      detection.SampleRows,
		"row_estimate":     detection.RowEstimate,
		"date_range":       detection.DateRange,
		"suggested_mapping": detection.SuggestedMapping,
	})
}

// MigrateStart handles POST /api/migrate/start
// Accepts JSON body with analysis_id, source, domain, column_mapping.
func (h *Handlers) MigrateStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AnalysisID    string            `json:"analysis_id"`
		Source        string            `json:"source"`
		Domain        string            `json:"domain"`
		ColumnMapping map[string]string `json:"column_mapping"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if body.AnalysisID == "" || body.Source == "" || body.Domain == "" {
		writeError(w, http.StatusBadRequest, "analysis_id, source, and domain are required")
		return
	}

	// Verify the temp file exists
	filePath := h.migrateManager.TempDir() + "/" + body.AnalysisID
	info, err := os.Stat(filePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Analysis file not found, re-upload required")
		return
	}

	mappingJSON, _ := json.Marshal(body.ColumnMapping)

	claims := auth.GetUserFromContext(r.Context())
	createdBy := ""
	if claims != nil {
		createdBy = claims.Email
	}

	jobID := generateID()
	now := time.Now().UnixMilli()

	job := &migrate.Job{
		ID:            jobID,
		Source:        body.Source,
		Status:        "pending",
		Domain:        body.Domain,
		FileName:      body.AnalysisID,
		FileSize:      info.Size(),
		RowsTotal:     0,
		RowsImported:  0,
		RowsSkipped:   0,
		ColumnMapping: string(mappingJSON),
		Warnings:      "[]",
		CreatedBy:     createdBy,
		CreatedAt:     now,
	}

	if err := h.migrateManager.Store().Create(job); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create job: "+err.Error())
		return
	}

	h.migrateManager.RunJob(jobID, filePath)
	h.logAudit(r, "create", "import_job", jobID, "source="+body.Source+" domain="+body.Domain)

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// MigrateListJobs handles GET /api/migrate/jobs
func (h *Handlers) MigrateListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.migrateManager.Store().List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list jobs: "+err.Error())
		return
	}
	if jobs == nil {
		jobs = []*migrate.Job{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

// MigrateGetJob handles GET /api/migrate/jobs/{id}
func (h *Handlers) MigrateGetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.migrateManager.Store().Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// MigrateRollback handles DELETE /api/migrate/jobs/{id}
func (h *Handlers) MigrateRollback(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.migrateManager.Rollback(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.logAudit(r, "rollback", "import_job", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "rolled_back"})
}

// MigrateCancelJob handles POST /api/migrate/jobs/{id}/cancel
func (h *Handlers) MigrateCancelJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.migrateManager.CancelJob(id) {
		writeError(w, http.StatusBadRequest, "Job is not running")
		return
	}
	h.logAudit(r, "cancel", "import_job", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelling"})
}

// MigrateGTMConvert handles POST /api/migrate/gtm/convert
// Accepts multipart form with "file" field containing GTM container JSON.
func (h *Handlers) MigrateGTMConvert(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Failed to parse multipart form: "+err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing file field")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to read file")
		return
	}

	result, err := migrate.ConvertGTMContainer(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "GTM conversion failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}
