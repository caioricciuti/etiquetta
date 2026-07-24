package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/mailer"
	"github.com/caioricciuti/etiquetta/internal/settings"
)

// EmailSettings represents the email configuration
type EmailSettings struct {
	Provider    string `json:"email_provider"`
	FromAddress string `json:"email_from_address"`
	BaseURL     string `json:"email_base_url"`
	SMTPHost    string `json:"smtp_host"`
	SMTPPort    int    `json:"smtp_port"`
	SMTPUser    string `json:"smtp_username"`
	SMTPPass    string `json:"smtp_password"`
	SMTPUseTLS  bool   `json:"smtp_use_tls"`
	ResendKey   string `json:"resend_api_key"`
}
func newSettingsService(h *Handlers) *settings.Service {
	svc := settings.New(h.db.Conn())
	secretKey, _ := svc.Get("secret_key")
	if secretKey != "" {
		svc.SetMasterKey(secretKey)
	}
	return svc
}

// GetEmailSettings returns the current email settings (with masked credentials)
func (h *Handlers) GetEmailSettings(w http.ResponseWriter, r *http.Request) {
	svc := newSettingsService(h)

	provider := svc.GetWithDefault("email_provider", "disabled")
	fromAddress, _ := svc.Get("email_from_address")
	baseURL, _ := svc.Get("email_base_url")
	smtpHost, _ := svc.Get("smtp_host")
	smtpPort := svc.GetInt("smtp_port", 587)
	smtpUser, _ := svc.Get("smtp_username")
	smtpPass, _ := svc.Get("smtp_password")
	smtpTLS := svc.GetBool("smtp_use_tls", true)
	resendKey, _ := svc.Get("resend_api_key")

	// Mask sensitive values
	maskedPass := ""
	if smtpPass != "" {
		maskedPass = "••••••••"
	}
	maskedResend := ""
	if resendKey != "" {
		maskedResend = "••••••••"
	}

	writeJSON(w, http.StatusOK, EmailSettings{
		Provider:    provider,
		FromAddress: fromAddress,
		BaseURL:     baseURL,
		SMTPHost:    smtpHost,
		SMTPPort:    smtpPort,
		SMTPUser:    smtpUser,
		SMTPPass:    maskedPass,
		SMTPUseTLS:  smtpTLS,
		ResendKey:   maskedResend,
	})
}

// UpdateEmailSettings updates the email settings
func (h *Handlers) UpdateEmailSettings(w http.ResponseWriter, r *http.Request) {
	var input map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	svc := newSettingsService(h)

	keyMap := map[string]string{
		"email_provider":     "email_provider",
		"email_from_address": "email_from_address",
		"email_base_url":     "email_base_url",
		"smtp_host":          "smtp_host",
		"smtp_port":          "smtp_port",
		"smtp_username":      "smtp_username",
		"smtp_password":      "smtp_password",
		"smtp_use_tls":       "smtp_use_tls",
		"resend_api_key":     "resend_api_key",
	}

	for jsonKey, settingKey := range keyMap {
		val, ok := input[jsonKey]
		if !ok {
			continue
		}

		var strVal string
		switch v := val.(type) {
		case string:
			strVal = v
		case float64:
			strVal = strconv.Itoa(int(v))
		case bool:
			if v {
				strVal = "true"
			} else {
				strVal = "false"
			}
		default:
			strVal = fmt.Sprintf("%v", v)
		}

		svc.Set(settingKey, strVal)
	}

	h.logAudit(r, "update", "settings", "email", "Email settings updated")
	w.WriteHeader(http.StatusNoContent)
}

// TestEmailSettings sends a real test email using the configured provider.
// Accepts an optional {"to": "address@example.com"} in the request body;
// falls back to the authenticated user's email.
func (h *Handlers) TestEmailSettings(w http.ResponseWriter, r *http.Request) {
	svc := newSettingsService(h)

	var body struct {
		To string `json:"to"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	to := body.To
	if to == "" {
		if claims := auth.GetUserFromContext(r.Context()); claims != nil {
			to = claims.Email
		}
	}
	if to == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": "No recipient address provided",
		})
		return
	}

	m, err := mailer.FromSettings(svc)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	provider := svc.GetWithDefault("email_provider", "disabled")
	msg, err := mailer.Render("test", "", map[string]string{
		"Provider": provider,
		"From":     m.From(),
		"SentAt":   time.Now().UTC().Format(time.RFC3339),
	}, []string{to})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": "Render failed: " + err.Error(),
		})
		return
	}

	if err := m.Send(r.Context(), msg); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Test email sent to %s via %s", to, provider),
	})
}
