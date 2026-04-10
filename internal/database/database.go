package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

type DB struct {
	conn *sql.DB
}

// Event represents a tracking event
type Event struct {
	ID           string          `json:"id"`
	Timestamp    time.Time       `json:"timestamp"`
	EventType    string          `json:"event_type"`
	EventName    *string         `json:"event_name,omitempty"`
	SessionID    string          `json:"session_id"`
	VisitorHash  string          `json:"visitor_hash"`
	Domain       string          `json:"domain"`
	URL          string          `json:"url"`
	Path         string          `json:"path"`
	PageTitle    *string         `json:"page_title,omitempty"`
	ReferrerURL  *string         `json:"referrer_url,omitempty"`
	ReferrerType *string         `json:"referrer_type,omitempty"`
	UTMSource    *string         `json:"utm_source,omitempty"`
	UTMMedium    *string         `json:"utm_medium,omitempty"`
	UTMCampaign  *string         `json:"utm_campaign,omitempty"`
	GeoCountry   *string         `json:"geo_country,omitempty"`
	GeoCity      *string         `json:"geo_city,omitempty"`
	GeoRegion    *string         `json:"geo_region,omitempty"`
	GeoLatitude  *float64        `json:"geo_latitude,omitempty"`
	GeoLongitude *float64        `json:"geo_longitude,omitempty"`
	BrowserName  *string         `json:"browser_name,omitempty"`
	OSName       *string         `json:"os_name,omitempty"`
	DeviceType   *string         `json:"device_type,omitempty"`
	IsBot        bool            `json:"is_bot"`
	Props        json.RawMessage `json:"props,omitempty"`

	// Bot detection fields
	BotScore     int     `json:"bot_score"`
	BotSignals   string  `json:"bot_signals"`
	BotCategory  string  `json:"bot_category"`
	HasScroll    bool    `json:"has_scroll"`
	HasMouseMove bool    `json:"has_mouse_move"`
	HasClick     bool    `json:"has_click"`
	HasTouch     bool    `json:"has_touch"`
	ClickX       *int    `json:"click_x,omitempty"`
	ClickY       *int    `json:"click_y,omitempty"`
	PageDuration *int    `json:"page_duration,omitempty"`
	DatacenterIP bool    `json:"datacenter_ip"`
	IPHash       *string `json:"ip_hash,omitempty"`
}

// Performance represents web vitals
type Performance struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	SessionID      string    `json:"session_id"`
	VisitorHash    string    `json:"visitor_hash"`
	Domain         string    `json:"domain"`
	URL            string    `json:"url"`
	Path           string    `json:"path"`
	LCP            *float64  `json:"lcp,omitempty"`
	CLS            *float64  `json:"cls,omitempty"`
	FCP            *float64  `json:"fcp,omitempty"`
	TTFB           *float64  `json:"ttfb,omitempty"`
	INP            *float64  `json:"inp,omitempty"`
	PageLoadTime   *float64  `json:"page_load_time,omitempty"`
	DeviceType     *string   `json:"device_type,omitempty"`
	ConnectionType *string   `json:"connection_type,omitempty"`
	GeoCountry     *string   `json:"geo_country,omitempty"`
	BotScore       int       `json:"bot_score"`
	BotCategory    string    `json:"bot_category"`
}

// Error represents a JS error
type Error struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	SessionID    string    `json:"session_id"`
	VisitorHash  string    `json:"visitor_hash"`
	Domain       string    `json:"domain"`
	URL          string    `json:"url"`
	Path         string    `json:"path"`
	ErrorType    string    `json:"error_type"`
	ErrorMessage string    `json:"error_message"`
	ErrorStack   *string   `json:"error_stack,omitempty"`
	ErrorHash    string    `json:"error_hash"`
	ScriptURL    *string   `json:"script_url,omitempty"`
	LineNumber   *int      `json:"line_number,omitempty"`
	ColumnNumber *int      `json:"column_number,omitempty"`
	BrowserName  *string   `json:"browser_name,omitempty"`
	GeoCountry   *string   `json:"geo_country,omitempty"`
}

func New(path string) (*DB, error) {
	// Ensure data directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	conn, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(30 * time.Minute)

	// Test connection
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{conn: conn}, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) Conn() *sql.DB {
	return db.conn
}

// GetEventCount returns total event count
func (db *DB) GetEventCount() (int64, error) {
	var count int64
	err := db.conn.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	return count, err
}

// LookupVisitorData counts records across all tables for a given visitor hash
func (db *DB) LookupVisitorData(visitorHash string) (map[string]int64, error) {
	counts := map[string]int64{}

	tables := []struct {
		name   string
		column string
	}{
		{"events", "visitor_hash"},
		{"performance", "visitor_hash"},
		{"errors", "visitor_hash"},
		{"consent_records", "visitor_hash"},
		{"visitor_sessions", "visitor_hash"},
	}

	for _, t := range tables {
		var count int64
		err := db.conn.QueryRow(
			fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", t.name, t.column),
			visitorHash,
		).Scan(&count)
		if err != nil {
			// Table may not exist yet — treat as zero
			counts[t.name] = 0
			continue
		}
		counts[t.name] = count
	}

	return counts, nil
}

