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
	"github.com/arinbalyan/jobhunter/internal/llm/prompt"
	"github.com/arinbalyan/jobhunter/internal/llm/providers"
	"github.com/arinbalyan/jobhunter/internal/llm/router"
	"github.com/arinbalyan/jobhunter/internal/logging"
	"github.com/arinbalyan/jobhunter/internal/migrations"
	"github.com/google/uuid"
)

func main() {
	if val := os.Getenv("GOMEMLIMIT"); val == "" {
		debug.SetMemoryLimit(80 * 1024 * 1024)
	}

	dryRun := false
	for _, arg := range os.Args[1:] {
		if arg == "--dry-run" || arg == "-n" {
			dryRun = true
		}
	}

	startTime := time.Now()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	yamlCfg, err := config.LoadYAML(".agent-data/config.yaml")
	if err == nil {
		yamlCfg.MergeIntoConfig(cfg)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	logger := logging.New(cfg.LogLevel, os.Stdout)
	logger.Info("Send workflow starting...")

	// Load LLM provider config from llm.yaml
	llmProviders, err := providers.LoadProviders(".agent-data/llm.yaml")
	if err != nil {
		logger.Warn("could not load llm.yaml: %v", err)
	}
	activeProviders := llmProviders.GetActiveProviders("complex")
	logger.Info("loaded %d active LLM providers from llm.yaml", len(activeProviders))

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

	// ── Quota check ──
	quota := newQuotaTracker(ctx, dbPool, cfg.DailyEmailLimit)
	if quota.exhausted() {
		logger.Warn("daily email quota exhausted (%d/%d)", quota.todayCount, cfg.DailyEmailLimit)
		recordRun(ctx, dbPool, "send", "quota_exhausted", 0, 0, 0, 0, 0, time.Since(startTime), "daily quota exhausted")
		return
	}
	logger.Info("email quota: %d/%d used, %d remaining", quota.todayCount, cfg.DailyEmailLimit, quota.remaining())

	// ── Fetch pending queue ──
	maxToSend := quota.remaining()
	if maxToSend > cfg.MaxEmailsPerRun {
		maxToSend = cfg.MaxEmailsPerRun
	}

	queueItems, err := dbPool.GetPendingQueue(ctx, maxToSend)
	if err != nil {
		logger.Error("fetch queue: %v", err)
		os.Exit(1)
	}
	logger.Info("found %d pending emails to send", len(queueItems))
	if len(queueItems) == 0 {
		recordRun(ctx, dbPool, "send", "completed", 0, 0, 0, 0, 0, time.Since(startTime), "")
		return
	}

	// ── Load context for LLM ──
	contextText := loadContext()

	// ── Init LLM router from llm.yaml ──
	var llmRouter *router.Router
	activeProvs := activeProviders
	if len(activeProvs) > 0 {
		routerCfgs := make([]router.ProviderConfig, len(activeProvs))
		for i, p := range activeProvs {
			routerCfgs[i] = router.ProviderConfig{
				Kind:    router.ProviderKind(p.Kind),
				APIKey:  p.APIKey,
				BaseURL: p.BaseURL,
				Model:   p.Model,
				Weight:  p.Weight,
			}
		}
		llmRouter = router.New(routerCfgs, cfg.MaxTokensPerRun, logger)
		logger.Info("LLM router initialized with %d providers", len(activeProvs))
	}

	// ── Init email sender with resume attachment support ──
	emailSender := sender.New(sender.SMTPConfig{
		User:       cfg.GmailUser,
		Password:   cfg.GmailAppPass,
		FromName:   cfg.EmailFromName,
		FromAddr:   cfg.GmailUser,
		ResumePath: cfg.ResumeDriveLink,
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

		// Determine experience match from item metadata (stored during scrape)
		expMatch := item.ExperienceMatch
		if expMatch == "" {
			expMatch = "qualified"
		}

		// Build LLM prompt
		sysPrompt := prompt.BuildSystemPrompt(120, 300)
		userPrompt := prompt.BuildUserPrompt(
			contextText,
			item.JobTitle,
			item.Company,
			item.JobDescription,
			item.Seniority,
			item.JobLocation,
			item.JobType,
			formatSalary(item.SalaryMin, item.SalaryMax, item.SalaryCurrency),
			item.Skills,
			item.CompanyIndustry,
			expMatch,
			"yes", // role match
			cfg.UserYearsExperience,
			3000,
		)

		// Generate email via LLM or fallback
		subject, body := generateEmail(ctx, llmRouter, sysPrompt, userPrompt, item, cfg.ContactName, cfg, logger)

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

		if dryRun {
			logger.Info("[DRY-RUN] would send: subject=%s, body=%d chars", msg.Subject, len(msg.PlainBody))
			sent++
		} else if err := emailSender.Send(ctx, msg); err != nil {
			logger.Error("failed: %v", err)
			dbPool.UpdateQueueStatus(ctx, item.ID, "failed", err.Error())
			failed++
		} else {
			logger.Info("sent successfully")
			dbPool.UpdateQueueStatus(ctx, item.ID, "sent", "")
			dbPool.UpdateJobStatus(ctx, item.JobID, "sent", "", item.RecipientEmail)
			sent++
		}

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

	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		msg := fmt.Sprintf(
			"<b>Send Complete</b>\nSent: %d\nFailed: %d\nDuration: %.0fs",
			sent, failed, duration.Seconds(),
		)
		telegramNotify(ctx, cfg.TelegramBotToken, cfg.TelegramChatID, msg)
	}
}

func generateEmail(ctx context.Context, llmRouter *router.Router, sysPrompt, userPrompt string, item db.QueueItem, contactName string, cfg *config.Config, logger *logging.Logger) (string, string) {
	subject := fmt.Sprintf("Interested in %s role at %s", item.JobTitle, item.Company)
	body := fmt.Sprintf(
		"Hi %s team,\n\nI came across your %s opening and wanted to reach out. "+
			"My background aligns well with what you're looking for. "+
			"I'd love to connect and discuss how I can contribute.\n\nBest,\n%s",
		item.Company, item.JobTitle, contactName,
	)

	if llmRouter == nil || sysPrompt == "" || userPrompt == "" {
		return subject, body
	}

	resp, err := llmRouter.Complete(ctx, router.TaskComplex, &router.CompletionRequest{
		SystemPrompt: sysPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    1024,
		Temperature:  0.7,
	})
	if err != nil {
		logger.Warn("LLM generation failed: %v — using fallback template", err)
		return subject, body
	}

	// Parse SUBJECT: prefix
	content := resp.Content
	subj := subject
	if strings.HasPrefix(strings.ToUpper(content), "SUBJECT:") {
		parts := strings.SplitN(content, "\n", 2)
		subj = strings.TrimSpace(strings.TrimPrefix(parts[0], "SUBJECT:"))
		subj = strings.Trim(subj, "\"' ")
		if len(parts) > 1 {
			content = parts[1]
		}
	}

	// Append contact footer (no resume mention)
	body = strings.TrimSpace(content)
	body += fmt.Sprintf("\n\n%s\nPhone: %s\nPortfolio: %s\nGitHub: %s",
		contactName, cfg.ContactPhone, cfg.ContactPortfolio, cfg.ContactGithub)
	if cfg.ContactLinkedin != "" {
		body += "\nLinkedIn: " + cfg.ContactLinkedin
	}

	return subj, body
}

func loadContext() string {
	data, err := os.ReadFile(".agent-data/CONTEXT.md")
	if err != nil {
		return ""
	}
	return string(data)
}

func formatSalary(min, max *float64, currency string) string {
	if min == nil && max == nil {
		return "Not specified"
	}
	if min != nil && max != nil && *min == *max {
		return fmt.Sprintf("%s%.0f", currency, *min)
	}
	if min != nil && max != nil {
		return fmt.Sprintf("%s%.0f - %s%.0f", currency, *min, currency, *max)
	}
	if min != nil {
		return fmt.Sprintf("From %s%.0f", currency, *min)
	}
	return fmt.Sprintf("Up to %s%.0f", currency, *max)
}

type quotaTracker struct {
	todayCount int
	dailyLimit int
}

func newQuotaTracker(ctx context.Context, pool *db.Pool, dailyLimit int) *quotaTracker {
	count := 0
	if pool != nil {
		if c, err := pool.GetTodaySentCount(ctx); err == nil {
			count = c
		}
	}
	return &quotaTracker{todayCount: count, dailyLimit: dailyLimit}
}

func (q *quotaTracker) remaining() int {
	r := q.dailyLimit - q.todayCount
	if r < 0 {
		return 0
	}
	return r
}

func (q *quotaTracker) exhausted() bool {
	return q.todayCount >= q.dailyLimit
}

func recordRun(ctx context.Context, pool *db.Pool, workflow, status string, scraped, pending, skipped, sent, failed int, dur time.Duration, errMsg string) {
	if pool == nil {
		return
	}
	_ = pool.RecordRunLog(ctx, workflow, status, scraped, pending, skipped, sent, failed, int(dur.Milliseconds()), errMsg)
}

func telegramNotify(ctx context.Context, token, chatID, msg string) {
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
