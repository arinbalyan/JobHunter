// Command: followup
// Finds sent emails with no reply after N days from the same domain
// and sends a gentle follow-up via the email_queue system.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
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
	"github.com/google/uuid"
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
	logger.Info("Follow-up workflow starting...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
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

	// Find sent emails (no reply, no bounce) from 4+ days ago
	// These are candidates for follow-up
	candidates, err := dbPool.GetFollowUpCandidates(ctx, 4)
	if err != nil {
		logger.Error("fetch follow-up candidates: %v", err)
		os.Exit(1)
	}

	logger.Info("found %d follow-up candidates", len(candidates))

	followups := 0
	skipped := 0

	for _, c := range candidates {
		// Extract domain from the original sent email
		domain := extractDomain(c.RecipientEmail)
		if domain == "" {
			skipped++
			continue
		}

		// Check if we already sent a follow-up to this domain today
		// by looking at the email_queue for follow-up entries
		alreadyFollowed, err := dbPool.HasFollowUpForDomain(ctx, domain, 24)
		if err != nil {
			logger.Warn("check follow-up domain: %v", err)
			skipped++
			continue
		}
		if alreadyFollowed {
			logger.Debug("already followed up to domain %s today, skipping", domain)
			skipped++
			continue
		}

		// Insert a follow-up queue item
		subject := fmt.Sprintf("Following up on %s role", c.JobTitle)
		body := fmt.Sprintf(
			"Hi %s team,\n\nI wanted to follow up on my recent application for the %s role. "+
				"I remain very interested in the opportunity and would love to hear any updates.\n\n"+
				"Happy to hop on a call at your convenience to discuss further.\n\nBest,\n%s",
			c.Company, c.JobTitle, cfg.ContactName,
		)

		trackingID := uuid.New().String()
		messageID := fmt.Sprintf("<followup-%s@jobhunter>", uuid.New().String())

		// Use the original email's ID as parent_job_id for tracking
		_, err = dbPool.InsertFollowUp(ctx, &db.FollowUpRecord{
			JobID:           c.JobID,
			OriginalEmailID: c.ID,
			RecipientEmail:  c.RecipientEmail,
			Domain:          domain,
			Company:         c.Company,
			JobTitle:        c.JobTitle,
			Subject:         subject,
			Body:            body,
			TrackingID:      trackingID,
			MessageID:       messageID,
			Status:          "pending",
		})
		if err != nil {
			logger.Error("insert follow-up: %v", err)
			skipped++
			continue
		}

		followups++
		logger.Info("queued follow-up for %s at %s (domain: %s)", c.JobTitle, c.Company, domain)
	}

	duration := time.Since(startTime)
	logger.Info("follow-up complete: %d queued, %d skipped in %.0fs", followups, skipped, duration.Seconds())

	_ = dbPool.RecordRunLog(ctx, "followup", "completed", 0, followups, skipped, 0, 0, int(duration.Milliseconds()), "")

	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		msg := fmt.Sprintf(
			"<b>Follow-up Complete</b>\nQueued: %d\nSkipped: %d\nDuration: %.0fs",
			followups, skipped, duration.Seconds(),
		)
		sendTelegram(ctx, cfg.TelegramBotToken, cfg.TelegramChatID, msg)
	}
}

// extractDomain pulls the domain from an email address.
func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[1]))
}

func sendTelegram(ctx context.Context, token, chatID, msg string) {
	body := fmt.Sprintf(`{"chat_id":"%s","text":"%s","parse_mode":"HTML"}`, chatID, strings.ReplaceAll(msg, `"`, `\"`))
	req, _ := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}
