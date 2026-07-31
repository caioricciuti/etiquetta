package alerts

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// WebhookEvent is the JSON body posted to both Slack-style URLs and
// generic webhooks. Slack ignores extra fields and uses only `text`.
type WebhookEvent struct {
	Event     string    `json:"event"`
	FiredAt   time.Time `json:"fired_at"`
	DomainID  string    `json:"domain_id"`
	AlertID   string    `json:"alert_id"`
	AlertName string    `json:"alert_name"`
	Metric    string    `json:"metric"`
	Threshold float64   `json:"threshold"`
	Observed  float64   `json:"observed"`
	Text      string    `json:"text"`
}

// Webhook is a configured outbound destination.
type Webhook struct {
	ID          string   `json:"id"`
	DomainID    string   `json:"domain_id"`
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Secret      string   `json:"secret"`
	Events      []string `json:"events"`
	Enabled     bool     `json:"enabled"`
	CreatedBy   string   `json:"created_by"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
	LastFiredAt *int64   `json:"last_fired_at,omitempty"`
	LastStatus  *int     `json:"last_status,omitempty"`
	LastError   string   `json:"last_error,omitempty"`
}

// SlackPayload is the minimal Slack-compatible body.
type SlackPayload struct {
	Text string `json:"text"`
}

func (d *Dispatcher) sendSlack(ctx context.Context, a Alert, f Fire) error {
	// Slack channels expect a Slack webhook URL stored alongside the alert
	// recipients as a URL prefixed with "slack:". Keeps schema simple.
	urls := filterPrefixed(a.Recipients, "slack:")
	if len(urls) == 0 {
		return fmt.Errorf("no slack recipients configured")
	}
	payload := SlackPayload{Text: slackMessage(a, f)}
	body, _ := json.Marshal(payload)

	var firstErr error
	for _, u := range urls {
		if err := postJSON(ctx, u, body, ""); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (d *Dispatcher) sendWebhooks(ctx context.Context, a Alert, f Fire) error {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, url, secret FROM webhooks WHERE domain_id = ? AND enabled = 1`,
		a.DomainID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	evt := WebhookEvent{
		Event:     "alert.fired",
		FiredAt:   time.UnixMilli(f.FiredAt),
		DomainID:  a.DomainID,
		AlertID:   a.ID,
		AlertName: a.Name,
		Metric:    a.Metric,
		Threshold: a.Threshold,
		Observed:  f.MetricValue,
		Text:      slackMessage(a, f),
	}
	body, _ := json.Marshal(evt)

	var firstErr error
	count := 0
	for rows.Next() {
		var id, url, secret string
		if err := rows.Scan(&id, &url, &secret); err != nil {
			continue
		}
		count++
		status, err := postJSONWithStatus(ctx, url, body, sign(body, secret))
		now := time.Now().UnixMilli()
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		}
		_, _ = d.db.ExecContext(ctx,
			`UPDATE webhooks SET last_fired_at = ?, last_status = ?, last_error = ? WHERE id = ?`,
			now, status, errMsg, id,
		)
	}
	if count == 0 {
		return fmt.Errorf("no webhooks configured for domain")
	}
	return firstErr
}

func slackMessage(a Alert, f Fire) string {
	op := "exceeded"
	if a.Comparator == CompLt {
		op = "fell below"
	}
	return fmt.Sprintf(
		":warning: *%s* %s threshold: %s = %g (threshold %g, window %d min)",
		a.Name, op, a.Metric, f.MetricValue, a.Threshold, a.WindowMinutes,
	)
}

func filterPrefixed(values []string, prefix string) []string {
	var out []string
	for _, v := range values {
		if len(v) > len(prefix) && v[:len(prefix)] == prefix {
			out = append(out, v[len(prefix):])
		}
	}
	return out
}

func postJSON(ctx context.Context, url string, body []byte, signature string) error {
	_, err := postJSONWithStatus(ctx, url, body, signature)
	return err
}

// allowPrivateWebhooks lets self-hosters deliberately target internal
// services (ntfy, an internal Slack proxy, …). Off by default: without it a
// webhook URL is an admin-reachable SSRF primitive into the host's network.
// Read lazily (not as a package var) so a value loaded from .env at startup is
// honored — a package var would freeze before .env is applied.
func allowPrivateWebhooks() bool {
	return os.Getenv("ETIQUETTA_ALLOW_PRIVATE_WEBHOOKS") == "true"
}

func isPublicWebhookIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	// CGNAT range (100.64.0.0/10) — covers cloud metadata-adjacent setups.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64 {
		return false
	}
	return true
}

// secureWebhookDial resolves the host itself and connects to the checked IP,
// so a DNS answer can't be swapped between validation and connect.
func secureWebhookDial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var lastErr error
	for _, ia := range ipAddrs {
		if !allowPrivateWebhooks() && !isPublicWebhookIP(ia.IP) {
			return nil, fmt.Errorf("webhook destination %s resolves to a private or internal address (set ETIQUETTA_ALLOW_PRIVATE_WEBHOOKS=true to allow)", host)
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ia.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no addresses for %s", host)
	}
	return nil, lastErr
}

var webhookClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Redirects would bypass the dial-time address check.
		return errors.New("webhook redirects are not allowed")
	},
	Transport: &http.Transport{
		DialContext:         secureWebhookDial,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

func postJSONWithStatus(ctx context.Context, rawURL string, body []byte, signature string) (int, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return 0, fmt.Errorf("invalid webhook URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Etiquetta-Webhook/1.0")
	if signature != "" {
		req.Header.Set("X-Etiquetta-Signature", signature)
	}

	resp, err := webhookClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	// Drain (bounded) for connection reuse, but never surface the body:
	// error strings reach the UI, and echoing responses would turn webhook
	// tests into a read primitive against whatever the URL points at.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// sign returns the hex-encoded HMAC-SHA256 of body using secret.
func sign(body []byte, secret string) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// TestFire sends a synthetic fire to a single webhook URL for UI testing.
func TestFire(ctx context.Context, url, secret string) error {
	evt := WebhookEvent{
		Event:     "test",
		FiredAt:   time.Now(),
		DomainID:  "test",
		AlertID:   "test",
		AlertName: "Test webhook",
		Metric:    "test",
		Threshold: 0,
		Observed:  1,
		Text:      ":white_check_mark: Etiquetta webhook test delivery",
	}
	body, _ := json.Marshal(evt)
	return postJSON(ctx, url, body, sign(body, secret))
}

// LoadWebhook fetches a single webhook by id for handler use.
func LoadWebhook(ctx context.Context, db *sql.DB, id string) (*Webhook, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, domain_id, name, url, secret, events, enabled, created_by,
		       created_at, updated_at, last_fired_at, last_status, last_error
		FROM webhooks WHERE id = ?
	`, id)
	var w Webhook
	var events string
	var enabled int
	var lastFired sql.NullInt64
	var lastStatus sql.NullInt64
	var lastError sql.NullString
	err := row.Scan(&w.ID, &w.DomainID, &w.Name, &w.URL, &w.Secret, &events, &enabled,
		&w.CreatedBy, &w.CreatedAt, &w.UpdatedAt, &lastFired, &lastStatus, &lastError)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(events), &w.Events)
	w.Enabled = enabled == 1
	if lastFired.Valid {
		v := lastFired.Int64
		w.LastFiredAt = &v
	}
	if lastStatus.Valid {
		v := int(lastStatus.Int64)
		w.LastStatus = &v
	}
	if lastError.Valid {
		w.LastError = lastError.String
	}
	return &w, nil
}
