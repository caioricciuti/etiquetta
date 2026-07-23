package api

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/caioricciuti/etiquetta/internal/alerts"
	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/go-chi/chi/v5"
)

// ========== Alerts CRUD ==========

func (h *Handlers) ListAlerts(w http.ResponseWriter, r *http.Request) {
	domainID := r.URL.Query().Get("domain_id")
	if domainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}

	rows, err := h.db.Conn().QueryContext(r.Context(), `
		SELECT id, domain_id, name, metric, comparator, threshold,
		       window_minutes, cooldown_minutes, channels, recipients,
		       enabled, created_by, created_at, updated_at
		FROM alerts WHERE domain_id = ? ORDER BY created_at DESC
	`, domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query alerts")
		return
	}
	defer rows.Close()

	out := []alerts.Alert{}
	for rows.Next() {
		var a alerts.Alert
		var channels, recipients string
		var enabled int
		if err := rows.Scan(&a.ID, &a.DomainID, &a.Name, &a.Metric, &a.Comparator, &a.Threshold,
			&a.WindowMinutes, &a.CooldownMinutes, &channels, &recipients,
			&enabled, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(channels), &a.Channels)
		_ = json.Unmarshal([]byte(recipients), &a.Recipients)
		a.Enabled = enabled == 1
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) CreateAlert(w http.ResponseWriter, r *http.Request) {
	var req alerts.Alert
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DomainID == "" || req.Name == "" || req.Metric == "" {
		writeError(w, http.StatusBadRequest, "domain_id, name, and metric are required")
		return
	}
	if req.WindowMinutes <= 0 {
		req.WindowMinutes = 60
	}
	if req.CooldownMinutes <= 0 {
		req.CooldownMinutes = 60
	}
	if req.Comparator == "" {
		req.Comparator = alerts.CompGt
	}

	claims := auth.GetUserFromContext(r.Context())
	createdBy := ""
	if claims != nil {
		createdBy = claims.UserID
	}

	id := generateID()
	now := time.Now().UnixMilli()
	channels, _ := json.Marshal(req.Channels)
	recipients, _ := json.Marshal(req.Recipients)
	enabled := 0
	if req.Enabled {
		enabled = 1
	}

	_, err := h.db.Conn().ExecContext(r.Context(), `
		INSERT INTO alerts (id, domain_id, name, metric, comparator, threshold,
			window_minutes, cooldown_minutes, channels, recipients,
			enabled, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.DomainID, req.Name, req.Metric, req.Comparator, req.Threshold,
		req.WindowMinutes, req.CooldownMinutes, string(channels), string(recipients),
		enabled, createdBy, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create alert")
		return
	}

	h.logAudit(r, "create", "alert", id, req.Name)

	req.ID = id
	req.CreatedBy = createdBy
	req.CreatedAt = now
	req.UpdatedAt = now
	writeJSON(w, http.StatusCreated, req)
}

func (h *Handlers) UpdateAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req alerts.Alert
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Metric == "" {
		writeError(w, http.StatusBadRequest, "name and metric are required")
		return
	}

	channels, _ := json.Marshal(req.Channels)
	recipients, _ := json.Marshal(req.Recipients)
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	now := time.Now().UnixMilli()

	res, err := h.db.Conn().ExecContext(r.Context(), `
		UPDATE alerts SET name = ?, metric = ?, comparator = ?, threshold = ?,
		                  window_minutes = ?, cooldown_minutes = ?, channels = ?,
		                  recipients = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Metric, req.Comparator, req.Threshold,
		req.WindowMinutes, req.CooldownMinutes, string(channels), string(recipients),
		enabled, now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update alert")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "alert not found")
		return
	}

	h.logAudit(r, "update", "alert", id, req.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) DeleteAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := h.db.Conn().ExecContext(r.Context(), "DELETE FROM alerts WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete alert")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "alert not found")
		return
	}
	h.logAudit(r, "delete", "alert", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) ListAlertFires(w http.ResponseWriter, r *http.Request) {
	domainID := r.URL.Query().Get("domain_id")
	if domainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}

	rows, err := h.db.Conn().QueryContext(r.Context(), `
		SELECT id, alert_id, domain_id, fired_at, metric_value, threshold, channels_sent, error
		FROM alert_fires WHERE domain_id = ? ORDER BY fired_at DESC LIMIT 200
	`, domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query fires")
		return
	}
	defer rows.Close()

	out := []alerts.Fire{}
	for rows.Next() {
		var f alerts.Fire
		var channels string
		var errCol sql.NullString
		if err := rows.Scan(&f.ID, &f.AlertID, &f.DomainID, &f.FiredAt,
			&f.MetricValue, &f.Threshold, &channels, &errCol); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(channels), &f.ChannelsSent)
		if errCol.Valid {
			f.Error = errCol.String
		}
		out = append(out, f)
	}
	writeJSON(w, http.StatusOK, out)
}

