package rollup

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

// setupDB creates a temp DuckDB with the minimal events + rollup_daily schema
// the refresher needs.
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE events (
			id VARCHAR, timestamp BIGINT, event_type VARCHAR, event_name VARCHAR,
			session_id VARCHAR, visitor_hash VARCHAR, domain VARCHAR,
			bot_category VARCHAR, props VARCHAR DEFAULT '{}'
		);
		CREATE TABLE rollup_daily (
			date_key VARCHAR NOT NULL, domain VARCHAR NOT NULL, bot_category VARCHAR NOT NULL,
			pageviews BIGINT DEFAULT 0, events BIGINT DEFAULT 0, sessions BIGINT DEFAULT 0,
			visitors BIGINT DEFAULT 0, bounce_sessions BIGINT DEFAULT 0,
			visible_ms_sum BIGINT DEFAULT 0, engagement_count BIGINT DEFAULT 0,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (date_key, domain, bot_category)
		);
	`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func insertEvent(t *testing.T, db *sql.DB, ts int64, etype, sess, vis, cat, props string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO events (id, timestamp, event_type, event_name, session_id, visitor_hash, domain, bot_category, props)
		 VALUES (?, ?, ?, '', ?, ?, 'example.com', ?, ?)`,
		sess+vis+etype+props, ts, etype, sess, vis, cat, props)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestRefreshAggregatesCorrectly(t *testing.T) {
	db := setupDB(t)
	day := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC).UnixMilli()

	// Session s1 (visitor v1): 2 pageviews -> not a bounce.
	insertEvent(t, db, day, "pageview", "s1", "v1", "human", `{}`)
	insertEvent(t, db, day+1000, "pageview", "s1", "v1", "human", `{}`)
	// Session s2 (visitor v2): 1 pageview -> bounce; plus an engagement w/ visible time.
	insertEvent(t, db, day+2000, "pageview", "s2", "v2", "human", `{}`)
	insertEvent(t, db, day+3000, "engagement", "s2", "v2", "human", `{"visible_time_ms": 5000}`)
	// A bad_bot event in a different category bucket.
	insertEvent(t, db, day+4000, "pageview", "s3", "v3", "bad_bot", `{}`)

	rf := NewRefresher(db, &sync.RWMutex{}, time.Hour)
	if err := rf.RefreshSince(context.Background(), 0); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Human bucket assertions.
	var pv, ev, sess, vis, bounce, visMs, eng int64
	err := db.QueryRow(`SELECT pageviews, events, sessions, visitors, bounce_sessions, visible_ms_sum, engagement_count
		FROM rollup_daily WHERE date_key='2026-05-20' AND bot_category='human'`).
		Scan(&pv, &ev, &sess, &vis, &bounce, &visMs, &eng)
	if err != nil {
		t.Fatalf("scan human: %v", err)
	}
	if pv != 3 {
		t.Errorf("pageviews = %d, want 3", pv)
	}
	if ev != 4 {
		t.Errorf("events = %d, want 4", ev)
	}
	if sess != 2 {
		t.Errorf("sessions = %d, want 2", sess)
	}
	if vis != 2 {
		t.Errorf("visitors = %d, want 2", vis)
	}
	if bounce != 1 {
		t.Errorf("bounce_sessions = %d, want 1 (s2 single pageview)", bounce)
	}
	if visMs != 5000 {
		t.Errorf("visible_ms_sum = %d, want 5000", visMs)
	}
	if eng != 1 {
		t.Errorf("engagement_count = %d, want 1", eng)
	}

	// Bad_bot bucket is separate.
	var bpv int64
	if err := db.QueryRow(`SELECT pageviews FROM rollup_daily WHERE date_key='2026-05-20' AND bot_category='bad_bot'`).Scan(&bpv); err != nil {
		t.Fatalf("scan bad_bot: %v", err)
	}
	if bpv != 1 {
		t.Errorf("bad_bot pageviews = %d, want 1", bpv)
	}
}

func TestRefreshIsIdempotent(t *testing.T) {
	db := setupDB(t)
	day := time.Date(2026, 5, 21, 8, 0, 0, 0, time.UTC).UnixMilli()
	insertEvent(t, db, day, "pageview", "s1", "v1", "human", `{}`)

	rf := NewRefresher(db, &sync.RWMutex{}, time.Hour)
	for i := 0; i < 3; i++ {
		if err := rf.RefreshSince(context.Background(), 0); err != nil {
			t.Fatalf("refresh %d: %v", i, err)
		}
	}

	var rows, pv int64
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(pageviews),0) FROM rollup_daily WHERE bot_category='human'`).Scan(&rows, &pv); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if rows != 1 {
		t.Errorf("rollup rows = %d, want 1 (upsert must not duplicate)", rows)
	}
	if pv != 1 {
		t.Errorf("pageviews = %d, want 1 (must overwrite, not accumulate)", pv)
	}
}

// TestRollupSumsMatchRaw is the core guarantee: additive metrics summed from
// rollup_daily equal a direct raw aggregation over the same day+category.
func TestRollupSumsMatchRaw(t *testing.T) {
	db := setupDB(t)
	base := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)

	// Spread events across 3 days, 2 categories, several sessions/visitors.
	for d := 0; d < 3; d++ {
		dayMs := base.AddDate(0, 0, d).UnixMilli()
		for s := 0; s < 5; s++ {
			sid := "s" + string(rune('a'+d)) + string(rune('0'+s))
			vid := "v" + string(rune('0'+(s%3)))
			insertEvent(t, db, dayMs+int64(s*1000), "pageview", sid, vid, "human", `{}`)
		}
		insertEvent(t, db, dayMs+9000, "pageview", "bot"+string(rune('0'+d)), "bv", "bad_bot", `{}`)
	}

	rf := NewRefresher(db, &sync.RWMutex{}, time.Hour)
	if err := rf.RefreshSince(context.Background(), 0); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Raw totals for the human bucket.
	var rawPV, rawSess int64
	db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT session_id) FROM events WHERE bot_category='human' AND event_type='pageview'`).Scan(&rawPV, &rawSess)

	var rollPV, rollSess int64
	db.QueryRow(`SELECT COALESCE(SUM(pageviews),0), COALESCE(SUM(sessions),0) FROM rollup_daily WHERE bot_category='human'`).Scan(&rollPV, &rollSess)

	if rawPV != rollPV {
		t.Errorf("pageviews: raw=%d rollup=%d", rawPV, rollPV)
	}
	if rawSess != rollSess {
		t.Errorf("sessions: raw=%d rollup=%d (each session lives in one day bucket)", rawSess, rollSess)
	}
}
