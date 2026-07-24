package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/go-chi/chi/v5"
)

// Goal represents a single conversion target on a domain.
type Goal struct {
	ID         string `json:"id"`
	DomainID   string `json:"domain_id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"` // "pageview" | "custom" | "outbound"
	MatchValue string `json:"match_value"`
	ValueCents int    `json:"value_cents"`
	Currency   string `json:"currency"`
	CreatedBy  string `json:"created_by"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

// GoalStat reports completions and revenue for a goal over a time range.
type GoalStat struct {
	GoalID       string  `json:"goal_id"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	Completions  int     `json:"completions"`
	UniqueVisits int     `json:"unique_visitors"`
	Conversion   float64 `json:"conversion_rate"`
	RevenueCents int     `json:"revenue_cents"`
	Currency     string  `json:"currency"`
}

func (h *Handlers) ListGoals(w http.ResponseWriter, r *http.Request) {
	domainID := r.URL.Query().Get("domain_id")
	if domainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}
	rows, err := h.db.Conn().QueryContext(r.Context(), `
		SELECT id, domain_id, name, kind, match_value, value_cents, currency,
		       created_by, created_at, updated_at
		FROM goals WHERE domain_id = ? ORDER BY created_at DESC
	`, domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query goals")
		return
	}
	defer rows.Close()

	out := []Goal{}
	for rows.Next() {
		var g Goal
		if err := rows.Scan(&g.ID, &g.DomainID, &g.Name, &g.Kind, &g.MatchValue,
			&g.ValueCents, &g.Currency, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt); err != nil {
			continue
		}
		out = append(out, g)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) CreateGoal(w http.ResponseWriter, r *http.Request) {
	var req Goal
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DomainID == "" || req.Name == "" || req.MatchValue == "" {
		writeError(w, http.StatusBadRequest, "domain_id, name, and match_value are required")
		return
	}
	if req.Kind == "" {
		req.Kind = "pageview"
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}

	claims := auth.GetUserFromContext(r.Context())
	createdBy := ""
	if claims != nil {
		createdBy = claims.UserID
	}
	id := generateID()
	now := time.Now().UnixMilli()

	_, err := h.db.Conn().ExecContext(r.Context(), `
		INSERT INTO goals (id, domain_id, name, kind, match_value, value_cents, currency,
			created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.DomainID, req.Name, req.Kind, req.MatchValue, req.ValueCents, req.Currency,
		createdBy, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create goal")
		return
	}

	h.logAudit(r, "create", "goal", id, req.Name)

	req.ID = id
	req.CreatedBy = createdBy
	req.CreatedAt = now
	req.UpdatedAt = now
	writeJSON(w, http.StatusCreated, req)
}

func (h *Handlers) UpdateGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req Goal
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.MatchValue == "" {
		writeError(w, http.StatusBadRequest, "name and match_value are required")
		return
	}
	now := time.Now().UnixMilli()
	res, err := h.db.Conn().ExecContext(r.Context(), `
		UPDATE goals SET name = ?, kind = ?, match_value = ?, value_cents = ?,
		                 currency = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Kind, req.MatchValue, req.ValueCents, req.Currency, now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update goal")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	h.logAudit(r, "update", "goal", id, req.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) DeleteGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := h.db.Conn().ExecContext(r.Context(), "DELETE FROM goals WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete goal")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	h.logAudit(r, "delete", "goal", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetGoalStats computes completions, unique visitors, and revenue for each
// goal over the requested time range.
func (h *Handlers) GetGoalStats(w http.ResponseWriter, r *http.Request) {
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
	startMs := startT.UnixMilli()
	endMs := endT.UnixMilli()

	domainName, err := h.domainNameByID(r.Context(), domainID)
	if err != nil {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}

	goalRows, err := h.db.Conn().QueryContext(r.Context(), `
		SELECT id, name, kind, match_value, value_cents, currency
		FROM goals WHERE domain_id = ?
	`, domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query goals")
		return
	}
	defer goalRows.Close()

	type goalRow struct {
		id, name, kind, match, currency string
		valueCents                      int
	}
	var goals []goalRow
	for goalRows.Next() {
		var g goalRow
		if err := goalRows.Scan(&g.id, &g.name, &g.kind, &g.match, &g.valueCents, &g.currency); err != nil {
			continue
		}
		goals = append(goals, g)
	}

	// Total unique visitors over the range (for conversion rate).
	var totalVisitors int
	_ = h.db.Conn().QueryRowContext(r.Context(), `
		SELECT COUNT(DISTINCT visitor_hash) FROM events
		WHERE domain = ? AND is_bot = 0 AND timestamp BETWEEN ? AND ?
	`, domainName, startMs, endMs).Scan(&totalVisitors)

	out := []GoalStat{}
	for _, g := range goals {
		var where string
		var args []interface{}

		switch g.kind {
		case "pageview":
			where = "event_type = 'pageview' AND path = ?"
			args = []interface{}{g.match}
		case "custom":
			where = "event_type = 'custom' AND event_name = ?"
			args = []interface{}{g.match}
		case "outbound":
			where = "event_type = 'custom' AND event_name = 'outbound' AND json_extract_string(props, '$.url') LIKE ?"
			args = []interface{}{"%" + g.match + "%"}
		default:
			continue
		}

		query := fmt.Sprintf(`
			SELECT COUNT(*), COUNT(DISTINCT visitor_hash)
			FROM events
			WHERE domain = ? AND is_bot = 0 AND timestamp BETWEEN ? AND ? AND %s
		`, where)
		fullArgs := append([]interface{}{domainName, startMs, endMs}, args...)

		var completions, uniqueVisits int
		_ = h.db.Conn().QueryRowContext(r.Context(), query, fullArgs...).Scan(&completions, &uniqueVisits)

		rate := 0.0
		if totalVisitors > 0 {
			rate = float64(uniqueVisits) / float64(totalVisitors) * 100
		}

		out = append(out, GoalStat{
			GoalID:       g.id,
			Name:         g.name,
			Kind:         g.kind,
			Completions:  completions,
			UniqueVisits: uniqueVisits,
			Conversion:   rate,
			RevenueCents: completions * g.valueCents,
			Currency:     g.currency,
		})
	}

	writeJSON(w, http.StatusOK, out)
}

// GetCustomEventNames returns distinct custom event names seen for a domain.
// Used by the funnel/goal UI for autocomplete. Limits by recent data.
func (h *Handlers) GetCustomEventNames(w http.ResponseWriter, r *http.Request) {
	domainID := r.URL.Query().Get("domain_id")
	if domainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}
	domainName, err := h.domainNameByID(r.Context(), domainID)
	if err != nil {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}
	// Look at the last 30 days.
	since := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()

	rows, err := h.db.Conn().QueryContext(r.Context(), `
		SELECT event_name, COUNT(*) AS c
		FROM events
		WHERE domain = ? AND event_type = 'custom' AND event_name IS NOT NULL
		  AND timestamp > ?
		GROUP BY event_name
		ORDER BY c DESC
		LIMIT 100
	`, domainName, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query event names")
		return
	}
	defer rows.Close()

	type nameCount struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	out := []nameCount{}
	for rows.Next() {
		var nc nameCount
		if err := rows.Scan(&nc.Name, &nc.Count); err != nil {
			continue
		}
		out = append(out, nc)
	}
	writeJSON(w, http.StatusOK, out)
}
