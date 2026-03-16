package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// containerCache stores generated container JS keyed by site_id
var containerCache sync.Map // map[string][]byte

// ServeContainerScript serves the published container JS for a site
func (h *Handlers) ServeContainerScript(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteId")
	if siteID == "" {
		log.Printf("[tm] ServeContainerScript: missing siteId")
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte("/* etiquetta: container not available */"))
		return
	}

	// Check cache first
	if cached, ok := containerCache.Load(siteID); ok {
		log.Printf("[tm] ServeContainerScript: serving cached JS for siteId=%s", siteID)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(cached.([]byte))
		return
	}

	// Look up domain by site_id
	var domainID string
	err := h.db.Conn().QueryRow("SELECT id FROM domains WHERE site_id = ? AND is_active = 1", siteID).Scan(&domainID)
	if err != nil {
		log.Printf("[tm] ServeContainerScript: domain not found for siteId=%s: %v", siteID, err)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte("/* etiquetta: container not available */"))
		return
	}

	// Find container for domain
	var containerID string
	var publishedVersion int
	err = h.db.Conn().QueryRow(`
		SELECT id, published_version FROM tm_containers WHERE domain_id = ? AND published_version > 0
	`, domainID).Scan(&containerID, &publishedVersion)
	if err != nil {
		log.Printf("[tm] ServeContainerScript: no published container for domainId=%s: %v", domainID, err)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte("/* etiquetta: container not available */"))
		return
	}

	// Get published snapshot
	var snapshotJSON string
	err = h.db.Conn().QueryRow(`
		SELECT snapshot FROM tm_snapshots WHERE container_id = ? AND version = ?
	`, containerID, publishedVersion).Scan(&snapshotJSON)
	if err != nil {
		log.Printf("[tm] ServeContainerScript: snapshot not found for container=%s version=%d: %v", containerID, publishedVersion, err)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte("/* etiquetta: container not available */"))
		return
	}

	js := generateContainerJS(snapshotJSON)
	jsBytes := []byte(js)

	// Cache it
	containerCache.Store(siteID, jsBytes)

	log.Printf("[tm] ServeContainerScript: generated and cached JS for siteId=%s (%d bytes)", siteID, len(jsBytes))
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(jsBytes)
}

// ListContainers returns all tag manager containers with domain info
func (h *Handlers) ListContainers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Conn().Query(`
		SELECT c.id, c.domain_id, c.name, c.published_version, c.draft_version,
		       c.published_at, c.published_by, c.created_at, c.updated_at,
		       d.name as domain_name, d.domain, d.site_id
		FROM tm_containers c
		JOIN domains d ON d.id = c.domain_id
		ORDER BY c.created_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var containers []map[string]interface{}
	for rows.Next() {
		var (
			id, domainID, name                string
			publishedVersion, draftVersion    int
			publishedAt                       *int64
			publishedBy                       *string
			createdAt, updatedAt              int64
			domainName, domain, siteID        string
		)
		if err := rows.Scan(&id, &domainID, &name, &publishedVersion, &draftVersion,
			&publishedAt, &publishedBy, &createdAt, &updatedAt,
			&domainName, &domain, &siteID); err != nil {
			continue
		}
		containers = append(containers, map[string]interface{}{
			"id":                id,
			"domain_id":         domainID,
			"name":              name,
			"published_version": publishedVersion,
			"draft_version":     draftVersion,
			"published_at":      publishedAt,
			"published_by":      publishedBy,
			"created_at":        createdAt,
			"updated_at":        updatedAt,
			"domain_name":       domainName,
			"domain":            domain,
			"site_id":           siteID,
		})
	}

	if containers == nil {
		containers = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, containers)
}

// CreateContainer creates a new tag manager container
func (h *Handlers) CreateContainer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DomainID string `json:"domain_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.DomainID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "domain_id and name are required")
		return
	}

	now := time.Now().UnixMilli()
	id := generateID()

	_, err := h.db.Conn().Exec(`
		INSERT INTO tm_containers (id, domain_id, name, published_version, draft_version, created_at, updated_at)
		VALUES (?, ?, ?, 0, 1, ?, ?)
	`, id, req.DomainID, req.Name, now, now)
	if err != nil {
		writeError(w, http.StatusConflict, "Container already exists for this domain")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                id,
		"domain_id":         req.DomainID,
		"name":              req.Name,
		"published_version": 0,
		"draft_version":     1,
		"created_at":        now,
		"updated_at":        now,
	})
}

