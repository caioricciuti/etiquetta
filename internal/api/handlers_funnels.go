package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/go-chi/chi/v5"
)

type FunnelStep struct {
	EventType string `json:"event_type"` // "pageview" or "custom"
	EventName string `json:"event_name"` // event name for custom events
	PagePath  string `json:"page_path"`  // path pattern for pageview steps
}

type Funnel struct {
	ID          string       `json:"id"`
	DomainID    string       `json:"domain_id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Steps       []FunnelStep `json:"steps"`
	CreatedBy   string       `json:"created_by"`
	CreatedAt   int64        `json:"created_at"`
	UpdatedAt   int64        `json:"updated_at"`
}

type FunnelStepMetric struct {
	Step        int     `json:"step"`
	Name        string  `json:"name"`
	Visitors    int     `json:"visitors"`
	Completions int     `json:"completions"`
	DropOff     int     `json:"drop_off"`
	Rate        float64 `json:"rate"`
}

// ListFunnels returns funnels for a domain
func (h *Handlers) ListFunnels(w http.ResponseWriter, r *http.Request) {
	domainID := r.URL.Query().Get("domain_id")
	if domainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}

	rows, err := h.db.Conn().QueryContext(r.Context(),
		"SELECT id, domain_id, name, description, steps, created_by, created_at, updated_at FROM funnels WHERE domain_id = ? ORDER BY created_at DESC",
		domainID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query funnels")
		return
	}
	defer rows.Close()

	var funnels []Funnel
	for rows.Next() {
		var f Funnel
		var stepsJSON string
		if err := rows.Scan(&f.ID, &f.DomainID, &f.Name, &f.Description, &stepsJSON, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
			continue
		}
		json.Unmarshal([]byte(stepsJSON), &f.Steps)
		funnels = append(funnels, f)
	}

	if funnels == nil {
		funnels = []Funnel{}
	}
	writeJSON(w, http.StatusOK, funnels)
}

// CreateFunnel creates a new funnel
func (h *Handlers) CreateFunnel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DomainID    string       `json:"domain_id"`
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Steps       []FunnelStep `json:"steps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DomainID == "" || req.Name == "" || len(req.Steps) < 2 {
		writeError(w, http.StatusBadRequest, "domain_id, name, and at least 2 steps are required")
		return
	}

	claims := auth.GetUserFromContext(r.Context())
	createdBy := ""
	if claims != nil {
		createdBy = claims.UserID
	}

	stepsJSON, _ := json.Marshal(req.Steps)
	id := generateID()
	now := time.Now().UnixMilli()

	_, err := h.db.Conn().ExecContext(r.Context(),
		"INSERT INTO funnels (id, domain_id, name, description, steps, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		id, req.DomainID, req.Name, req.Description, string(stepsJSON), createdBy, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create funnel")
		return
	}

	h.logAudit(r, "create", "funnel", id, req.Name)

	writeJSON(w, http.StatusCreated, Funnel{
		ID:          id,
		DomainID:    req.DomainID,
		Name:        req.Name,
		Description: req.Description,
		Steps:       req.Steps,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

// UpdateFunnel updates an existing funnel
func (h *Handlers) UpdateFunnel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Steps       []FunnelStep `json:"steps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || len(req.Steps) < 2 {
		writeError(w, http.StatusBadRequest, "name and at least 2 steps are required")
		return
	}

	stepsJSON, _ := json.Marshal(req.Steps)
	now := time.Now().UnixMilli()

	res, err := h.db.Conn().ExecContext(r.Context(),
		"UPDATE funnels SET name = ?, description = ?, steps = ?, updated_at = ? WHERE id = ?",
		req.Name, req.Description, string(stepsJSON), now, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update funnel")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "funnel not found")
		return
	}

	h.logAudit(r, "update", "funnel", id, req.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteFunnel deletes a funnel
func (h *Handlers) DeleteFunnel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	res, err := h.db.Conn().ExecContext(r.Context(), "DELETE FROM funnels WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete funnel")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "funnel not found")
		return
	}

	h.logAudit(r, "delete", "funnel", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetFunnelMetrics calculates funnel conversion rates by querying the events table.
// For each step, it counts unique visitors who completed that step AND all previous steps in order.
func (h *Handlers) GetFunnelMetrics(w http.ResponseWriter, r *http.Request) {
	funnelID := chi.URLParam(r, "id")
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	if start == "" || end == "" {
		writeError(w, http.StatusBadRequest, "start and end are required")
		return
	}

	// Load funnel definition
	var stepsJSON string
	var funnelName string
	err := h.db.Conn().QueryRowContext(r.Context(),
		"SELECT name, steps FROM funnels WHERE id = ?", funnelID,
	).Scan(&funnelName, &stepsJSON)
	if err != nil {
		writeError(w, http.StatusNotFound, "funnel not found")
		return
	}

	var steps []FunnelStep
	json.Unmarshal([]byte(stepsJSON), &steps)
	if len(steps) < 2 {
		writeError(w, http.StatusBadRequest, "funnel has fewer than 2 steps")
		return
	}

	startMs, _ := time.Parse(time.RFC3339, start)
	endMs, _ := time.Parse(time.RFC3339, end)

	// Build step queries using CTEs for sequential funnel analysis.
	// Each step CTE finds visitors who matched that step AFTER their previous step.
	var ctes []string
	var args []interface{}

	for i, step := range steps {
		var condition string
		if step.EventType == "pageview" {
			condition = "event_type = 'pageview' AND path = ?"
			args = append(args, step.PagePath)
		} else {
			condition = "event_type = 'custom' AND event_name = ?"
			args = append(args, step.EventName)
		}

		if i == 0 {
			ctes = append(ctes, fmt.Sprintf(
				`step%d AS (
					SELECT DISTINCT visitor_hash, MIN(timestamp) AS ts
					FROM events
					WHERE %s AND is_bot = 0 AND timestamp BETWEEN ? AND ?
					GROUP BY visitor_hash
				)`, i, condition))
			args = append(args, startMs.UnixMilli(), endMs.UnixMilli())
		} else {
			ctes = append(ctes, fmt.Sprintf(
				`step%d AS (
					SELECT DISTINCT e.visitor_hash, MIN(e.timestamp) AS ts
					FROM events e
					INNER JOIN step%d prev ON e.visitor_hash = prev.visitor_hash AND e.timestamp > prev.ts
					WHERE %s AND e.is_bot = 0 AND e.timestamp BETWEEN ? AND ?
					GROUP BY e.visitor_hash
				)`, i, i-1, condition))
			args = append(args, startMs.UnixMilli(), endMs.UnixMilli())
		}
	}

	// Build final SELECT counting visitors per step
	var selects []string
	for i := range steps {
		selects = append(selects, fmt.Sprintf("(SELECT COUNT(*) FROM step%d) AS step%d_count", i, i))
	}

	query := "WITH " + strings.Join(ctes, ", ") + " SELECT " + strings.Join(selects, ", ")

	row := h.db.Conn().QueryRowContext(r.Context(), query, args...)

	// Scan results
	counts := make([]int, len(steps))
	ptrs := make([]interface{}, len(steps))
	for i := range counts {
		ptrs[i] = &counts[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to compute funnel metrics")
		return
	}

	// Build response
	metrics := make([]FunnelStepMetric, len(steps))
	for i, step := range steps {
		name := step.PagePath
		if step.EventType == "custom" {
			name = step.EventName
		}

		dropOff := 0
		if i > 0 {
			dropOff = counts[i-1] - counts[i]
		}

		rate := 0.0
		if counts[0] > 0 {
			rate = float64(counts[i]) / float64(counts[0]) * 100
		}

		metrics[i] = FunnelStepMetric{
			Step:        i + 1,
			Name:        name,
			Visitors:    counts[i],
			Completions: counts[i],
			DropOff:     dropOff,
			Rate:        rate,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"funnel_id": funnelID,
		"name":      funnelName,
		"steps":     metrics,
	})
}
