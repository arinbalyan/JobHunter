package sender

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SMTPConfig holds Gmail SMTP configuration.
type SMTPConfig struct {
	User       string
	Password   string
	FromName   string
	FromAddr   string
	ResumePath string // optional path to PDF resume
}

// EmailMessage represents a complete email to send.
type EmailMessage struct {
	To         string
	Subject    string
	HTMLBody   string
	PlainBody  string
	TrackingID string
	MessageID  string
}

// Sender handles sending emails via Gmail SMTP with retry + optional resume.
type Sender struct {
	config    SMTPConfig
	auth      smtp.Auth
	host      string
	port      string
}

const maxRetries = 3
const retryDelay = 5 * time.Second

// New creates a new Gmail SMTP sender.
// Uses smtp.PlainAuth (SASL PLAIN mechanism). The password is sent base64-encoded,
// but smtp.SendMail always negotiates STARTTLS on port 587, so the entire session
// is encrypted. This is the standard Gmail SMTP approach — not plaintext.
func New(cfg SMTPConfig) *Sender {
	return &Sender{
		config: cfg,
		auth:   smtp.PlainAuth("", cfg.User, cfg.Password, "smtp.gmail.com"),
		host:   "smtp.gmail.com",
		port:   "587",
	}
}

// Send sends an email with retry logic and optional resume attachment.
func (s *Sender) Send(ctx context.Context, msg *EmailMessage) error {
	if msg.TrackingID == "" {
		msg.TrackingID = uuid.New().String()
	}
	if msg.MessageID == "" {
		msg.MessageID = fmt.Sprintf("<%s@jobhunter>", uuid.New().String())
	}

	emailBody, err := s.buildEmail(msg)
	if err != nil {
		return fmt.Errorf("build email: %w", err)
	}

	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	to := []string{msg.To}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := smtp.SendMail(addr, s.auth, s.config.FromAddr, to, emailBody)
		if err == nil {
			return nil
		}

		lastErr = err
		if isQuotaError(err) {
			return fmt.Errorf("gmail quota exceeded: %w", err)
		}

		if attempt < maxRetries {
			time.Sleep(retryDelay)
		}
	}

	return fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}

// buildEmail constructs the raw MIME email with optional PDF attachment.
func (s *Sender) buildEmail(msg *EmailMessage) ([]byte, error) {
	// Check if we have a resume to attach
	resumePath := s.findResume()
	hasResume := resumePath != ""

	var buf bytes.Buffer

	// Choose content type
	contentType := `multipart/alternative; boundary="boundary-jobhunter"`
	boundary := "boundary-jobhunter"
	closeBoundary := "--boundary-jobhunter--"

	if hasResume {
		// Mixed content: multipart/mixed wrapping multipart/alternative + attachment
		boundary = "boundary-mixed"
		closeBoundary = "--boundary-mixed--"
		contentType = fmt.Sprintf(`multipart/mixed; boundary="%s"`, boundary)
	}

	from := s.config.FromAddr
	if s.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromAddr)
	}

	headers := map[string]string{
		"From":         from,
		"To":           msg.To,
		"Subject":      msg.Subject,
		"MIME-Version": "1.0",
		"Content-Type": contentType,
		"Message-ID":   msg.MessageID,
		"Date":         time.Now().UTC().Format(time.RFC1123Z),
	}
	for k, v := range headers {
		fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
	}
	buf.WriteString("\r\n")

	if hasResume {
		// Start the multipart/alternative wrapper inside mixed
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString(`Content-Type: multipart/alternative; boundary="boundary-jobhunter"` + "\r\n\r\n")
	}

	// Plain text part
	buf.WriteString("--boundary-jobhunter\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	buf.WriteString(msg.PlainBody)
	buf.WriteString("\r\n")

	// HTML part
	buf.WriteString("--boundary-jobhunter\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	buf.WriteString(msg.HTMLBody)
	buf.WriteString("\r\n")

	buf.WriteString("--boundary-jobhunter--\r\n")

	// Attach resume PDF if found
	if hasResume {
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString(fmt.Sprintf(`Content-Type: application/pdf; name="%s"`+"\r\n", filepath.Base(resumePath)))
		buf.WriteString("Content-Transfer-Encoding: base64\r\n")
		buf.WriteString(fmt.Sprintf(`Content-Disposition: attachment; filename="%s"`+"\r\n\r\n", filepath.Base(resumePath)))

		data, err := os.ReadFile(resumePath)
		if err == nil {
			// Use stdlib base64 with RFC 2045 line wrapping (76 chars per line)
			encoded := base64.StdEncoding.EncodeToString(data)
			for len(encoded) > 0 {
				chunk := encoded
				if len(chunk) > 76 {
					chunk = chunk[:76]
				}
				buf.WriteString(chunk)
				buf.WriteString("\r\n")
				encoded = encoded[len(chunk):]
			}
		}

		buf.WriteString(closeBoundary + "\r\n")
	}

	return buf.Bytes(), nil
}

// findResume looks for a PDF in .agent-data/ or the configured path.
func (s *Sender) findResume() string {
	// Check configured path first
	if s.config.ResumePath != "" {
		if _, err := os.Stat(s.config.ResumePath); err == nil {
			return s.config.ResumePath
		}
	}

	// Scan .agent-data/ for first PDF
	entries, err := os.ReadDir(".agent-data")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".pdf") {
			return filepath.Join(".agent-data", e.Name())
		}
	}
	return ""
}

func isQuotaError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "quota") ||
		strings.Contains(msg, "daily limit") ||
		strings.Contains(msg, "too many messages") ||
		strings.Contains(msg, "exceeded")
}

// InjectTrackingPixel adds a tracking pixel to an HTML email body.
func InjectTrackingPixel(htmlBody string, trackingServerURL, trackingID string) string {
	pixelURL := fmt.Sprintf("%s/track?id=%s", strings.TrimRight(trackingServerURL, "/"), trackingID)
	pixel := fmt.Sprintf(`<img src="%s" width="1" height="1" alt="" style="display:none;" />`, pixelURL)
	if strings.Contains(htmlBody, "</body>") {
		return strings.Replace(htmlBody, "</body>", pixel+"</body>", 1)
	}
	return htmlBody + pixel
}

