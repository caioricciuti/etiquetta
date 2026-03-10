package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
	mu   sync.RWMutex
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

	// Enable WAL mode and other optimizations via connection string
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=10000&_cache_size=-20000", path)

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Single connection serializes all writes — database/sql blocks goroutines
	// waiting for a connection, preventing SQLITE_BUSY entirely.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0)

	// Test connection
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Ensure PRAGMAs are applied (DSN _-prefixed params may not be parsed by modernc.org/sqlite)
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=10000",
		"PRAGMA cache_size=-20000",
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			return nil, fmt.Errorf("failed to set %s: %w", p, err)
		}
	}

	return &DB{conn: conn}, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) Conn() *sql.DB {
	return db.conn
}

// InsertEvent inserts a tracking event
func (db *DB) InsertEvent(e *Event) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	props := "{}"
	if e.Props != nil {
		props = string(e.Props)
	}

	botSignals := "[]"
	if e.BotSignals != "" {
		botSignals = e.BotSignals
	}

	botCategory := "human"
	if e.BotCategory != "" {
		botCategory = e.BotCategory
	}

	_, err := db.conn.Exec(`
		INSERT INTO events (
			id, timestamp, event_type, event_name, session_id, visitor_hash,
			domain, url, path, page_title, referrer_url, referrer_type,
			utm_source, utm_medium, utm_campaign,
			geo_country, geo_city, geo_region, geo_latitude, geo_longitude,
			browser_name, os_name, device_type, is_bot, props,
			bot_score, bot_signals, bot_category,
			has_scroll, has_mouse_move, has_click, has_touch,
			click_x, click_y, page_duration, datacenter_ip, ip_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		e.ID, e.Timestamp.UnixMilli(), e.EventType, e.EventName, e.SessionID, e.VisitorHash,
		e.Domain, e.URL, e.Path, e.PageTitle, e.ReferrerURL, e.ReferrerType,
		e.UTMSource, e.UTMMedium, e.UTMCampaign,
		e.GeoCountry, e.GeoCity, e.GeoRegion, e.GeoLatitude, e.GeoLongitude,
		e.BrowserName, e.OSName, e.DeviceType, e.IsBot, props,
		e.BotScore, botSignals, botCategory,
		e.HasScroll, e.HasMouseMove, e.HasClick, e.HasTouch,
		e.ClickX, e.ClickY, e.PageDuration, e.DatacenterIP, e.IPHash,
	)
	return err
}

// InsertPerformance inserts web vitals data
func (db *DB) InsertPerformance(p *Performance) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`
		INSERT INTO performance (
			id, timestamp, session_id, visitor_hash, domain, url, path,
			lcp, cls, fcp, ttfb, inp, page_load_time,
			device_type, connection_type, geo_country
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		p.ID, p.Timestamp.UnixMilli(), p.SessionID, p.VisitorHash, p.Domain, p.URL, p.Path,
		p.LCP, p.CLS, p.FCP, p.TTFB, p.INP, p.PageLoadTime,
		p.DeviceType, p.ConnectionType, p.GeoCountry,
	)
	return err
}

// InsertError inserts a JS error
func (db *DB) InsertError(e *Error) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`
		INSERT INTO errors (
			id, timestamp, session_id, visitor_hash, domain, url, path,
			error_type, error_message, error_stack, error_hash,
			script_url, line_number, column_number, browser_name, geo_country
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		e.ID, e.Timestamp.UnixMilli(), e.SessionID, e.VisitorHash, e.Domain, e.URL, e.Path,
		e.ErrorType, e.ErrorMessage, e.ErrorStack, e.ErrorHash,
		e.ScriptURL, e.LineNumber, e.ColumnNumber, e.BrowserName, e.GeoCountry,
	)
	return err
}

// InsertBatch inserts multiple events in a transaction
func (db *DB) InsertBatch(events []*Event, perfs []*Performance, errs []*Error) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Prepare statements
	eventStmt, err := tx.Prepare(`
		INSERT INTO events (
			id, timestamp, event_type, event_name, session_id, visitor_hash,
			domain, url, path, page_title, referrer_url, referrer_type,
			utm_source, utm_medium, utm_campaign,
			geo_country, geo_city, geo_region, geo_latitude, geo_longitude,
			browser_name, os_name, device_type, is_bot, props,
			bot_score, bot_signals, bot_category,
			has_scroll, has_mouse_move, has_click, has_touch,
			click_x, click_y, page_duration, datacenter_ip, ip_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer eventStmt.Close()

	perfStmt, err := tx.Prepare(`
		INSERT INTO performance (
			id, timestamp, session_id, visitor_hash, domain, url, path,
			lcp, cls, fcp, ttfb, inp, page_load_time,
			device_type, connection_type, geo_country
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer perfStmt.Close()

	errStmt, err := tx.Prepare(`
		INSERT INTO errors (
			id, timestamp, session_id, visitor_hash, domain, url, path,
			error_type, error_message, error_stack, error_hash,
			script_url, line_number, column_number, browser_name, geo_country
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer errStmt.Close()

	// Insert events
	for _, e := range events {
		props := "{}"
		if e.Props != nil {
			props = string(e.Props)
		}
		botSignals := "[]"
		if e.BotSignals != "" {
			botSignals = e.BotSignals
		}
		botCategory := "human"
		if e.BotCategory != "" {
			botCategory = e.BotCategory
		}
		_, err := eventStmt.Exec(
			e.ID, e.Timestamp.UnixMilli(), e.EventType, e.EventName, e.SessionID, e.VisitorHash,
			e.Domain, e.URL, e.Path, e.PageTitle, e.ReferrerURL, e.ReferrerType,
			e.UTMSource, e.UTMMedium, e.UTMCampaign,
			e.GeoCountry, e.GeoCity, e.GeoRegion, e.GeoLatitude, e.GeoLongitude,
			e.BrowserName, e.OSName, e.DeviceType, e.IsBot, props,
			e.BotScore, botSignals, botCategory,
			e.HasScroll, e.HasMouseMove, e.HasClick, e.HasTouch,
			e.ClickX, e.ClickY, e.PageDuration, e.DatacenterIP, e.IPHash,
		)
		if err != nil {
			return err
		}
	}

	// Insert performance
	for _, p := range perfs {
		_, err := perfStmt.Exec(
			p.ID, p.Timestamp.UnixMilli(), p.SessionID, p.VisitorHash, p.Domain, p.URL, p.Path,
			p.LCP, p.CLS, p.FCP, p.TTFB, p.INP, p.PageLoadTime,
			p.DeviceType, p.ConnectionType, p.GeoCountry,
		)
		if err != nil {
			return err
		}
	}

	// Insert errors
	for _, e := range errs {
		_, err := errStmt.Exec(
			e.ID, e.Timestamp.UnixMilli(), e.SessionID, e.VisitorHash, e.Domain, e.URL, e.Path,
			e.ErrorType, e.ErrorMessage, e.ErrorStack, e.ErrorHash,
			e.ScriptURL, e.LineNumber, e.ColumnNumber, e.BrowserName, e.GeoCountry,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetEventCount returns total event count
func (db *DB) GetEventCount() (int64, error) {
	var count int64
	err := db.conn.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	return count, err
}

// CleanupOldData removes data older than retentionDays
func (db *DB) CleanupOldData(retentionDays int) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -retentionDays).UnixMilli()

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec("DELETE FROM events WHERE timestamp < ?", cutoff)
	tx.Exec("DELETE FROM performance WHERE timestamp < ?", cutoff)
	tx.Exec("DELETE FROM errors WHERE timestamp < ?", cutoff)

	return tx.Commit()
}
