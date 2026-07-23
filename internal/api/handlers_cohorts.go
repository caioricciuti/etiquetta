package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/go-chi/chi/v5"
)

// CohortDefinition captures the filter clauses that define a cohort.
// All non-empty fields are AND-ed together.
type CohortDefinition struct {
	UTMSource        string `json:"utm_source,omitempty"`
	UTMMedium        string `json:"utm_medium,omitempty"`
	UTMCampaign      string `json:"utm_campaign,omitempty"`
	ReferrerContains string `json:"referrer_contains,omitempty"`
	PathContains     string `json:"path_contains,omitempty"`
	Country          string `json:"country,omitempty"`
}

type Cohort struct {
	ID          string           `json:"id"`
	DomainID    string           `json:"domain_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Definition  CohortDefinition `json:"definition"`
	CreatedBy   string           `json:"created_by"`
	CreatedAt   int64            `json:"created_at"`
	UpdatedAt   int64            `json:"updated_at"`
}

// RetentionResponse is a grid of return rates per cohort day.
// Cohorts: rows = first_seen date; columns = days since first_seen.
type RetentionResponse struct {
	Days    int               `json:"days"`
	Cohorts []RetentionCohort `json:"cohorts"`
}

type RetentionCohort struct {
	Date       string    `json:"date"`
	Size       int       `json:"size"`
	Returns    []int     `json:"returns"`
	Rates      []float64 `json:"rates"`
}

func (h *Handlers) ListCohorts(w http.ResponseWriter, r *http.Request) {
	domainID := r.URL.Query().Get("domain_id")
	if domainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}
	rows, err := h.db.Conn().QueryContext(r.Context(), `
		SELECT id, domain_id, name, description, definition, created_by, created_at, updated_at
		FROM cohorts WHERE domain_id = ? ORDER BY created_at DESC
	`, domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query cohorts")
		return
	}
	defer rows.Close()

	out := []Cohort{}
	for rows.Next() {
		var c Cohort
		var defJSON string
		if err := rows.Scan(&c.ID, &c.DomainID, &c.Name, &c.Description, &defJSON,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(defJSON), &c.Definition)
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) CreateCohort(w http.ResponseWriter, r *http.Request) {
	var req Cohort
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DomainID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "domain_id and name are required")
		return
	}

	claims := auth.GetUserFromContext(r.Context())
	createdBy := ""
	if claims != nil {
		createdBy = claims.UserID
	}
	id := generateID()
	now := time.Now().UnixMilli()
	defJSON, _ := json.Marshal(req.Definition)

	_, err := h.db.Conn().ExecContext(r.Context(), `
		INSERT INTO cohorts (id, domain_id, name, description, definition,
			created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.DomainID, req.Name, req.Description, string(defJSON),
		createdBy, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create cohort")
		return
	}

	h.logAudit(r, "create", "cohort", id, req.Name)

	req.ID = id
	req.CreatedBy = createdBy
	req.CreatedAt = now
	req.UpdatedAt = now
	writeJSON(w, http.StatusCreated, req)
}