// EraseVisitorData deletes all data for a visitor hash across all tables (GDPR Art. 17)
func (db *DB) EraseVisitorData(visitorHash string) (map[string]int64, error) {
	counts := map[string]int64{}

	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	tables := []struct {
		name   string
		column string
	}{
		{"events", "visitor_hash"},
		{"performance", "visitor_hash"},
		{"errors", "visitor_hash"},
		{"consent_records", "visitor_hash"},
		{"visitor_sessions", "visitor_hash"},
	}

	for _, t := range tables {
		result, err := tx.Exec(
			fmt.Sprintf("DELETE FROM %s WHERE %s = ?", t.name, t.column),
			visitorHash,
		)
		if err != nil {
			// Table may not exist — skip
			counts[t.name] = 0
			continue
		}
		affected, _ := result.RowsAffected()
		counts[t.name] = affected
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return counts, nil
}

// VisitorTableData holds column names and row data for one table
type VisitorTableData struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	Count   int             `json:"count"`
}

// VisitorDataExport holds all data for a single visitor across all tables
type VisitorDataExport struct {
	VisitorHash string                       `json:"visitor_hash"`
	Tables      map[string]*VisitorTableData `json:"tables"`
	TotalRows   int                          `json:"total_rows"`
	ExportedAt  string                       `json:"exported_at"`
}

// ExportVisitorData returns all data for a visitor hash across all tables (GDPR Art. 15)
func (db *DB) ExportVisitorData(visitorHash string) (*VisitorDataExport, error) {
	export := &VisitorDataExport{
		VisitorHash: visitorHash,
		Tables:      make(map[string]*VisitorTableData),
		ExportedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	tableNames := []string{"events", "performance", "errors", "consent_records", "visitor_sessions"}

	for _, table := range tableNames {
		rows, err := db.conn.Query(
			fmt.Sprintf("SELECT * FROM %s WHERE visitor_hash = ? LIMIT 10000", table),
			visitorHash,
		)
		if err != nil {
			export.Tables[table] = &VisitorTableData{Columns: []string{}, Rows: [][]interface{}{}}
			continue
		}

		cols, _ := rows.Columns()
		td := &VisitorTableData{Columns: cols}

		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				continue
			}
			// Convert []byte to string for JSON compatibility
			row := make([]interface{}, len(cols))
			for i, v := range values {
				if b, ok := v.([]byte); ok {
					row[i] = string(b)
				} else {
					row[i] = v
				}
			}
			td.Rows = append(td.Rows, row)
		}
		rows.Close()

		if td.Rows == nil {
			td.Rows = [][]interface{}{}
		}
		td.Count = len(td.Rows)
		export.Tables[table] = td
		export.TotalRows += td.Count
	}

	return export, nil
}

// AuditLogEntry represents an admin audit log entry
type AuditLogEntry struct {
	ID           string `json:"id"`
	Timestamp    int64  `json:"timestamp"`
	UserID       string `json:"user_id"`
	UserEmail    string `json:"user_email"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Detail       string `json:"detail"`
	IPAddress    string `json:"ip_address"`
}

// InsertAuditLog records an admin action
func (db *DB) InsertAuditLog(entry *AuditLogEntry) error {
	_, err := db.conn.Exec(`
		INSERT INTO audit_log (id, timestamp, user_id, user_email, action, resource_type, resource_id, detail, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.Timestamp, entry.UserID, entry.UserEmail, entry.Action, entry.ResourceType, entry.ResourceID, entry.Detail, entry.IPAddress)
	return err
}

// QueryAuditLog retrieves paginated audit log entries with optional filters
func (db *DB) QueryAuditLog(page, perPage int, action, resourceType string) ([]AuditLogEntry, int64, error) {
	where := "1=1"
	var args []interface{}

	if action != "" {
		where += " AND action = ?"
		args = append(args, action)
	}
	if resourceType != "" {
		where += " AND resource_type = ?"
		args = append(args, resourceType)
	}

	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	db.conn.QueryRow("SELECT COUNT(*) FROM audit_log WHERE "+where, countArgs...).Scan(&total)

	offset := (page - 1) * perPage
	args = append(args, perPage, offset)
	rows, err := db.conn.Query(
		"SELECT id, timestamp, user_id, user_email, action, resource_type, COALESCE(resource_id, ''), COALESCE(detail, ''), COALESCE(ip_address, '') FROM audit_log WHERE "+where+" ORDER BY timestamp DESC LIMIT ? OFFSET ?",
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []AuditLogEntry
	for rows.Next() {
		var e AuditLogEntry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.UserID, &e.UserEmail, &e.Action, &e.ResourceType, &e.ResourceID, &e.Detail, &e.IPAddress); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []AuditLogEntry{}
	}

	return entries, total, nil
}

// CleanupOldData removes data older than retentionDays
func (db *DB) CleanupOldData(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UnixMilli()

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec("DELETE FROM events WHERE timestamp < ?", cutoff)
	tx.Exec("DELETE FROM performance WHERE timestamp < ?", cutoff)
	tx.Exec("DELETE FROM errors WHERE timestamp < ?", cutoff)
	tx.Exec("DELETE FROM session_recordings WHERE start_time < ?", cutoff)

	return tx.Commit()
}
