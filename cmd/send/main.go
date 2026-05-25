// Command: send
// Picks up pending emails from email_queue and sends them.
// Updates status to sent/failed after each attempt.
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
	"github.com/arinbalyan/jobhunter/internal/email/sender"
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
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	logger := logging.New(cfg.LogLevel, os.Stdout)
	logger.Info("Send workflow starting...")

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

	// Fetch pending queue items
	queueItems, err := dbPool.GetPendingQueue(ctx, cfg.MaxEmailsPerRun)
	if err != nil {
		logger.Error("fetch queue: %v", err)
		os.Exit(1)
	}

	logger.Info("found %d pending emails to send", len(queueItems))

	if len(queueItems) == 0 {
		recordRun(ctx, dbPool, "send", "completed", 0, 0, 0, 0, 0, time.Since(startTime), "")
		return
	}

	emailSender := sender.New(sender.SMTPConfig{
		User:     cfg.GmailUser,
		Password: cfg.GmailAppPass,
		FromName: cfg.EmailFromName,
		FromAddr: cfg.GmailUser,
	})

	sent := 0
	failed := 0

	for i, item := range queueItems {
		select {
		case <-ctx.Done():
			logger.Info("interrupted after %d sent", sent)
			recordRun(ctx, dbPool, "send", "interrupted", 0, len(queueItems)-i, 0, sent, failed, time.Since(startTime), "interrupted")
			return
		default:
		}

		trackingID := uuid.New().String()
		messageID := fmt.Sprintf("<%s@jobhunter>", uuid.New().String())

		subject := fmt.Sprintf("Interested in %s role at %s", item.JobTitle, item.Company)
		body := buildEmail(item, cfg.ContactName)
		htmlBody := fmt.Sprintf("<html><body><p>%s</p></body></html>", body)
		htmlBody = sender.InjectTrackingPixel(htmlBody, cfg.TrackingServerURL, trackingID)

		msg := &sender.EmailMessage{
			To:         item.RecipientEmail,
			Subject:    subject,
			HTMLBody:   htmlBody,
			PlainBody:  body,
			TrackingID: trackingID,
			MessageID:  messageID,
		}

		logger.Info("sending (%d/%d): %s at %s -> %s", i+1, len(queueItems), item.JobTitle, item.Company, item.RecipientEmail)

		if err := emailSender.Send(ctx, msg); err != nil {
			logger.Error("failed: %v", err)
			dbPool.UpdateQueueStatus(ctx, item.ID, "failed", err.Error())
			failed++
		} else {
			logger.Info("sent successfully")
			dbPool.UpdateQueueStatus(ctx, item.ID, "sent", "")
			// Also update the job status
			dbPool.UpdateJobStatus(ctx, item.JobID, "sent", "", item.RecipientEmail)
			sent++
		}

		// Delay between sends
		if i < len(queueItems)-1 && cfg.EmailDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(cfg.EmailDelay):
			}
		}
	}

	duration := time.Since(startTime)
	logger.Info("send complete: %d sent, %d failed in %.0fs", sent, failed, duration.Seconds())

	recordRun(ctx, dbPool, "send", "completed", 0, 0, 0, sent, failed, duration, "")

	// Telegram notification
	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		msg := fmt.Sprintf(
			"<b>Send Complete</b>\nSent: %d\nFailed: %d\nDuration: %.0fs",
			sent, failed, duration.Seconds(),
		)
		sendTelegram(ctx, cfg.TelegramBotToken, cfg.TelegramChatID, msg)
	}
}

func buildEmail(item db.QueueItem, contactName string) string {
	// Simple template — will be replaced by LLM
	return fmt.Sprintf(
		"Hi %s team,\n\nI came across your %s opening and wanted to reach out. "+
			"My background aligns well with what you're looking for. "+
			"I'd love to connect and discuss how I can contribute.\n\nBest,\n%s",
		item.Company, item.JobTitle, contactName,
	)
}

func recordRun(ctx context.Context, pool *db.Pool, workflow, status string, scraped, pending, skipped, sent, failed int, dur time.Duration, errMsg string) {
	if pool == nil {
		return
	}
	_ = pool.RecordRunLog(ctx, workflow, status, scraped, pending, skipped, sent, failed, int(dur.Milliseconds()), errMsg)
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
