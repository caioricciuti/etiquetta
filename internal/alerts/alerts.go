// Package alerts evaluates configured alert rules against recent event
// data and dispatches fires over email, Slack, and generic webhooks.
package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/caioricciuti/etiquetta/internal/mailer"
	"github.com/caioricciuti/etiquetta/internal/settings"
)

// Metric identifiers
const (
	MetricPageviews  = "pageviews"
	MetricVisitors   = "visitors"
	MetricErrorRate  = "error_rate"
	MetricErrorCount = "errors_count"
	MetricBotRate    = "bot_rate"
)

// Comparators
const (
	CompGt = "gt"
	CompLt = "lt"
)

// Channels
const (
	ChannelEmail   = "email"
	ChannelSlack   = "slack"
	ChannelWebhook = "webhook"
)

// Alert is a single configured rule.
type Alert struct {
	ID              string   `json:"id"`
	DomainID        string   `json:"domain_id"`
	Name            string   `json:"name"`
	Metric          string   `json:"metric"`
	Comparator      string   `json:"comparator"`
	Threshold       float64  `json:"threshold"`
	WindowMinutes   int      `json:"window_minutes"`
	CooldownMinutes int      `json:"cooldown_minutes"`
	Channels        []string `json:"channels"`
	Recipients      []string `json:"recipients"`
	Enabled         bool     `json:"enabled"`
	CreatedBy       string   `json:"created_by"`
	CreatedAt       int64    `json:"created_at"`
	UpdatedAt       int64    `json:"updated_at"`
}

// Fire records a single alert firing.
type Fire struct {
	ID           string   `json:"id"`
	AlertID      string   `json:"alert_id"`
	DomainID     string   `json:"domain_id"`
	FiredAt      int64    `json:"fired_at"`
	MetricValue  float64  `json:"metric_value"`
	Threshold    float64  `json:"threshold"`
	ChannelsSent []string `json:"channels_sent"`
	Error        string   `json:"error,omitempty"`
}

// Evaluator periodically runs all enabled alerts.
type Evaluator struct {
	db         *sql.DB
	dbMu       *sync.RWMutex
	settings   *settings.Service
	dispatcher *Dispatcher
	interval   time.Duration
}

// NewEvaluator constructs an Evaluator with default 5-minute cadence. dbMu is
// the shared DuckDB access lock (compaction takes the write lock); every query
// against compacted tables must hold the read lock or risk the heap-corruption
// class of bug that lock exists to prevent.
func NewEvaluator(db *sql.DB, svc *settings.Service, dbMu *sync.RWMutex) *Evaluator {
	return &Evaluator{
		db:         db,
		dbMu:       dbMu,
		settings:   svc,
		dispatcher: NewDispatcher(db, svc),
		interval:   5 * time.Minute,
	}
}

// withReadLock runs fn while holding the DuckDB read lock, keeping network
// dispatch (which can block for seconds) outside the critical section.
func (e *Evaluator) withReadLock(fn func() error) error {
	if e.dbMu != nil {
		e.dbMu.RLock()
		defer e.dbMu.RUnlock()
	}
	return fn()
}

// Start runs the evaluation loop until ctx is cancelled.
func (e *Evaluator) Start(ctx context.Context) {
	t := time.NewTicker(e.interval)
	defer t.Stop()

	// Run once immediately so alerts fire shortly after server boot.
	e.runOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.runOnce(ctx)
		}
	}
}

func (e *Evaluator) runOnce(ctx context.Context) {
	var alerts []Alert
	err := e.withReadLock(func() error {
		var loadErr error
		alerts, loadErr = loadEnabledAlerts(ctx, e.db)
		return loadErr
	})
	if err != nil {
		log.Printf("alerts: load failed: %v", err)
		return
	}
	for _, a := range alerts {
		if err := e.evaluate(ctx, a); err != nil {
			log.Printf("alerts: evaluate %s: %v", a.ID, err)
		}
	}
}

func (e *Evaluator) evaluate(ctx context.Context, a Alert) error {
	var value float64
	skip := false
	err := e.withReadLock(func() error {
		if inCooldown(ctx, e.db, a) {
			skip = true
			return nil
		}
		// events.domain stores the hostname; alerts reference domains.id.
		domainName, err := domainNameByID(ctx, e.db, a.DomainID)
		if err != nil {
			return fmt.Errorf("resolve domain %s: %w", a.DomainID, err)
		}
		value, err = measure(ctx, e.db, a, domainName)
		return err
	})
	if err != nil {
		return err
	}
	if skip || !triggered(a, value) {
		return nil
	}

	fire := Fire{
		ID:          newID(),
		AlertID:     a.ID,
		DomainID:    a.DomainID,
		FiredAt:     time.Now().UnixMilli(),
		MetricValue: value,
		Threshold:   a.Threshold,
	}

	sent, dispatchErr := e.dispatcher.Dispatch(ctx, a, fire)
	fire.ChannelsSent = sent
	if dispatchErr != nil {
		fire.Error = dispatchErr.Error()
	}

	return e.withReadLock(func() error {
		return recordFire(ctx, e.db, fire)
	})
}

