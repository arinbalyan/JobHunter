// Command: bouncescan
// Reads mailer-daemon bounce emails from Gmail IMAP, matches them to our
// sent email records in the database, and updates bounce status.
//
// Run manually:
//   go run ./cmd/bouncescan/
//
// Or via GitHub Actions (weekly schedule).
//
// Environment variables needed:
//   DATABASE_URL, GMAIL_USER, GMAIL_APP_PASS
//   TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID (optional, for notifications)
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
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/logging"
	"github.com/arinbalyan/jobhunter/internal/migrations"
	"github.com/arinbalyan/jobhunter/internal/telegram"
)

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

	// Login
	reader := bufio.NewReaderSize(conn, 4096)

	resp := imapCommand(conn, reader, fmt.Sprintf("a001 LOGIN %s %s", cfg.IMAPUser, cfg.IMAPPass))
	if strings.Contains(resp, "NO") || strings.Contains(resp, "BAD") {
		logger.Error("IMAP login failed")
		os.Exit(1)
	}
	logger.Info("IMAP login successful")

	// Select INBOX
	imapCommand(conn, reader, "a002 SELECT INBOX")

	// Search for mailer-daemon bounces
	resp = imapCommand(conn, reader, fmt.Sprintf(`a003 SEARCH FROM "mailer-daemon" SINCE %s`, sinceDate))
	bounceIDs := parseSearchIDs(resp)
	logger.Info("found %d mailer-daemon bounces (since %s)", len(bounceIDs), sinceDate)

	// Also check "MAILER-DAEMON" (some systems use uppercase)
	resp = imapCommand(conn, reader, fmt.Sprintf(`a004 SEARCH FROM "MAILER-DAEMON" SINCE %s`, sinceDate))
	moreIDs := parseSearchIDs(resp)
	bounceIDs = append(bounceIDs, moreIDs...)

	// Remove duplicates
	seen := make(map[string]bool)
	var uniqueIDs []string
	for _, id := range bounceIDs {
		if !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}
	bounceIDs = uniqueIDs

	logger.Info("unique bounces: %d", len(bounceIDs))

	var totalBounces int
	var matchedBounces int
	var newBounces int

	for _, msgID := range bounceIDs {
		// Fetch the email body to find the original recipient
		resp = imapCommand(conn, reader, fmt.Sprintf("a005 FETCH %s BODY[]", msgID))
		bouncedEmail := extractBouncedRecipient(resp)

		// Fetch headers for date
		resp = imapCommand(conn, reader, fmt.Sprintf("a006 FETCH %s (INTERNALDATE BODY[HEADER.FIELDS (SUBJECT FROM DATE)])", msgID))
		bounceDate := extractDate(resp)

		totalBounces++

		if bouncedEmail == "" {
			logger.Debug("could not extract bounced recipient from message %s", msgID)
			continue
		}

		matchedBounces++

		// Look up this recipient in our emails table
		email, err := dbPool.GetEmailByRecipient(ctx, bouncedEmail)
		if err != nil {
			logger.Debug("no matching email found for %s: %v", bouncedEmail, err)
			continue
		}

		// Check if already marked as bounced
		if email.Bounced {
			logger.Debug("email to %s already marked as bounced", bouncedEmail)
			continue
		}

		// Mark as bounced
		bounceType := extractBounceType(resp)
		if err := dbPool.MarkBounced(ctx, email.MessageID, bounceType); err != nil {
			logger.Error("failed to mark bounce for %s: %v", bouncedEmail, err)
			continue
		}

		// Also update the email_queue item to reflect the bounce
		if email.JobID != nil && *email.JobID > 0 {
			errMsg := fmt.Sprintf("bounced: %s", bounceType)
			_ = dbPool.UpdateQueueStatusByJobID(ctx, *email.JobID, "bounced", errMsg)
		}

		newBounces++
		logger.Info("marked bounced: %s → %s — %s (date: %s, queue updated)", bouncedEmail, email.RecipientEmail, bounceType, bounceDate)
	}

	// Search for replies (Re: subject lines)
	resp = imapCommand(conn, reader, fmt.Sprintf("a007 SEARCH SUBJECT \"Re:\" SINCE %s", sinceDate))
	replyIDs := parseSearchIDs(resp)

	var matchedReplies int
	for _, msgID := range replyIDs {
		resp = imapCommand(conn, reader, fmt.Sprintf("a008 FETCH %s (BODY[HEADER.FIELDS (SUBJECT FROM IN-REPLY-TO MESSAGE-ID)])", msgID))
		from := extractHeader(resp, "FROM")
		inReplyTo := extractHeader(resp, "IN-REPLY-TO")

		if inReplyTo != "" {
			if err := dbPool.MarkReplied(ctx, inReplyTo); err == nil {
				matchedReplies++
			}
		} else if from != "" {
			// Match by sender email
			replyEmail := extractEmailFromHeader(from)
			if replyEmail != "" {
				// Mark all sent emails to this person as replied
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
	imapCommand(conn, reader, "a009 LOGOUT")

	logger.Info(
		"bounce scan complete: %d total bounces, %d matched, %d new, %d replies in %.0fs",
		totalBounces, matchedBounces, newBounces, matchedReplies, duration.Seconds(),
	)

	recordRun(ctx, dbPool, "bouncescan", "completed", totalBounces, matchedBounces, newBounces, matchedReplies, int(duration.Milliseconds()), "")

	// ── Enhanced Telegram report ──
	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		// Get today's stats for the report
		var sentToday, bouncedToday int
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE sent_at > CURRENT_DATE").Scan(&sentToday)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE bounced=true AND bounced_at > CURRENT_DATE").Scan(&bouncedToday)

		// All-time stats
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

// imapCommand sends a command and reads the response until completion.
func imapCommand(conn net.Conn, reader *bufio.Reader, cmd string) string {
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	if _, err := fmt.Fprintf(conn, "%s\r\n", cmd); err != nil {
		return ""
	}
	var resp strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		resp.WriteString(line)
		if strings.HasPrefix(line, "a001") || strings.HasPrefix(line, "a002") ||
			strings.HasPrefix(line, "a003") || strings.HasPrefix(line, "a004") ||
			strings.HasPrefix(line, "a005") || strings.HasPrefix(line, "a006") ||
			strings.HasPrefix(line, "a007") || strings.HasPrefix(line, "a008") ||
			strings.HasPrefix(line, "a009") {
			if strings.Contains(line, "OK") || strings.Contains(line, "BAD") || strings.Contains(line, "NO") {
				break
			}
		}
		if strings.Contains(line, ")") && strings.Contains(line, "FETCH") {
			continue
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
		if strings.Contains(line, "SEARCH") && !strings.HasPrefix(line, "a0") {
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

// extractBouncedRecipient tries to find the original recipient email in a
// mailer-daemon bounce message body.
func extractBouncedRecipient(resp string) string {
	// Look for common bounce patterns
	patterns := []string{
		"Original-Recipient:",
		"Final-Recipient:",
		"<",
	}
	for _, pattern := range patterns {
		idx := strings.Index(resp, pattern)
		if idx < 0 {
			continue
		}
		line := resp[idx:]
		end := strings.IndexAny(line, "\r\n>")
		if end > 0 {
			email := line[:end]
			email = strings.TrimSpace(email)
			email = strings.TrimPrefix(email, pattern)
			email = strings.TrimSpace(email)
			email = strings.Trim(email, "<>")
			if strings.Contains(email, "@") {
				return email
			}
		}
	}

	// Try to find any email address in the body
	lines := strings.Split(resp, "\r\n")
	for _, line := range lines {
		if strings.Contains(line, "@") && strings.Contains(line, ".") {
			// Look for email-like pattern
			fields := strings.Fields(line)
			for _, field := range fields {
				field = strings.Trim(field, "<>[]()")
				if strings.Count(field, "@") == 1 && strings.Contains(field, ".") {
					return field
				}
			}
		}
	}

	return ""
}

// extractBounceType determines the type of bounce from the message.
func extractBounceType(resp string) string {
	respLower := strings.ToLower(resp)
	if strings.Contains(respLower, "5.1.1") || strings.Contains(respLower, "does not exist") || strings.Contains(respLower, "no such user") {
		return "invalid_address"
	}
	if strings.Contains(respLower, "5.2.2") || strings.Contains(respLower, "mailbox full") || strings.Contains(respLower, "quota") {
		return "mailbox_full"
	}
	if strings.Contains(respLower, "4.2.2") || strings.Contains(respLower, "rate limit") || strings.Contains(respLower, "too many") {
		return "rate_limited"
	}
	if strings.Contains(respLower, "5.7.1") || strings.Contains(respLower, "rejected") || strings.Contains(respLower, "spam") {
		return "rejected"
	}
	if strings.Contains(respLower, "5.4.1") || strings.Contains(respLower, "no mx") || strings.Contains(respLower, "domain") {
		return "domain_invalid"
	}
	return "unknown"
}

// extractDate extracts a date from an IMAP FETCH response.
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
	header = strings.ToUpper(header)
	lines := strings.Split(resp, "\r\n")
	for _, line := range lines {
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, header+":") {
			return strings.TrimSpace(line[len(header)+1:])
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
	// Map bounce stats to generic fields: scraped=totalFound, sent=replies, failed=newBounces
	_ = pool.RecordRunLog(ctx, workflow, status, totalBounces, matchedBounces, newBounces, replies, 0, durationMs, errMsg)
}
