// Package mailer provides a minimal transactional email sender backed by
// either SMTP or Resend. Configuration is read from the shared settings
// service so operators can switch providers at runtime.
package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/caioricciuti/etiquetta/internal/settings"
)

// ErrDisabled is returned when no email provider is configured.
var ErrDisabled = errors.New("mailer: email provider is disabled")

// Message is a single transactional email.
type Message struct {
	To      []string
	Subject string
	HTML    string
	Text    string
}

// Mailer sends transactional email.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
	From() string
}

// FromSettings builds a Mailer from the settings service. Returns
// ErrDisabled when the provider is "disabled".
func FromSettings(svc *settings.Service) (Mailer, error) {
	provider := svc.GetWithDefault("email_provider", "disabled")
	from := svc.GetWithDefault("email_from_address", "")
	if from == "" {
		return nil, errors.New("mailer: email_from_address not configured")
	}

	switch provider {
	case "disabled", "":
		return nil, ErrDisabled
	case "smtp":
		host, _ := svc.Get("smtp_host")
		if host == "" {
			return nil, errors.New("mailer: smtp_host not configured")
		}
		user, _ := svc.Get("smtp_username")
		pass, _ := svc.Get("smtp_password")
		return &smtpMailer{
			from:   from,
			host:   host,
			port:   svc.GetInt("smtp_port", 587),
			user:   user,
			pass:   pass,
			useTLS: svc.GetBool("smtp_use_tls", true),
		}, nil
	case "resend":
		key, _ := svc.Get("resend_api_key")
		if key == "" {
			return nil, errors.New("mailer: resend_api_key not configured")
		}
		return &resendMailer{from: from, apiKey: key}, nil
	default:
		return nil, fmt.Errorf("mailer: unknown provider %q", provider)
	}
}

type smtpMailer struct {
	from   string
	host   string
	port   int
	user   string
	pass   string
	useTLS bool
}

func (m *smtpMailer) From() string { return m.from }

func (m *smtpMailer) Send(ctx context.Context, msg Message) error {
	if len(msg.To) == 0 {
		return errors.New("mailer: no recipients")
	}

	addr := net.JoinHostPort(m.host, fmt.Sprintf("%d", m.port))
	body := buildMIME(m.from, msg)

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}

	var client *smtp.Client
	if m.port == 465 {
		// Implicit TLS
		tlsConn := tls.Client(conn, &tls.Config{ServerName: m.host})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return fmt.Errorf("smtp tls: %w", err)
		}
		client, err = smtp.NewClient(tlsConn, m.host)
	} else {
		client, err = smtp.NewClient(conn, m.host)
	}
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if m.useTLS && m.port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: m.host}); err != nil {
				return fmt.Errorf("smtp starttls: %w", err)
			}
		}
	}

	if m.user != "" {
		auth := smtp.PlainAuth("", m.user, m.pass, m.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(extractEmail(m.from)); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	for _, to := range msg.To {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("smtp rcpt %s: %w", to, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}

	return client.Quit()
}

func buildMIME(from string, msg Message) []byte {
	var buf bytes.Buffer
	boundary := fmt.Sprintf("etq_boundary_%d", time.Now().UnixNano())

	fmt.Fprintf(&buf, "From: %s\r\n", sanitizeHeader(from))
	fmt.Fprintf(&buf, "To: %s\r\n", sanitizeHeader(strings.Join(msg.To, ", ")))
	fmt.Fprintf(&buf, "Subject: %s\r\n", mimeEncodeHeader(msg.Subject))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")

	if msg.HTML != "" && msg.Text != "" {
		fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
		fmt.Fprintf(&buf, "--%s\r\n", boundary)
		fmt.Fprintf(&buf, "Content-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n\r\n", msg.Text)
		fmt.Fprintf(&buf, "--%s\r\n", boundary)
		fmt.Fprintf(&buf, "Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n\r\n", msg.HTML)
		fmt.Fprintf(&buf, "--%s--\r\n", boundary)
	} else if msg.HTML != "" {
		fmt.Fprintf(&buf, "Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n", msg.HTML)
	} else {
		fmt.Fprintf(&buf, "Content-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", msg.Text)
	}

	return buf.Bytes()
}

// sanitizeHeader strips CR/LF so a value can never inject additional MIME
// headers (e.g. an alert named "x\r\nBcc: ...").
func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	return strings.ReplaceAll(s, "\n", "")
}

func mimeEncodeHeader(s string) string {
	for _, r := range s {
		if r > 127 || r == '\r' || r == '\n' {
			return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
		}
	}
	return s
}

// extractEmail pulls "user@host" out of "Name <user@host>" or returns the input.
func extractEmail(addr string) string {
	if i := strings.LastIndex(addr, "<"); i >= 0 {
		if j := strings.LastIndex(addr, ">"); j > i {
			return addr[i+1 : j]
		}
	}
	return strings.TrimSpace(addr)
}

type resendMailer struct {
	from   string
	apiKey string
}

func (m *resendMailer) From() string { return m.from }

func (m *resendMailer) Send(ctx context.Context, msg Message) error {
	if len(msg.To) == 0 {
		return errors.New("mailer: no recipients")
	}

	payload := map[string]any{
		"from":    m.from,
		"to":      msg.To,
		"subject": msg.Subject,
	}
	if msg.HTML != "" {
		payload["html"] = msg.HTML
	}
	if msg.Text != "" {
		payload["text"] = msg.Text
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("resend request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