func loadEnabledAlerts(ctx context.Context, db *sql.DB) ([]Alert, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, domain_id, name, metric, comparator, threshold,
		       window_minutes, cooldown_minutes, channels, recipients,
		       enabled, created_by, created_at, updated_at
		FROM alerts WHERE enabled = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Alert
	for rows.Next() {
		var a Alert
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
	return out, nil
}

func inCooldown(ctx context.Context, db *sql.DB, a Alert) bool {
	if a.CooldownMinutes <= 0 {
		return false
	}
	cutoff := time.Now().Add(-time.Duration(a.CooldownMinutes) * time.Minute).UnixMilli()
	var last sql.NullInt64
	err := db.QueryRowContext(ctx,
		"SELECT MAX(fired_at) FROM alert_fires WHERE alert_id = ?",
		a.ID,
	).Scan(&last)
	if err != nil {
		return false
	}
	return last.Valid && last.Int64 > cutoff
}

// domainNameByID resolves a domains.id to its hostname.
func domainNameByID(ctx context.Context, db *sql.DB, domainID string) (string, error) {
	var name string
	err := db.QueryRowContext(ctx,
		"SELECT domain FROM domains WHERE id = ?", domainID).Scan(&name)
	return name, err
}

// measure returns the metric value over the alert's window. domainName is the
// hostname stored in events.domain / errors.domain.
func measure(ctx context.Context, db *sql.DB, a Alert, domainName string) (float64, error) {
	end := time.Now().UnixMilli()
	start := time.Now().Add(-time.Duration(a.WindowMinutes) * time.Minute).UnixMilli()

	switch a.Metric {
	case MetricPageviews:
		return scanFloat(db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM events WHERE domain = ? AND event_type = 'pageview'
			 AND is_bot = 0 AND timestamp BETWEEN ? AND ?`,
			domainName, start, end))
	case MetricVisitors:
		return scanFloat(db.QueryRowContext(ctx,
			`SELECT COUNT(DISTINCT visitor_hash) FROM events WHERE domain = ?
			 AND is_bot = 0 AND timestamp BETWEEN ? AND ?`,
			domainName, start, end))
	case MetricErrorCount:
		return scanFloat(db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM errors WHERE domain = ? AND timestamp BETWEEN ? AND ?`,
			domainName, start, end))
	case MetricErrorRate:
		return scanRate(db, ctx,
			`SELECT COUNT(*) FROM errors WHERE domain = ? AND timestamp BETWEEN ? AND ?`,
			`SELECT COUNT(*) FROM events WHERE domain = ? AND event_type = 'pageview' AND timestamp BETWEEN ? AND ?`,
			domainName, start, end)
	case MetricBotRate:
		return scanRate(db, ctx,
			`SELECT COUNT(*) FROM events WHERE domain = ? AND is_bot = 1 AND timestamp BETWEEN ? AND ?`,
			`SELECT COUNT(*) FROM events WHERE domain = ? AND timestamp BETWEEN ? AND ?`,
			domainName, start, end)
	default:
		return 0, fmt.Errorf("unknown metric %q", a.Metric)
	}
}

func scanFloat(row *sql.Row) (float64, error) {
	var v float64
	err := row.Scan(&v)
	return v, err
}

func scanRate(db *sql.DB, ctx context.Context, numSQL, denSQL, domain string, start, end int64) (float64, error) {
	var num, den float64
	if err := db.QueryRowContext(ctx, numSQL, domain, start, end).Scan(&num); err != nil {
		return 0, err
	}
	if err := db.QueryRowContext(ctx, denSQL, domain, start, end).Scan(&den); err != nil {
		return 0, err
	}
	if den == 0 {
		return 0, nil
	}
	return num / den * 100, nil
}

func triggered(a Alert, value float64) bool {
	switch a.Comparator {
	case CompLt:
		return value < a.Threshold
	default:
		return value > a.Threshold
	}
}

func recordFire(ctx context.Context, db *sql.DB, f Fire) error {
	channels, _ := json.Marshal(f.ChannelsSent)
	_, err := db.ExecContext(ctx, `
		INSERT INTO alert_fires (id, alert_id, domain_id, fired_at, metric_value, threshold, channels_sent, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, f.ID, f.AlertID, f.DomainID, f.FiredAt, f.MetricValue, f.Threshold, string(channels), f.Error)
	return err
}

// Dispatcher sends fires to configured channels.
type Dispatcher struct {
	db       *sql.DB
	settings *settings.Service
}

func NewDispatcher(db *sql.DB, svc *settings.Service) *Dispatcher {
	return &Dispatcher{db: db, settings: svc}
}

// Dispatch attempts every configured channel, returning the names of those
// that succeeded and the first error encountered (if any).
func (d *Dispatcher) Dispatch(ctx context.Context, a Alert, f Fire) ([]string, error) {
	var sent []string
	var firstErr error

	for _, ch := range a.Channels {
		var err error
		switch ch {
		case ChannelEmail:
			err = d.sendEmail(ctx, a, f)
		case ChannelSlack:
			err = d.sendSlack(ctx, a, f)
		case ChannelWebhook:
			err = d.sendWebhooks(ctx, a, f)
		default:
			err = fmt.Errorf("unknown channel %q", ch)
		}
		if err == nil {
			sent = append(sent, ch)
		} else if firstErr == nil {
			firstErr = err
		}
	}
	return sent, firstErr
}

func (d *Dispatcher) sendEmail(ctx context.Context, a Alert, f Fire) error {
	if len(a.Recipients) == 0 {
		return fmt.Errorf("no email recipients configured")
	}
	m, err := mailer.FromSettings(d.settings)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("[Etiquetta] %s triggered", a.Name)
	text := fmt.Sprintf(
		"Alert: %s\nDomain: %s\nMetric: %s %s %g\nObserved: %g\nWindow: %d min\n",
		a.Name, a.DomainID, a.Metric, a.Comparator, a.Threshold, f.MetricValue, a.WindowMinutes,
	)
	return m.Send(ctx, mailer.Message{
		To:      a.Recipients,
		Subject: subject,
		Text:    text,
	})
}

// newID returns a short random hex identifier. Uses crypto/rand via the
// shared helper in this package (see ids.go).
func newID() string { return generateID() }
