// Command: bouncescan
// Reads mailer-daemon bounce emails from Gmail IMAP, matches them to our
// sent email records in the database, and updates bounce status.
//
// Run manually:
//   go run ./cmd/bouncescan/
//
// Or via GitHub Actions (daily schedule).
//
// Environment variables needed:
//   DATABASE_URL, GMAIL_USER, GMAIL_APP_PASS
//   TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID (optional, for notifications)
//   BOUNCE_SCAN_DAYS (optional, default 2)
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/logging"
	"github.com/arinbalyan/jobhunter/internal/migrations"
	"github.com/arinbalyan/jobhunter/internal/telegram"
)

var tagCounter atomic.Int64

func nextTag() string {
	n := tagCounter.Add(1)
	return fmt.Sprintf("a%03d", n)
}

func main() {
	if val := os.Getenv("GOMEMLIMIT"); val == "" {
		debug.SetMemoryLimit(80 * 1024 * 1024)
	}

	startTime := time.Now()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	logger := logging.New(cfg.LogLevel, os.Stdout)
	logger.Info("Bounce scan starting...")

	if cfg.IMAPUser == "" || cfg.IMAPPass == "" {
		log.Fatalf("IMAP_USER and IMAP_PASS are required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	dbPool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed: %v", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	if err := migrations.Run(cfg.DatabaseURL); err != nil {
		logger.Error("migrations failed: %v", err)
		os.Exit(1)
	}

	// Scan window — default 48 hours for daily runs
	scanDays := 2
	if d := os.Getenv("BOUNCE_SCAN_DAYS"); d != "" {
		if parsed, err := fmt.Sscanf(d, "%d", &scanDays); err != nil || parsed != 1 {
			scanDays = 2
		}
	}
	sinceDate := time.Now().AddDate(0, 0, -scanDays).UTC().Format("02-Jan-2006")
	logger.Info("scanning bounces since %s (%d days)", sinceDate, scanDays)

	// ── IMAP Connection ──
	addr := net.JoinHostPort(cfg.IMAPHost, fmt.Sprintf("%d", cfg.IMAPPort))
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: cfg.IMAPHost})
	if err != nil {
		logger.Error("IMAP connection failed: %v", err)
		os.Exit(1)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	reader := bufio.NewReaderSize(conn, 4096)

	// Login
	resp := imapCommand(conn, reader, fmt.Sprintf(`%s LOGIN %s "%s"`, nextTag(), cfg.IMAPUser, cfg.IMAPPass))
	if strings.Contains(resp, "NO") || strings.Contains(resp, "BAD") {
		logger.Error("IMAP login failed")
		os.Exit(1)
	}
	logger.Info("IMAP login successful")

	// Select INBOX
	imapCommand(conn, reader, fmt.Sprintf("%s SELECT INBOX", nextTag()))

	// ── Multi-strategy bounce search ──
	// Gmail bounces come from various senders and formats.
	// Strategy 1: Search FROM for common bounce senders
	// Strategy 2: Search SUBJECT for bounce keywords
	// Strategy 3: Search the full message for our domain patterns if needed

	bounceSenders := []string{
		"mailer-daemon",
		"MAILER-DAEMON",
		"Mail Delivery System",
		"postmaster",
		"mailer-daemon@googlemail.com",
		"MAILER-DAEMON@",
	}

	bounceSubjects := []string{
		"Delivery Status Notification",
		"Undelivered Mail Returned",
		"Returned mail",
		"failure notice",
		"Delivery Failure",
		"Delivery failed",
		"Undeliverable",
		"Message not delivered",
		"Mail delivery failed",
		"Delivery Report",
		"status=bounced",
	}

	// Search FROM patterns
	var allMsgIDs []string
	for _, sender := range bounceSenders {
		resp = imapCommand(conn, reader, fmt.Sprintf("%s SEARCH FROM \"%s\" SINCE %s", nextTag(), sender, sinceDate))
		ids := parseSearchIDs(resp)
		allMsgIDs = append(allMsgIDs, ids...)
		if len(ids) > 0 {
			logger.Debug("  FROM \"%s\": found %d", sender, len(ids))
		}
	}

	// Search SUBJECT patterns
	for _, subj := range bounceSubjects {
		resp = imapCommand(conn, reader, fmt.Sprintf("%s SEARCH SUBJECT \"%s\" SINCE %s", nextTag(), subj, sinceDate))
		ids := parseSearchIDs(resp)
		allMsgIDs = append(allMsgIDs, ids...)
		if len(ids) > 0 {
			logger.Debug("  SUBJECT \"%s\": found %d", subj, len(ids))
		}
	}

	// Also search for OR combination: FROM postmaster OR FROM mailer-daemon
	// (already covered above individually)

	// Remove duplicates
	seen := make(map[string]bool)
	var bounceIDs []string
	for _, id := range allMsgIDs {
		if !seen[id] {
			seen[id] = true
			bounceIDs = append(bounceIDs, id)
		}
	}

	logger.Info("unique potential bounce messages: %d", len(bounceIDs))

	var totalBounces int
	var matchedBounces int
	var newBounces int

	for _, msgID := range bounceIDs {
		// Fetch subject + from to determine if this is actually a bounce
		resp = imapCommand(conn, reader, fmt.Sprintf("%s FETCH %s (BODY.PEEK[HEADER.FIELDS (SUBJECT FROM DATE)])", nextTag(), msgID))
		subject := extractHeader(resp, "SUBJECT")
		from := extractHeader(resp, "FROM")
		bounceDate := extractDate(resp)

		// Only process if it looks like a bounce notification
		if !isBounceMessage(subject, from) {
			logger.Debug("skipping non-bounce message: subject=%q from=%q", subject, from)
			continue
		}

		// Fetch the full body to extract the original recipient
		resp = imapCommand(conn, reader, fmt.Sprintf("%s FETCH %s BODY[]", nextTag(), msgID))

		totalBounces++

		// Try multiple strategies to extract the bounced email
		bouncedEmail := extractBouncedRecipient(resp)
		if bouncedEmail == "" {
			bouncedEmail = extractBouncedRecipientFromHeaders(resp)
		}
		if bouncedEmail == "" {
			bouncedEmail = extractEmailFromBounceText(resp)
		}

		if bouncedEmail == "" {
			logger.Debug("could not extract bounced recipient from msg %s (subj=%q from=%q)", msgID, subject, from)
			continue
		}

		matchedBounces++

		// Try to match in EMAILS table first (exact recipient match)
		emails, err := dbPool.GetEmailsByRecipient(ctx, bouncedEmail)
		if err != nil || len(emails) == 0 {
			logger.Debug("no email records for %s: %v — trying queue fallback", bouncedEmail, err)

			// FALLBACK: Try matching against email_queue by recipient email
			queueItem, qErr := dbPool.GetQueueByRecipient(ctx, bouncedEmail)
			if qErr != nil {
				logger.Debug("no queue item for %s either: %v", bouncedEmail, qErr)
				continue
			}

			// Mark the queue item as bounced
			bounceType := extractBounceType(resp)
			errMsg := fmt.Sprintf("bounced: %s", bounceType)
			_ = dbPool.UpdateQueueStatus(ctx, queueItem.ID, "bounced", errMsg)
			newBounces++
			logger.Info("marked queue bounced: %s — %s (from=%s, date=%s)", bouncedEmail, bounceType, from, bounceDate)
			continue
		}

		// Mark ALL matching email records as bounced
		bounceType := extractBounceType(resp)
		for _, email := range emails {
			if email.Bounced {
				logger.Debug("email %d to %s already marked as bounced", email.ID, bouncedEmail)
				continue
			}

			if err := dbPool.MarkBounced(ctx, email.MessageID, bounceType); err != nil {
				logger.Error("failed to mark bounce for email %d: %v", email.ID, err)
				continue
			}

			// Also update the email_queue item for this job
			if email.JobID != nil && *email.JobID > 0 {
				errMsg := fmt.Sprintf("bounced: %s", bounceType)
				_ = dbPool.UpdateQueueStatusByJobID(ctx, *email.JobID, "bounced", errMsg)
			}

			newBounces++
			logger.Info("marked bounced email #%d: %s — %s (from=%s, date=%s, queue updated)", email.ID, bouncedEmail, bounceType, from, bounceDate)
		}
	}

	// ── Reply detection ──
	resp = imapCommand(conn, reader, fmt.Sprintf("%s SEARCH SUBJECT \"Re:\" SINCE %s", nextTag(), sinceDate))
	replyIDs := parseSearchIDs(resp)

	var matchedReplies int
	for _, msgID := range replyIDs {
		resp = imapCommand(conn, reader, fmt.Sprintf("%s FETCH %s (BODY[HEADER.FIELDS (SUBJECT FROM IN-REPLY-TO MESSAGE-ID)])", nextTag(), msgID))
		from := extractHeader(resp, "FROM")
		inReplyTo := extractHeader(resp, "IN-REPLY-TO")
		replySubject := extractHeader(resp, "SUBJECT")

		// Skip auto-replies like "Out of Office" or "Auto-Reply"
		if isAutoReply(replySubject) {
			continue
		}

		if inReplyTo != "" {
			if err := dbPool.MarkReplied(ctx, inReplyTo); err == nil {
				matchedReplies++
			}
		} else if from != "" {
			replyEmail := extractEmailFromHeader(from)
			if replyEmail != "" {
				emails, err := dbPool.GetEmailsByRecipient(ctx, replyEmail)
				if err == nil {
					for _, e := range emails {
						if !e.Replied {
							dbPool.MarkReplied(ctx, e.MessageID)
							matchedReplies++
						}
					}
				}
			}
		}
	}

	duration := time.Since(startTime)

	// ── Logout ──
	imapCommand(conn, reader, fmt.Sprintf("%s LOGOUT", nextTag()))

	logger.Info(
		"bounce scan complete: %d total bounces, %d matched, %d new, %d replies in %.0fs",
		totalBounces, matchedBounces, newBounces, matchedReplies, duration.Seconds(),
	)

	recordRun(ctx, dbPool, "bouncescan", "completed", totalBounces, matchedBounces, newBounces, matchedReplies, int(duration.Milliseconds()), "")

	// ── Telegram report ──
	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		var sentToday, bouncedToday int
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE sent_at > CURRENT_DATE").Scan(&sentToday)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE bounced=true AND bounced_at > CURRENT_DATE").Scan(&bouncedToday)

		var totalSent, totalBouncedAll, totalOpened, totalClicked, totalReplied int
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE status='sent'").Scan(&totalSent)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE bounced=true").Scan(&totalBouncedAll)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE opened=true").Scan(&totalOpened)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE clicked=true").Scan(&totalClicked)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE replied=true").Scan(&totalReplied)

		msg := fmt.Sprintf(
			"<b>📬 Bounce Scan Complete</b>\n\n"+
				"── This Scan ──\n"+
				"Bounces found: %d\n"+
				"Newly marked: %d\n"+
				"Replies detected: %d\n"+
				"Duration: %.0fs\n\n"+
				"── All-Time Stats ──\n"+
				"Total sent: %d\n"+
				"Opened: %d\n"+
				"Clicked: %d\n"+
				"Bounced: %d\n"+
				"Replied: %d\n"+
				"Deliverability: %.1f%%\n\n"+
				"── Today ──\n"+
				"Sent: %d\n"+
				"Bounced: %d",
			totalBounces, newBounces, matchedReplies, duration.Seconds(),
			totalSent, totalOpened, totalClicked, totalBouncedAll, totalReplied,
			func() float64 {
				if totalSent > 0 {
					return float64(totalSent-totalBouncedAll) / float64(totalSent) * 100
				}
				return 0
			}(),
			sentToday, bouncedToday,
		)
		_ = telegram.SendMessage(ctx, cfg.TelegramBotToken, cfg.TelegramChatID, msg)
	}
}

