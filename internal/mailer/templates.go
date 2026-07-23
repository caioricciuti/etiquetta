package mailer

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
	textTemplate "text/template"
)

//go:embed templates/*.html templates/*.txt
var templateFS embed.FS

// Render builds a Message from a template name and data. The template name
// corresponds to files under internal/mailer/templates/: <name>.html and
// <name>.txt (both optional, at least one required). The subject is taken
// from the first non-empty <title> tag in the HTML, or a "Subject:" header
// in the .txt file, or the explicit subject argument.
func Render(name, subject string, data any, to []string) (Message, error) {
	msg := Message{To: to, Subject: subject}

	if html, err := renderHTML(name, data); err == nil {
		msg.HTML = html
		if subject == "" {
			msg.Subject = extractTitle(html)
		}
	} else if !isMissingTemplate(err) {
		return msg, err
	}

	if text, err := renderText(name, data); err == nil {
		subj, body := splitTextSubject(text)
		msg.Text = body
		if msg.Subject == "" {
			msg.Subject = subj
		}
	} else if !isMissingTemplate(err) {
		return msg, err
	}

	if msg.HTML == "" && msg.Text == "" {
		return msg, fmt.Errorf("mailer: no template found for %q", name)
	}
	if msg.Subject == "" {
		msg.Subject = name
	}
	return msg, nil
}

func renderHTML(name string, data any) (string, error) {
	raw, err := templateFS.ReadFile("templates/" + name + ".html")
	if err != nil {
		return "", err
	}
	t, err := template.New(name).Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderText(name string, data any) (string, error) {
	raw, err := templateFS.ReadFile("templates/" + name + ".txt")
	if err != nil {
		return "", err
	}
	t, err := textTemplate.New(name).Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func isMissingTemplate(err error) bool {
	return err != nil && strings.Contains(err.Error(), "file does not exist")
}

func extractTitle(html string) string {
	lower := strings.ToLower(html)
	i := strings.Index(lower, "<title>")
	if i < 0 {
		return ""
	}
	j := strings.Index(lower[i:], "</title>")
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(html[i+len("<title>") : i+j])
}

func splitTextSubject(text string) (subject, body string) {
	text = strings.TrimLeft(text, "\r\n")
	if strings.HasPrefix(strings.ToLower(text), "subject:") {
		nl := strings.IndexByte(text, '\n')
		if nl > 0 {
			subject = strings.TrimSpace(text[len("Subject:"):nl])
			body = strings.TrimLeft(text[nl+1:], "\r\n")
			return
		}
	}
	return "", text
}