// ========== Webhooks CRUD ==========

func (h *Handlers) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	domainID := r.URL.Query().Get("domain_id")
	if domainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}
	rows, err := h.db.Conn().QueryContext(r.Context(), `
		SELECT id, domain_id, name, url, secret, events, enabled, created_by,
		       created_at, updated_at, last_fired_at, last_status, last_error
		FROM webhooks WHERE domain_id = ? ORDER BY created_at DESC
	`, domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query webhooks")
		return
	}
	defer rows.Close()

	out := []alerts.Webhook{}
	for rows.Next() {
		var wh alerts.Webhook
		var events string
		var enabled int
		var lastFired sql.NullInt64
		var lastStatus sql.NullInt64
		var lastError sql.NullString
		if err := rows.Scan(&wh.ID, &wh.DomainID, &wh.Name, &wh.URL, &wh.Secret, &events, &enabled,
			&wh.CreatedBy, &wh.CreatedAt, &wh.UpdatedAt, &lastFired, &lastStatus, &lastError); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(events), &wh.Events)
		wh.Enabled = enabled == 1
		if lastFired.Valid {
			v := lastFired.Int64
			wh.LastFiredAt = &v
		}
		if lastStatus.Valid {
			v := int(lastStatus.Int64)
			wh.LastStatus = &v
		}
		if lastError.Valid {
			wh.LastError = lastError.String
		}
		// Mask secret in list view; full secret is returned on creation only.
		if len(wh.Secret) > 8 {
			wh.Secret = wh.Secret[:4] + "…" + wh.Secret[len(wh.Secret)-4:]
		}
		out = append(out, wh)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var req alerts.Webhook
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Webhook.Enabled is a plain bool, which can't distinguish "omitted"
	// (default enabled) from an explicit false — decode that one field again
	// as a pointer.
	var enabledField struct {
		Enabled *bool `json:"enabled"`
	}
	_ = json.Unmarshal(body, &enabledField)
	if req.DomainID == "" || req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "domain_id, name, and url are required")
		return
	}

	claims := auth.GetUserFromContext(r.Context())
	createdBy := ""
	if claims != nil {
		createdBy = claims.UserID
	}

	id := generateID()
	secret := alerts.GenerateSecret()
	events, _ := json.Marshal(req.Events)
	now := time.Now().UnixMilli()
	// Default enabled unless explicitly false.
	enabled := 1
	if enabledField.Enabled != nil && !*enabledField.Enabled {
		enabled = 0
	}

	_, err = h.db.Conn().ExecContext(r.Context(), `
		INSERT INTO webhooks (id, domain_id, name, url, secret, events, enabled,
			created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.DomainID, req.Name, req.URL, secret, string(events), enabled,
		createdBy, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create webhook")
		return
	}

	h.logAudit(r, "create", "webhook", id, req.Name)

	req.ID = id
	req.Secret = secret
	req.Enabled = enabled == 1
	req.CreatedBy = createdBy
	req.CreatedAt = now
	req.UpdatedAt = now
	writeJSON(w, http.StatusCreated, req)
}

func (h *Handlers) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req alerts.Webhook
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}

	events, _ := json.Marshal(req.Events)
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	now := time.Now().UnixMilli()

	res, err := h.db.Conn().ExecContext(r.Context(), `
		UPDATE webhooks SET name = ?, url = ?, events = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.URL, string(events), enabled, now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update webhook")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	h.logAudit(r, "update", "webhook", id, req.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := h.db.Conn().ExecContext(r.Context(), "DELETE FROM webhooks WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete webhook")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	h.logAudit(r, "delete", "webhook", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) TestWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	wh, err := alerts.LoadWebhook(r.Context(), h.db.Conn(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	if err := alerts.TestFire(r.Context(), wh.URL, wh.Secret); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Test payload delivered",
	})
}