// ── IMAP Helpers ──

// imapCommand sends a command and reads the response until the tagged response.
func imapCommand(conn net.Conn, reader *bufio.Reader, cmd string) string {
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	if _, err := fmt.Fprintf(conn, "%s\r\n", cmd); err != nil {
		return ""
	}

	// Extract tag prefix (e.g., "a0001")
	tag := strings.Fields(cmd)[0]

	var resp strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		resp.WriteString(line)
		// Tagged response ends with "tag OK", "tag BAD", or "tag NO"
		if strings.HasPrefix(line, tag) {
			if strings.Contains(line, "OK") || strings.Contains(line, "BAD") || strings.Contains(line, "NO") {
				break
			}
		}
	}
	conn.SetDeadline(time.Time{})
	return resp.String()
}

// parseSearchIDs extracts numeric message IDs from an IMAP SEARCH response.
func parseSearchIDs(resp string) []string {
	var ids []string
	lines := strings.Split(resp, "\r\n")
	for _, line := range lines {
		if strings.Contains(line, "SEARCH") && !strings.HasPrefix(line, "a") {
			parts := strings.Fields(line)
			for _, p := range parts {
				if p == "SEARCH" || p == "*" {
					continue
				}
				isNum := true
				for _, c := range p {
					if c < '0' || c > '9' {
						isNum = false
						break
					}
				}
				if isNum {
					ids = append(ids, p)
				}
			}
		}
	}
	return ids
}

