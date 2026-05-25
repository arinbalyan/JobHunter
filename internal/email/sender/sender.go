package sender

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SMTPConfig holds Gmail SMTP configuration.
type SMTPConfig struct {
	User     string
	Password string
	FromName string
	FromAddr string
}

// EmailMessage represents a complete email to send.
type EmailMessage struct {
	To           string
	Subject      string
	HTMLBody     string
	PlainBody    string
	TrackingID   string
	MessageID    string
}

// Sender handles sending emails via Gmail SMTP.
type Sender struct {
	config    SMTPConfig
	auth      smtp.Auth
	host      string
	port      string
}

// New creates a new Gmail SMTP sender.
func New(cfg SMTPConfig) *Sender {
	return &Sender{
		config: cfg,
		auth:   smtp.PlainAuth("", cfg.User, cfg.Password, "smtp.gmail.com"),
		host:   "smtp.gmail.com",
		port:   "587",
	}
}

// Send sends an email. Returns the tracking ID and message ID.
func (s *Sender) Send(ctx context.Context, msg *EmailMessage) error {
	// Generate tracking ID if not provided
	if msg.TrackingID == "" {
		msg.TrackingID = uuid.New().String()
	}

	// Generate Message-ID for bounce/reply tracking
	if msg.MessageID == "" {
		msg.MessageID = fmt.Sprintf("<%s@jobhunter>", uuid.New().String())
	}

	// Build the email
	emailBody, err := s.buildEmail(msg)
	if err != nil {
		return fmt.Errorf("build email: %w", err)
	}

	addr := fmt.Sprintf("%s:%s", s.host, s.port)

	// Send via SMTP
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := smtp.SendMail(addr, s.auth, s.config.FromAddr, []string{msg.To}, emailBody); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	return nil
}

// buildEmail constructs the raw MIME email.
func (s *Sender) buildEmail(msg *EmailMessage) ([]byte, error) {
	var buf bytes.Buffer

	// Headers
	from := fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromAddr)
	if s.config.FromName == "" {
		from = s.config.FromAddr
	}

	headers := map[string]string{
		"From":         from,
		"To":           msg.To,
		"Subject":      msg.Subject,
		"MIME-Version": "1.0",
		"Content-Type": `multipart/alternative; boundary="boundary-jobhunter"`,
		"Message-ID":   msg.MessageID,
		"Date":         time.Now().UTC().Format(time.RFC1123Z),
	}

	for k, v := range headers {
		fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
	}
	buf.WriteString("\r\n")

	// Plain text part
	buf.WriteString("--boundary-jobhunter\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(msg.PlainBody)
	buf.WriteString("\r\n")

	// HTML part with tracking pixel
	buf.WriteString("--boundary-jobhunter\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(msg.HTMLBody)
	buf.WriteString("\r\n")

	buf.WriteString("--boundary-jobhunter--\r\n")

	return buf.Bytes(), nil
}

// InjectTrackingPixel adds a tracking pixel to an HTML email body.
// trackingURL should be the full URL to the tracking server's /track endpoint.
func InjectTrackingPixel(htmlBody string, trackingServerURL, trackingID string) string {
	pixelURL := fmt.Sprintf("%s/track?id=%s", strings.TrimRight(trackingServerURL, "/"), trackingID)
	pixel := fmt.Sprintf(
		`<img src="%s" width="1" height="1" alt="" style="display:none;" />`,
		pixelURL,
	)

	// Insert before closing </body> or append
	if strings.Contains(htmlBody, "</body>") {
		return strings.Replace(htmlBody, "</body>", pixel+"</body>", 1)
	}
	return htmlBody + pixel
}

// RenderTemplate renders an HTML template with the given data.
func RenderTemplate(tmpl *template.Template, name string, data interface{}) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("render template %s: %w", name, err)
	}
	return buf.String(), nil
}
