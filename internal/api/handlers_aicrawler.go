package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/caioricciuti/etiquetta/internal/bot"
)

// ServeRobotsTxt generates a robots.txt based on AI crawler settings
func (h *Handlers) ServeRobotsTxt(w http.ResponseWriter, r *http.Request) {
	// Load rules from settings
	var rulesJSON string
	err := h.db.Conn().QueryRow("SELECT value FROM settings WHERE key = 'ai_crawler_rules'").Scan(&rulesJSON)

	rules := make(map[string]string) // crawler name -> "allow" or "block"
	if err == nil && rulesJSON != "" {
		json.Unmarshal([]byte(rulesJSON), &rules)
	}

	var sb strings.Builder

	// Default: allow all standard crawlers
	sb.WriteString("User-agent: *\nAllow: /\n\n")

	// AI crawler rules
	for _, name := range bot.GetAICrawlersList() {
		action, ok := rules[name]
		if !ok || action == "block" {
			// Default to block for AI crawlers
			sb.WriteString(fmt.Sprintf("User-agent: %s\nDisallow: /\n\n", name))
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(sb.String()))
}

// GetAICrawlerSettings returns known AI crawlers and current rules
func (h *Handlers) GetAICrawlerSettings(w http.ResponseWriter, r *http.Request) {
	var rulesJSON string
	err := h.db.Conn().QueryRow("SELECT value FROM settings WHERE key = 'ai_crawler_rules'").Scan(&rulesJSON)

	rules := make(map[string]string)
	if err == nil && rulesJSON != "" {
		json.Unmarshal([]byte(rulesJSON), &rules)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"known_crawlers": bot.GetAICrawlersList(),
		"rules":          rules,
	})
}

// UpdateAICrawlerSettings saves AI crawler robots.txt rules
func (h *Handlers) UpdateAICrawlerSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Rules map[string]string `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	rulesJSON, _ := json.Marshal(body.Rules)

	_, err := h.db.Conn().Exec(
		"INSERT INTO settings (key, value, updated_at) VALUES ('ai_crawler_rules', ?, ?) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at",
		string(rulesJSON), time.Now().UnixMilli(),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.logAudit(r, "update", "settings", "ai_crawler_rules", "Updated AI crawler rules")
	w.WriteHeader(http.StatusNoContent)
}