// isBounceMessage checks if a subject+from combo looks like a bounce notification.
func isBounceMessage(subject, from string) bool {
	subjLower := strings.ToLower(subject)
	fromLower := strings.ToLower(from)

	// Check FROM for known bounce senders
	bounceSenders := []string{
		"mailer-daemon", "postmaster", "mail delivery system",
	}
	for _, s := range bounceSenders {
		if strings.Contains(fromLower, s) {
			return true
		}
	}

	// Check subject for bounce keywords
	bounceSubjKeywords := []string{
		"delivery status", "undelivered", "returned mail", "failure notice",
		"delivery failure", "delivery failed", "undeliverable",
		"message not delivered", "mail delivery failed", "non-delivery",
		"status=bounced", "bounced mail", "return receipt",
	}
	for _, k := range bounceSubjKeywords {
		if strings.Contains(subjLower, k) {
			return true
		}
	}

	return false
}

// isAutoReply checks if a subject indicates an auto-reply (not a real reply).
func isAutoReply(subject string) bool {
	subjLower := strings.ToLower(subject)
	autoPatterns := []string{
		"out of office", "automatic reply", "auto-reply", "auto reply",
		"away from", "vacation", "on leave", "i am not available",
		"thank you for your email", "acknowledgement of receipt",
	}
	for _, pattern := range autoPatterns {
		if strings.Contains(subjLower, pattern) {
			return true
		}
	}
	return false
}