func (h *Handlers) UpdateCohort(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req Cohort
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	defJSON, _ := json.Marshal(req.Definition)
	now := time.Now().UnixMilli()
	res, err := h.db.Conn().ExecContext(r.Context(), `
		UPDATE cohorts SET name = ?, description = ?, definition = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Description, string(defJSON), now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update cohort")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "cohort not found")
		return
	}
	h.logAudit(r, "update", "cohort", id, req.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) DeleteCohort(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := h.db.Conn().ExecContext(r.Context(), "DELETE FROM cohorts WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete cohort")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "cohort not found")
		return
	}
	h.logAudit(r, "delete", "cohort", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetRetention returns a retention grid: for each first_seen date, the % of
// those visitors who returned on each subsequent day.
//
// Query params:
//   - domain_id (required)
//   - start, end (RFC3339 — range for the first_seen date)
//   - days (int — how many return-day columns, default 14, max 30)
//   - cohort_id (optional — filter first_seen events by a saved cohort)
func (h *Handlers) GetRetention(w http.ResponseWriter, r *http.Request) {
	domainID := r.URL.Query().Get("domain_id")
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if domainID == "" || start == "" || end == "" {
		writeError(w, http.StatusBadRequest, "domain_id, start, and end are required")
		return
	}
	startT, err := time.Parse(time.RFC3339, start)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start")
		return
	}
	endT, err := time.Parse(time.RFC3339, end)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end")
		return
	}

	days := 14
	if d := r.URL.Query().Get("days"); d != "" {
		// Parse into a temp so a negative or garbage value can't overwrite
		// the default (days sizes slices below; negative would panic).
		var parsed int
		if v, _ := fmt.Sscanf(d, "%d", &parsed); v == 1 && parsed > 0 {
			days = parsed
			if days > 30 {
				days = 30
			}
		}
	}

	domainName, err := h.domainNameByID(r.Context(), domainID)
	if err != nil {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}

	cohortFilter := ""
	var cohortArgs []interface{}
	if cohortID := r.URL.Query().Get("cohort_id"); cohortID != "" {
		def, err := loadCohortDefinition(r.Context(), h.db.Conn(), cohortID, domainID)
		if err != nil {
			writeError(w, http.StatusNotFound, "cohort not found")
			return
		}
		cohortFilter, cohortArgs = def.buildWhere()
	}

	// Build the first_seen set: visitors whose earliest event within the range
	// matches the cohort (if any), bucketed by date.
	firstSeenWhere := "WHERE domain = ? AND is_bot = 0 AND timestamp BETWEEN ? AND ?"
	firstSeenArgs := []interface{}{domainName, startT.UnixMilli(), endT.UnixMilli()}
	if cohortFilter != "" {
		firstSeenWhere += " AND " + cohortFilter
		firstSeenArgs = append(firstSeenArgs, cohortArgs...)
	}

	// Build the full query as a single CTE chain.
	// first_seen: (visitor_hash, first_day)
	// returns: visitor_hash x day_offset
	query := fmt.Sprintf(`
		WITH first_seen AS (
			SELECT visitor_hash, CAST(MIN(timestamp) / 86400000 AS INTEGER) AS first_day
			FROM events
			%s
			GROUP BY visitor_hash
		),
		visits AS (
			SELECT DISTINCT e.visitor_hash, CAST(e.timestamp / 86400000 AS INTEGER) AS day
			FROM events e
			INNER JOIN first_seen f ON e.visitor_hash = f.visitor_hash
			WHERE e.domain = ? AND e.is_bot = 0 AND e.timestamp <= ?
		)
		SELECT fs.first_day, v.day - fs.first_day AS offset, COUNT(DISTINCT v.visitor_hash)
		FROM first_seen fs
		INNER JOIN visits v ON v.visitor_hash = fs.visitor_hash
		WHERE v.day - fs.first_day BETWEEN 0 AND ?
		GROUP BY fs.first_day, offset
		ORDER BY fs.first_day, offset
	`, firstSeenWhere)

	queryArgs := append([]interface{}{}, firstSeenArgs...)
	queryArgs = append(queryArgs, domainName, endT.UnixMilli(), days)

	rows, err := h.db.Conn().QueryContext(r.Context(), query, queryArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to compute retention: "+err.Error())
		return
	}
	defer rows.Close()

	// Collect results keyed by cohort day.
	type cohortBucket struct {
		returns []int
	}
	buckets := map[int]*cohortBucket{}
	for rows.Next() {
		var firstDay, offset, count int
		if err := rows.Scan(&firstDay, &offset, &count); err != nil {
			continue
		}
		b, ok := buckets[firstDay]
		if !ok {
			b = &cohortBucket{returns: make([]int, days+1)}
			buckets[firstDay] = b
		}
		if offset >= 0 && offset <= days {
			b.returns[offset] = count
		}
	}

	// Produce every day in [startT, endT], even if empty.
	epochDayStart := int(startT.UnixMilli() / 86400000)
	epochDayEnd := int(endT.UnixMilli() / 86400000)

	cohorts := []RetentionCohort{}
	for day := epochDayStart; day <= epochDayEnd; day++ {
		b, ok := buckets[day]
		size := 0
		returns := make([]int, days+1)
		if ok {
			size = b.returns[0]
			copy(returns, b.returns)
		}
		rates := make([]float64, days+1)
		if size > 0 {
			for i := range rates {
				rates[i] = float64(returns[i]) / float64(size) * 100
			}
		}
		cohorts = append(cohorts, RetentionCohort{
			Date:    time.UnixMilli(int64(day) * 86400000).UTC().Format("2006-01-02"),
			Size:    size,
			Returns: returns,
			Rates:   rates,
		})
	}

	writeJSON(w, http.StatusOK, RetentionResponse{
		Days:    days,
		Cohorts: cohorts,
	})
}

// buildWhere renders a cohort definition into an SQL WHERE fragment + args,
// referencing the `events` table columns directly.
func (d CohortDefinition) buildWhere() (string, []interface{}) {
	var clauses []string
	var args []interface{}
	if d.UTMSource != "" {
		clauses = append(clauses, "utm_source = ?")
		args = append(args, d.UTMSource)
	}
	if d.UTMMedium != "" {
		clauses = append(clauses, "utm_medium = ?")
		args = append(args, d.UTMMedium)
	}
	if d.UTMCampaign != "" {
		clauses = append(clauses, "utm_campaign = ?")
		args = append(args, d.UTMCampaign)
	}
	if d.ReferrerContains != "" {
		clauses = append(clauses, "referrer_url LIKE ?")
		args = append(args, "%"+d.ReferrerContains+"%")
	}
	if d.PathContains != "" {
		clauses = append(clauses, "path LIKE ?")
		args = append(args, "%"+d.PathContains+"%")
	}
	if d.Country != "" {
		clauses = append(clauses, "geo_country = ?")
		args = append(args, d.Country)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return strings.Join(clauses, " AND "), args
}

func loadCohortDefinition(ctx context.Context, db *sql.DB, id, domainID string) (CohortDefinition, error) {
	var defJSON string
	err := db.QueryRowContext(ctx,
		"SELECT definition FROM cohorts WHERE id = ? AND domain_id = ?", id, domainID).Scan(&defJSON)
	if err != nil {
		return CohortDefinition{}, err
	}
	var def CohortDefinition
	_ = json.Unmarshal([]byte(defJSON), &def)
	return def, nil
}
