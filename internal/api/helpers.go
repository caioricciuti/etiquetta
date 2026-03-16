package api

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"
)

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getStringOr(m map[string]interface{}, key, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

func getFloatOr(m map[string]interface{}, key string, def float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return def
}

func getBoolFromFloat(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(float64); ok {
		return v != 0
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func hashIP(ip string) string {
	h := md5.Sum([]byte(ip))
	return hex.EncodeToString(h[:8])
}

// getBotFilterCondition returns SQL condition for bot filtering
func getBotFilterCondition(filter string) string {
	switch filter {
	case "all":
		return "1=1"
	case "humans":
		return "bot_category = 'human'"
	case "good_bots":
		return "bot_category = 'good_bot'"
	case "bad_bots":
		return "bot_category = 'bad_bot'"
	case "suspicious":
		return "bot_category = 'suspicious'"
	case "bots":
		return "is_bot = 1"
	default:
		// Default: exclude bots (maintain backward compatibility)
		return "is_bot = 0"
	}
}

func getDaysParam(r *http.Request, defaultVal int) int {
	if d := r.URL.Query().Get("days"); d != "" {
		if days, err := strconv.Atoi(d); err == nil && days > 0 && days <= 365 {
			return days
		}
	}
	return defaultVal
}

func getDomainParam(r *http.Request) string {
	return r.URL.Query().Get("domain")
}

// getDateRangeParams parses start/end ISO strings or falls back to days parameter
// Returns startMs and endMs as millisecond timestamps for queries
func getDateRangeParams(r *http.Request, defaultDays int) (startMs, endMs int64) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr != "" && endStr != "" {
		// Parse ISO 8601 / RFC3339 strings from JavaScript's toISOString()
		startTime, errS := time.Parse(time.RFC3339, startStr)
		endTime, errE := time.Parse(time.RFC3339, endStr)

		if errS == nil && errE == nil {
			return startTime.UTC().UnixMilli(), endTime.UTC().UnixMilli()
		}
	}

	// Fallback to days parameter
	days := getDaysParam(r, defaultDays)
	now := time.Now()
	return now.Add(-time.Duration(days) * 24 * time.Hour).UnixMilli(), now.UnixMilli()
}