// extractBouncedRecipient extracts the original recipient email from a bounce
// notification body using standard DSN (Delivery Status Notification) fields.
func extractBouncedRecipient(resp string) string {
	// Strategy 1: Look for RFC 1894 DSN fields
	dsnFields := []string{
		"Original-Recipient:",
		"Final-Recipient:",
		"X-Failed-Recipients:",
	}
	for _, field := range dsnFields {
		idx := strings.Index(resp, field)
		if idx < 0 {
			continue
		}
		line := resp[idx:]
		end := strings.IndexAny(line, "\r\n")
		if end > 0 {
			email := line[:end]
			email = strings.TrimSpace(email)
			email = strings.TrimPrefix(email, field)
			email = strings.TrimSpace(email)
			// DSN format: "rfc822; email@domain.com" or "email@domain.com"
			if semicolon := strings.LastIndex(email, ";"); semicolon >= 0 {
				email = strings.TrimSpace(email[semicolon+1:])
			}
			email = strings.Trim(email, "<> \t")
			if strings.Contains(email, "@") && strings.Contains(email, ".") {
				return email
			}
		}
	}

	return ""
}

// extractBouncedRecipientFromHeaders looks for the original recipient in
// embedded message headers within the bounce body.
func extractBouncedRecipientFromHeaders(resp string) string {
	// Strategy 2: Find original message headers inside the bounce
	// Look for "To:" header inside the bounced message (after "Content-Type: message/rfc822")
	parts := strings.Split(resp, "Content-Type: message/rfc822")
	if len(parts) > 1 {
		for i := 1; i < len(parts); i++ {
			headerBlock := parts[i]
			// Find the "To:" line
			lines := strings.Split(headerBlock, "\r\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(strings.ToLower(trimmed), "to:") {
					toVal := strings.TrimSpace(trimmed[3:])
					// Extract email from "Name <email>" or just "email"
					if idx := strings.Index(toVal, "<"); idx >= 0 {
						end := strings.Index(toVal, ">")
						if end > idx {
							email := toVal[idx+1 : end]
							if strings.Contains(email, "@") {
								return email
							}
						}
					}
					// Also check for bare email in the To field
					fields := strings.Fields(toVal)
					for _, f := range fields {
						f = strings.Trim(f, "<> \t")
						if strings.Count(f, "@") == 1 && strings.Contains(f, ".") {
							return f
						}
					}
				}
			}
		}
	}
	return ""
}