// GetContainer returns a container by ID
func (h *Handlers) GetContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var (
		domainID, name                string
		publishedVersion, draftVersion int
		publishedAt                   *int64
		publishedBy                   *string
		createdAt, updatedAt          int64
	)

	err := h.db.Conn().QueryRow(`
		SELECT id, domain_id, name, published_version, draft_version,
		       published_at, published_by, created_at, updated_at
		FROM tm_containers WHERE id = ?
	`, id).Scan(&id, &domainID, &name, &publishedVersion, &draftVersion,
		&publishedAt, &publishedBy, &createdAt, &updatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                id,
		"domain_id":         domainID,
		"name":              name,
		"published_version": publishedVersion,
		"draft_version":     draftVersion,
		"published_at":      publishedAt,
		"published_by":      publishedBy,
		"created_at":        createdAt,
		"updated_at":        updatedAt,
	})
}

// UpdateContainer updates a container's name
func (h *Handlers) UpdateContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	now := time.Now().UnixMilli()
	result, err := h.db.Conn().Exec("UPDATE tm_containers SET name = ?, updated_at = ? WHERE id = ?", req.Name, now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Update failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteContainer deletes a container and all associated data
func (h *Handlers) DeleteContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.db.Conn().Exec("DELETE FROM tm_containers WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Delete failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PublishContainer publishes the current container state as a new version
func (h *Handlers) PublishContainer(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")
	log.Printf("[tm] PublishContainer: containerID=%s", containerID)

	// Get container info
	var domainID string
	var currentPublished int
	err := h.db.Conn().QueryRow("SELECT domain_id, published_version FROM tm_containers WHERE id = ?", containerID).Scan(&domainID, &currentPublished)
	if err != nil {
		log.Printf("[tm] PublishContainer: container not found: %v", err)
		writeError(w, http.StatusNotFound, "Container not found")
		return
	}

	// Build snapshot: all tags, triggers, variables, and associations
	snapshot, err := buildContainerSnapshot(h, containerID)
	if err != nil {
		log.Printf("[tm] PublishContainer: failed to build snapshot: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to build snapshot")
		return
	}

	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("[tm] PublishContainer: failed to serialize snapshot: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to serialize snapshot")
		return
	}

	newVersion := currentPublished + 1
	now := time.Now().UnixMilli()
	snapshotID := generateID()

	// Get publisher info from context if available
	publishedBy := ""

	tx, err := h.db.Conn().Begin()
	if err != nil {
		log.Printf("[tm] PublishContainer: begin tx failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	// Insert snapshot
	_, err = tx.Exec(`
		INSERT INTO tm_snapshots (id, container_id, version, snapshot, published_by, published_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, snapshotID, containerID, newVersion, string(snapshotJSON), publishedBy, now)
	if err != nil {
		log.Printf("[tm] PublishContainer: insert snapshot failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to save snapshot")
		return
	}

	// Update container
	_, err = tx.Exec(`
		UPDATE tm_containers SET published_version = ?, published_at = ?, published_by = ?, updated_at = ?
		WHERE id = ?
	`, newVersion, now, publishedBy, now, containerID)
	if err != nil {
		log.Printf("[tm] PublishContainer: update container failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to update container")
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[tm] PublishContainer: commit failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Commit failed")
		return
	}

	// Clear cache for this domain's site_id
	var siteID string
	if err := h.db.Conn().QueryRow("SELECT site_id FROM domains WHERE id = ?", domainID).Scan(&siteID); err == nil {
		containerCache.Delete(siteID)
	}

	log.Printf("[tm] PublishContainer: published version %d for container %s", newVersion, containerID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":      newVersion,
		"published_at": now,
	})
}

// GetContainerVersions lists all published snapshots for a container
func (h *Handlers) GetContainerVersions(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")

	rows, err := h.db.Conn().Query(`
		SELECT id, container_id, version, published_by, published_at
		FROM tm_snapshots
		WHERE container_id = ?
		ORDER BY version DESC
	`, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var versions []map[string]interface{}
	for rows.Next() {
		var (
			id, cID   string
			version   int
			pubBy     *string
			pubAt     int64
		)
		if err := rows.Scan(&id, &cID, &version, &pubBy, &pubAt); err != nil {
			continue
		}
		versions = append(versions, map[string]interface{}{
			"id":           id,
			"container_id": cID,
			"version":      version,
			"published_by": pubBy,
			"published_at": pubAt,
		})
	}

	if versions == nil {
		versions = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, versions)
}

// RollbackContainer sets a previous snapshot as the new published version
func (h *Handlers) RollbackContainer(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")
	versionStr := chi.URLParam(r, "version")
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid version")
		return
	}

	// Verify the snapshot exists
	var snapshotJSON string
	err = h.db.Conn().QueryRow("SELECT snapshot FROM tm_snapshots WHERE container_id = ? AND version = ?", containerID, version).Scan(&snapshotJSON)
	if err != nil {
		writeError(w, http.StatusNotFound, "Snapshot not found")
		return
	}

	// Get domain_id for cache clearing
	var domainID string
	h.db.Conn().QueryRow("SELECT domain_id FROM tm_containers WHERE id = ?", containerID).Scan(&domainID)

	now := time.Now().UnixMilli()
	_, err = h.db.Conn().Exec(`
		UPDATE tm_containers SET published_version = ?, published_at = ?, updated_at = ? WHERE id = ?
	`, version, now, now, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Rollback failed")
		return
	}

	// Clear cache
	var siteID string
	if err := h.db.Conn().QueryRow("SELECT site_id FROM domains WHERE id = ?", domainID).Scan(&siteID); err == nil {
		containerCache.Delete(siteID)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":      version,
		"published_at": now,
	})
}

// ========== Tag CRUD ==========

// ListTags lists all tags for a container, including trigger associations
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	rows, err := h.db.Conn().Query(`
		SELECT id, container_id, name, tag_type, config, consent_category, priority, is_enabled, version, created_at, updated_at
		FROM tm_tags WHERE container_id = ?
		ORDER BY priority DESC, name ASC
	`, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	type tagRow struct {
		id, cID, name, tagType, config, consentCat string
		priority, version                          int
		isEnabled                                  bool
		createdAt, updatedAt                       int64
	}
	var tagRows []tagRow
	for rows.Next() {
		var t tagRow
		if err := rows.Scan(&t.id, &t.cID, &t.name, &t.tagType, &t.config, &t.consentCat, &t.priority, &t.isEnabled, &t.version, &t.createdAt, &t.updatedAt); err != nil {
			continue
		}
		tagRows = append(tagRows, t)
	}
	rows.Close()

	// Batch-fetch all trigger associations for this container
	triggerMap := getContainerTagTriggerIDs(h, containerID)

	tags := make([]map[string]interface{}, 0, len(tagRows))
	for _, t := range tagRows {
		triggerIDs := triggerMap[t.id]
		if triggerIDs == nil {
			triggerIDs = []string{}
		}
		tags = append(tags, map[string]interface{}{
			"id":               t.id,
			"container_id":     t.cID,
			"name":             t.name,
			"tag_type":         t.tagType,
			"config":           json.RawMessage(t.config),
			"consent_category": t.consentCat,
			"priority":         t.priority,
			"is_enabled":       t.isEnabled,
			"version":          t.version,
			"trigger_ids":      triggerIDs,
			"created_at":       t.createdAt,
			"updated_at":       t.updatedAt,
		})
	}

	writeJSON(w, http.StatusOK, tags)
}

// CreateTag creates a new tag in a container
func (h *Handlers) CreateTag(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	var req struct {
		Name            string          `json:"name"`
		TagType         string          `json:"tag_type"`
		Config          json.RawMessage `json:"config"`
		ConsentCategory string          `json:"consent_category"`
		Priority        int             `json:"priority"`
		TriggerIDs      []string        `json:"trigger_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Name == "" || req.TagType == "" {
		writeError(w, http.StatusBadRequest, "name and tag_type are required")
		return
	}
	if req.ConsentCategory == "" {
		req.ConsentCategory = "marketing"
	}
	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	now := time.Now().UnixMilli()
	id := generateID()

	tx, err := h.db.Conn().Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO tm_tags (id, container_id, name, tag_type, config, consent_category, priority, is_enabled, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, 1, ?, ?)
	`, id, containerID, req.Name, req.TagType, string(req.Config), req.ConsentCategory, req.Priority, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create tag")
		return
	}

	// Insert trigger associations
	for _, triggerID := range req.TriggerIDs {
		_, err = tx.Exec("INSERT INTO tm_tag_triggers (tag_id, trigger_id, is_exception) VALUES (?, ?, 0)", id, triggerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to associate triggers")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":               id,
		"container_id":     containerID,
		"name":             req.Name,
		"tag_type":         req.TagType,
		"config":           req.Config,
		"consent_category": req.ConsentCategory,
		"priority":         req.Priority,
		"is_enabled":       true,
		"version":          1,
		"trigger_ids":      req.TriggerIDs,
		"created_at":       now,
		"updated_at":       now,
	})
}

// GetTag returns a single tag by ID
func (h *Handlers) GetTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "id")

	var (
		id, cID, name, tagType, config, consentCat string
		priority, version                          int
		isEnabled                                  bool
		createdAt, updatedAt                       int64
	)
	err := h.db.Conn().QueryRow(`
		SELECT id, container_id, name, tag_type, config, consent_category, priority, is_enabled, version, created_at, updated_at
		FROM tm_tags WHERE id = ?
	`, tagID).Scan(&id, &cID, &name, &tagType, &config, &consentCat, &priority, &isEnabled, &version, &createdAt, &updatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "Tag not found")
		return
	}

	triggerIDs := getTagTriggerIDs(h, id)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":               id,
		"container_id":     cID,
		"name":             name,
		"tag_type":         tagType,
		"config":           json.RawMessage(config),
		"consent_category": consentCat,
		"priority":         priority,
		"is_enabled":       isEnabled,
		"version":          version,
		"trigger_ids":      triggerIDs,
		"created_at":       createdAt,
		"updated_at":       updatedAt,
	})
}

// UpdateTag updates a tag and its trigger associations
func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "id")

	var req struct {
		Name            string          `json:"name"`
		TagType         string          `json:"tag_type"`
		Config          json.RawMessage `json:"config"`
		ConsentCategory string          `json:"consent_category"`
		Priority        int             `json:"priority"`
		IsEnabled       *bool           `json:"is_enabled"`
		TriggerIDs      []string        `json:"trigger_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	now := time.Now().UnixMilli()

	tx, err := h.db.Conn().Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	isEnabled := 1
	if req.IsEnabled != nil && !*req.IsEnabled {
		isEnabled = 0
	}

	configStr := "{}"
	if req.Config != nil {
		configStr = string(req.Config)
	}

	result, err := tx.Exec(`
		UPDATE tm_tags SET name = ?, tag_type = ?, config = ?, consent_category = ?, priority = ?, is_enabled = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.TagType, configStr, req.ConsentCategory, req.Priority, isEnabled, now, tagID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Update failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Tag not found")
		return
	}

	// Replace trigger associations
	tx.Exec("DELETE FROM tm_tag_triggers WHERE tag_id = ?", tagID)
	for _, triggerID := range req.TriggerIDs {
		tx.Exec("INSERT INTO tm_tag_triggers (tag_id, trigger_id, is_exception) VALUES (?, ?, 0)", tagID, triggerID)
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Commit failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteTag deletes a tag
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "id")

	result, err := h.db.Conn().Exec("DELETE FROM tm_tags WHERE id = ?", tagID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Delete failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Tag not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ========== Trigger CRUD ==========

// ListTriggers lists all triggers for a container
func (h *Handlers) ListTriggers(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	rows, err := h.db.Conn().Query(`
		SELECT id, container_id, name, trigger_type, config, created_at, updated_at
		FROM tm_triggers WHERE container_id = ?
		ORDER BY name ASC
	`, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var triggers []map[string]interface{}
	for rows.Next() {
		var (
			id, cID, name, triggerType, config string
			createdAt, updatedAt               int64
		)
		if err := rows.Scan(&id, &cID, &name, &triggerType, &config, &createdAt, &updatedAt); err != nil {
			continue
		}
		triggers = append(triggers, map[string]interface{}{
			"id":           id,
			"container_id": cID,
			"name":         name,
			"trigger_type": triggerType,
			"config":       json.RawMessage(config),
			"created_at":   createdAt,
			"updated_at":   updatedAt,
		})
	}

	if triggers == nil {
		triggers = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, triggers)
}

// CreateTrigger creates a new trigger in a container
func (h *Handlers) CreateTrigger(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	var req struct {
		Name        string          `json:"name"`
		TriggerType string          `json:"trigger_type"`
		Config      json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Name == "" || req.TriggerType == "" {
		writeError(w, http.StatusBadRequest, "name and trigger_type are required")
		return
	}
	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	now := time.Now().UnixMilli()
	id := generateID()

	_, err := h.db.Conn().Exec(`
		INSERT INTO tm_triggers (id, container_id, name, trigger_type, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, containerID, req.Name, req.TriggerType, string(req.Config), now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create trigger")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":           id,
		"container_id": containerID,
		"name":         req.Name,
		"trigger_type": req.TriggerType,
		"config":       req.Config,
		"created_at":   now,
		"updated_at":   now,
	})
}

// UpdateTrigger updates a trigger
func (h *Handlers) UpdateTrigger(w http.ResponseWriter, r *http.Request) {
	triggerID := chi.URLParam(r, "id")

	var req struct {
		Name        string          `json:"name"`
		TriggerType string          `json:"trigger_type"`
		Config      json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	configStr := "{}"
	if req.Config != nil {
		configStr = string(req.Config)
	}

	now := time.Now().UnixMilli()
	result, err := h.db.Conn().Exec(`
		UPDATE tm_triggers SET name = ?, trigger_type = ?, config = ?, updated_at = ? WHERE id = ?
	`, req.Name, req.TriggerType, configStr, now, triggerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Update failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Trigger not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteTrigger deletes a trigger
func (h *Handlers) DeleteTrigger(w http.ResponseWriter, r *http.Request) {
	triggerID := chi.URLParam(r, "id")

	result, err := h.db.Conn().Exec("DELETE FROM tm_triggers WHERE id = ?", triggerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Delete failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Trigger not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ========== Variable CRUD ==========

// ListVariables lists all variables for a container
func (h *Handlers) ListVariables(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	rows, err := h.db.Conn().Query(`
		SELECT id, container_id, name, variable_type, config, created_at, updated_at
		FROM tm_variables WHERE container_id = ?
		ORDER BY name ASC
	`, containerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var variables []map[string]interface{}
	for rows.Next() {
		var (
			id, cID, name, varType, config string
			createdAt, updatedAt           int64
		)
		if err := rows.Scan(&id, &cID, &name, &varType, &config, &createdAt, &updatedAt); err != nil {
			continue
		}
		variables = append(variables, map[string]interface{}{
			"id":            id,
			"container_id":  cID,
			"name":          name,
			"variable_type": varType,
			"config":        json.RawMessage(config),
			"created_at":    createdAt,
			"updated_at":    updatedAt,
		})
	}

	if variables == nil {
		variables = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, variables)
}

// CreateVariable creates a new variable in a container
func (h *Handlers) CreateVariable(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "cid")

	var req struct {
		Name         string          `json:"name"`
		VariableType string          `json:"variable_type"`
		Config       json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Name == "" || req.VariableType == "" {
		writeError(w, http.StatusBadRequest, "name and variable_type are required")
		return
	}
	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	now := time.Now().UnixMilli()
	id := generateID()

	_, err := h.db.Conn().Exec(`
		INSERT INTO tm_variables (id, container_id, name, variable_type, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, containerID, req.Name, req.VariableType, string(req.Config), now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create variable")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            id,
		"container_id":  containerID,
		"name":          req.Name,
		"variable_type": req.VariableType,
		"config":        req.Config,
		"created_at":    now,
		"updated_at":    now,
	})
}

// UpdateVariable updates a variable
func (h *Handlers) UpdateVariable(w http.ResponseWriter, r *http.Request) {
	varID := chi.URLParam(r, "id")

	var req struct {
		Name         string          `json:"name"`
		VariableType string          `json:"variable_type"`
		Config       json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	configStr := "{}"
	if req.Config != nil {
		configStr = string(req.Config)
	}

	now := time.Now().UnixMilli()
	result, err := h.db.Conn().Exec(`
		UPDATE tm_variables SET name = ?, variable_type = ?, config = ?, updated_at = ? WHERE id = ?
	`, req.Name, req.VariableType, configStr, now, varID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Update failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Variable not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteVariable deletes a variable
func (h *Handlers) DeleteVariable(w http.ResponseWriter, r *http.Request) {
	varID := chi.URLParam(r, "id")

	result, err := h.db.Conn().Exec("DELETE FROM tm_variables WHERE id = ?", varID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Delete failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "Variable not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ========== Helpers ==========

// getContainerTagTriggerIDs batch-fetches all tag→trigger associations for a container.
// Returns a map of tagID → []triggerID. Safe to call without an open rows cursor.
func getContainerTagTriggerIDs(h *Handlers, containerID string) map[string][]string {
	rows, err := h.db.Conn().Query(`
		SELECT tt.tag_id, tt.trigger_id
		FROM tm_tag_triggers tt
		JOIN tm_tags t ON t.id = tt.tag_id
		WHERE t.container_id = ?
	`, containerID)
	if err != nil {
		return map[string][]string{}
	}
	defer rows.Close()

	m := make(map[string][]string)
	for rows.Next() {
		var tagID, triggerID string
		if rows.Scan(&tagID, &triggerID) == nil {
			m[tagID] = append(m[tagID], triggerID)
		}
	}
	return m
}

// getTagTriggerIDs returns trigger IDs associated with a single tag.
// Only safe when no rows cursor is open (e.g. after QueryRow).
func getTagTriggerIDs(h *Handlers, tagID string) []string {
	rows, err := h.db.Conn().Query("SELECT trigger_id FROM tm_tag_triggers WHERE tag_id = ?", tagID)
	if err != nil {
		return []string{}
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	if ids == nil {
		ids = []string{}
	}
	return ids
}

// buildContainerSnapshot builds a full snapshot of the container state
func buildContainerSnapshot(h *Handlers, containerID string) (map[string]interface{}, error) {
	// Tags
	tagRows, err := h.db.Conn().Query(`
		SELECT id, name, tag_type, config, consent_category, priority, is_enabled
		FROM tm_tags WHERE container_id = ? AND is_enabled = 1
	`, containerID)
	if err != nil {
		return nil, err
	}

	type snapTag struct {
		id, name, tagType, config, consentCat string
		priority                              int
	}
	var rawTags []snapTag
	for tagRows.Next() {
		var t snapTag
		var isEnabled bool
		if err := tagRows.Scan(&t.id, &t.name, &t.tagType, &t.config, &t.consentCat, &t.priority, &isEnabled); err != nil {
			continue
		}
		rawTags = append(rawTags, t)
	}
	tagRows.Close()

	// Batch-fetch trigger associations
	triggerMap := getContainerTagTriggerIDs(h, containerID)

	tags := make([]map[string]interface{}, 0, len(rawTags))
	for _, t := range rawTags {
		triggerIDs := triggerMap[t.id]
		if triggerIDs == nil {
			triggerIDs = []string{}
		}
		tags = append(tags, map[string]interface{}{
			"id":               t.id,
			"name":             t.name,
			"tag_type":         t.tagType,
			"config":           json.RawMessage(t.config),
			"consent_category": t.consentCat,
			"priority":         t.priority,
			"trigger_ids":      triggerIDs,
		})
	}

	// Triggers
	triggerRows, err := h.db.Conn().Query(`
		SELECT id, name, trigger_type, config FROM tm_triggers WHERE container_id = ?
	`, containerID)
	if err != nil {
		return nil, err
	}
	defer triggerRows.Close()

	var triggers []map[string]interface{}
	for triggerRows.Next() {
		var id, name, triggerType, config string
		if err := triggerRows.Scan(&id, &name, &triggerType, &config); err != nil {
			continue
		}
		triggers = append(triggers, map[string]interface{}{
			"id":           id,
			"name":         name,
			"trigger_type": triggerType,
			"config":       json.RawMessage(config),
		})
	}
	if triggers == nil {
		triggers = []map[string]interface{}{}
	}

	// Variables
	varRows, err := h.db.Conn().Query(`
		SELECT id, name, variable_type, config FROM tm_variables WHERE container_id = ?
	`, containerID)
	if err != nil {
		return nil, err
	}
	defer varRows.Close()

	var variables []map[string]interface{}
	for varRows.Next() {
		var id, name, varType, config string
		if err := varRows.Scan(&id, &name, &varType, &config); err != nil {
			continue
		}
		variables = append(variables, map[string]interface{}{
			"id":            id,
			"name":          name,
			"variable_type": varType,
			"config":        json.RawMessage(config),
		})
	}
	if variables == nil {
		variables = []map[string]interface{}{}
	}

	return map[string]interface{}{
		"tags":      tags,
		"triggers":  triggers,
		"variables": variables,
	}, nil
}

// generateContainerJS creates a self-executing JS string from a snapshot JSON
func generateContainerJS(snapshotJSON string) string {
	return fmt.Sprintf(`(function(){
"use strict";
var C=%s;
var _cl=[];
window.etiquettaDataLayer=window.etiquettaDataLayer||[];
var consent=window.__ETIQUETTA_CONSENT__||null;
function hasConsent(cat){return !consent||(consent[cat]===true);}
function resolveVar(v){
switch(v.variable_type){
case"data_layer":var dl=window.etiquettaDataLayer;var k=v.config.variable_name||"";for(var i=dl.length-1;i>=0;i--){if(dl[i]&&dl[i][k]!==undefined)return dl[i][k];}return v.config.default_value||"";
case"url_param":return new URL(location.href).searchParams.get(v.config.param_name||"")||"";
case"cookie":var cn=v.config.cookie_name||"";var m=document.cookie.match(new RegExp("(?:^|; )"+cn.replace(/[.*+?^${}()|[\]\\]/g,"\\$&")+"=([^;]*)"));return m?decodeURIComponent(m[1]):"";
case"dom_element":var el=document.querySelector(v.config.selector||"");return el?(v.config.attribute?el.getAttribute(v.config.attribute):el.textContent)||"":"";
case"js_variable":try{return(new Function("return "+v.config.variable_name))();}catch(e){return"";}
case"constant":return v.config.value||"";
case"referrer":return document.referrer;
case"page_url":return location.href;
case"page_path":return location.pathname;
case"page_hostname":return location.hostname;
default:return"";}
}
function loadScript(src,cb){var s=document.createElement("script");s.src=src;s.async=true;if(cb)s.onload=cb;document.head.appendChild(s);}
function fireTag(tag){
if(!hasConsent(tag.consent_category))return;
switch(tag.tag_type){
case"custom_html":var d=document.createElement("div");d.innerHTML=tag.config.html||"";var scripts=d.getElementsByTagName("script");for(var i=0;i<scripts.length;i++){var s=document.createElement("script");if(scripts[i].src){s.src=scripts[i].src;}else{s.textContent=scripts[i].textContent;}document.head.appendChild(s);}break;
case"ga4":if(!window.gtag){window.dataLayer=window.dataLayer||[];window.gtag=function(){window.dataLayer.push(arguments);};window.gtag("js",new Date());loadScript("https://www.googletagmanager.com/gtag/js?id="+tag.config.measurement_id);}window.gtag("config",tag.config.measurement_id);break;
case"meta_pixel":if(!window.fbq){var f=function(){f.callMethod?f.callMethod.apply(f,arguments):f.queue.push(arguments);};window.fbq=f;f.push=f;f.loaded=true;f.version="2.0";f.queue=[];loadScript("https://connect.facebook.net/en_US/fbevents.js");window.fbq("init",tag.config.pixel_id);}window.fbq("track","PageView");break;
case"google_ads":if(!window.gtag){window.dataLayer=window.dataLayer||[];window.gtag=function(){window.dataLayer.push(arguments);};window.gtag("js",new Date());loadScript("https://www.googletagmanager.com/gtag/js?id="+tag.config.conversion_id);}window.gtag("config",tag.config.conversion_id);if(tag.config.conversion_label){window.gtag("event","conversion",{send_to:tag.config.conversion_id+"/"+tag.config.conversion_label});}break;
case"linkedin":if(!window._linkedin_partner_id){window._linkedin_partner_id=tag.config.partner_id;window._linkedin_data_partner_ids=window._linkedin_data_partner_ids||[];window._linkedin_data_partner_ids.push(tag.config.partner_id);loadScript("https://snap.licdn.com/li.lms-analytics/insight.min.js");}break;
case"tiktok":if(!window.ttq){var tt=function(){tt.methods.forEach(function(m){tt[m]=function(){var a=Array.prototype.slice.call(arguments);a.unshift(m);tt.queue.push(a);};});};tt.methods=["page","track","identify","instances","debug","on","off","once","ready","alias","group","enableCookie","disableCookie"];tt.queue=[];tt();window.ttq=tt;loadScript("https://analytics.tiktok.com/i18n/pixel/events.js");window.ttq.load(tag.config.pixel_id);window.ttq.page();}break;
}
}
function evalTrigger(trigger,evType,evData){
var t=trigger.trigger_type,cfg=trigger.config||{};
if(t==="page_load"||t==="dom_ready")return true;
if(t==="click_all"&&evType==="click")return true;
if(t==="click_specific"&&evType==="click"){if(!cfg.selector)return false;return evData&&evData.target&&evData.target.closest&&!!evData.target.closest(cfg.selector);}
if(t==="custom_event"&&evType==="custom_event"&&evData===cfg.event_name)return true;
if(t==="scroll_depth"&&evType==="scroll_depth")return true;
if(t==="timer"&&evType==="timer")return true;
if(t==="history_change"&&evType==="history_change")return true;
if(t==="form_submit"&&evType==="form_submit"){if(!cfg.selector)return true;return evData&&evData.target&&evData.target.closest&&!!evData.target.closest(cfg.selector);}
return false;
}
function init(){
_cl.forEach(function(fn){fn();});_cl=[];
C.tags.sort(function(a,b){return(b.priority||0)-(a.priority||0);});
C.tags.forEach(function(tag){
var triggers=tag.trigger_ids.map(function(tid){return C.triggers.find(function(t){return t.id===tid;});}).filter(Boolean);
var immediate=triggers.length===0||triggers.some(function(tr){return evalTrigger(tr);});
if(immediate)fireTag(tag);
triggers.forEach(function(tr){
var t=tr.trigger_type,cfg=tr.config||{};
if(t==="click_all"||t==="click_specific"){var h=function(e){if(evalTrigger(tr,"click",{target:e.target}))fireTag(tag);};document.addEventListener("click",h);_cl.push(function(){document.removeEventListener("click",h);});}
if(t==="custom_event"&&cfg.event_name){var ce=function(){fireTag(tag);};window.addEventListener(cfg.event_name,ce);_cl.push(function(){window.removeEventListener(cfg.event_name,ce);});}
if(t==="scroll_depth"){var pct=parseInt(cfg.percentage,10)||50;var fired=false;var sh=function(){if(fired)return;var scrollTop=window.pageYOffset||document.documentElement.scrollTop;var docHeight=Math.max(document.body.scrollHeight,document.documentElement.scrollHeight)-window.innerHeight;if(docHeight>0&&(scrollTop/docHeight)*100>=pct){fired=true;fireTag(tag);}};window.addEventListener("scroll",sh,{passive:true});_cl.push(function(){window.removeEventListener("scroll",sh);});}
if(t==="timer"){var interval=parseInt(cfg.interval_ms,10)||5000;var limit=parseInt(cfg.limit,10)||0;var count=0;var tid=setInterval(function(){count++;fireTag(tag);if(limit>0&&count>=limit)clearInterval(tid);},interval);_cl.push(function(){clearInterval(tid);});}
if(t==="history_change"){var hp=function(){fireTag(tag);};window.addEventListener("popstate",hp);var origPush=history.pushState;var origReplace=history.replaceState;history.pushState=function(){origPush.apply(history,arguments);hp();};history.replaceState=function(){origReplace.apply(history,arguments);hp();};_cl.push(function(){window.removeEventListener("popstate",hp);history.pushState=origPush;history.replaceState=origReplace;});}
if(t==="form_submit"){var fh=function(e){if(evalTrigger(tr,"form_submit",{target:e.target}))fireTag(tag);};document.addEventListener("submit",fh);_cl.push(function(){document.removeEventListener("submit",fh);});}
});
});
}
window.addEventListener("etiquetta:consent",function(){consent=window.__ETIQUETTA_CONSENT__;init();});
if(document.readyState==="loading"){document.addEventListener("DOMContentLoaded",init);}else{init();}
})();`, snapshotJSON)
}