// extractEmailFromBounceText is a last-resort: find any email in the body
// that looks like it was the intended recipient.
func extractEmailFromBounceText(resp string) string {
	// Strategy 3: Find any email address in the text that matches
	// known patterns for "could not deliver to" or "message for"
	indicators := []string{
		"could not deliver", "message for", "delivery to",
		"failed to deliver", "unable to deliver", "delivery of your message",
		"was not delivered", "is not delivered", "to the following",
		"<", ">",
	}

	lines := strings.Split(resp, "\r\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		hasIndicator := false
		for _, ind := range indicators {
			if strings.Contains(lower, ind) {
				hasIndicator = true
				break
			}
		}
		if !hasIndicator {
			continue
		}

		// Extract email from the line
		re := regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.]+`)
		matches := re.FindAllString(line, -1)
		for _, m := range matches {
			// Filter out common false positives
			if strings.Contains(m, "google.com") || strings.Contains(m, "gmail.com") ||
				strings.Contains(m, "example.com") || strings.Contains(m, "domain.com") {
				continue
			}
			return strings.ToLower(m)
		}
	}

	return ""
}

// extractBounceType determines the type of bounce from the message.
func extractBounceType(resp string) string {
	respLower := strings.ToLower(resp)
	switch {
	case strings.Contains(respLower, "5.1.1") || strings.Contains(respLower, "does not exist") || strings.Contains(respLower, "no such user") || strings.Contains(respLower, "address rejected") || strings.Contains(respLower, "invalid address"):
		return "invalid_address"
	case strings.Contains(respLower, "5.2.2") || strings.Contains(respLower, "mailbox full") || strings.Contains(respLower, "quota") || strings.Contains(respLower, "over quota"):
		return "mailbox_full"
	case strings.Contains(respLower, "4.2.2") || strings.Contains(respLower, "rate limit") || strings.Contains(respLower, "too many") || strings.Contains(respLower, "temporarily"):
		return "rate_limited"
	case strings.Contains(respLower, "5.7.1") || strings.Contains(respLower, "rejected") || strings.Contains(respLower, "spam") || strings.Contains(respLower, "policy"):
		return "rejected"
	case strings.Contains(respLower, "5.4.1") || strings.Contains(respLower, "no mx") || strings.Contains(respLower, "domain") || strings.Contains(respLower, "dns"):
		return "domain_invalid"
	case strings.Contains(respLower, "5.4.4") || strings.Contains(respLower, "unrouteable") || strings.Contains(respLower, "cannot find"):
		return "domain_invalid"
	default:
		return "unknown"
	}
}

// extractDate extracts a date from an IMAP FETCH response INTERNALDATE.
func extractDate(resp string) string {
	if idx := strings.Index(resp, "INTERNALDATE"); idx >= 0 {
		rest := resp[idx+12:]
		end := strings.Index(rest, ")")
		if end > 0 {
			return strings.TrimSpace(rest[:end])
		}
	}
	return ""
}

// extractHeader extracts a header value from an IMAP FETCH response.
func extractHeader(resp, header string) string {
	headerKey := strings.ToUpper(header)
	lines := strings.Split(resp, "\r\n")
	for _, line := range lines {
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, headerKey+":") {
			return strings.TrimSpace(line[len(headerKey)+1:])
		}
	}
	return ""
}

// extractEmailFromHeader extracts an email address from a "Name <email>" format.
func extractEmailFromHeader(header string) string {
	if idx := strings.Index(header, "<"); idx >= 0 {
		end := strings.Index(header, ">")
		if end > idx {
			return header[idx+1 : end]
		}
	}
	return strings.TrimSpace(header)
}

func recordRun(ctx context.Context, pool *db.Pool, workflow, status string, totalBounces, matchedBounces, newBounces, replies, durationMs int, errMsg string) {
	_ = pool.RecordRunLog(ctx, workflow, status, totalBounces, matchedBounces, newBounces, replies, 0, durationMs, errMsg)
}
