// Command: scrape
// Runs scrappy with config.yaml, stores all results in DB, evaluates
// each job against filters/rejections/dedup, and marks as pending/skipped.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/dedup"
	"github.com/arinbalyan/jobhunter/internal/logging"
	"github.com/arinbalyan/jobhunter/internal/migrations"
	"github.com/arinbalyan/jobhunter/internal/scraper"
	"github.com/arinbalyan/jobhunter/internal/telegram"
	"github.com/google/uuid"
)

func main() {
	if val := os.Getenv("GOMEMLIMIT"); val == "" {
		debug.SetMemoryLimit(80 * 1024 * 1024)
	}

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
	os.Exit(run(cfg, yamlCfg, logger))
}

func run(cfg *config.Config, yamlCfg *config.YAMLConfig, logger *logging.Logger) int {
	startTime := time.Now()

	logger.Info("Scrape workflow starting...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		cancel()
	}()

	// Database
	dbPool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed: %v", err)
		return 1
	}
	defer dbPool.Close()

	// Migrations
	if err := migrations.Run(cfg.DatabaseURL); err != nil {
		logger.Error("migrations failed: %v", err)
		return 1
	}

	scrCfg := scraper.Config{
		SearchTerms:   cfg.JobSearchTerms,
		Locations:     cfg.JobLocations,
		Sites:         cfg.JobSites,
		ResultsWanted: cfg.JobResultsPerSite,
		HoursOld:      cfg.JobHoursOld,
		SinceDate:     cfg.JobSinceDate,
		RemoteOnly:    cfg.JobRemoteOnly,
		JobType:       cfg.JobType,
		MemoryCapMB:   cfg.ScrapyMemoryCapMB,
		Proxy:         cfg.ScrapyProxy,
		EmailOnly:     true,
	}

	scr := scraper.New(scrCfg)
	jobs, err := scr.Scrape(ctx)
	if err != nil {
		logger.Error("scrape failed: %v", err)
		// Record the failed run
		recordRun(ctx, dbPool, "scrape", "failed", 0, 0, 0, 0, 0, time.Since(startTime), err.Error())
		return 1
	}

	logger.Info("scraped %d jobs from %d sites", len(jobs), len(cfg.JobSites))

	if len(jobs) == 0 {
		logger.Info("no jobs found")
		recordRun(ctx, dbPool, "scrape", "completed", 0, 0, 0, 0, 0, time.Since(startTime), "")
		return 0
	}

	// Evaluates each job and insert into DB
	pending := 0
	skipped := 0
	skippedReasons := make(map[string]int)

	de := dedup.New(dbPool, yamlCfg.Dedup.EmailCooldownDays)

	for _, j := range jobs {
		// 1. Title rejection
		if yamlCfg.RejectTitle(j.Title) {
			skippedReasons["title_rejected"]++
			insertJob(ctx, dbPool, &j, "skipped", "title_rejected", "", &skipped)
			continue
		}

		// 2. Extract and filter emails
		// Prefer MX-verified emails (scrappy verifies via DNS MX lookup),
		// then fall back to unverified ones.
		emails := j.PreferredEmails()
		if len(emails) == 0 {
			skippedReasons["no_email"]++
			insertJob(ctx, dbPool, &j, "skipped", "no_email", "", &skipped)
			continue
		}

		// Filter emails
		var validEmails []string
		for _, e := range emails {
			if !yamlCfg.FilterEmail(e) {
				validEmails = append(validEmails, e)
			} else {
				skippedReasons["email_filtered"]++
			}
		}
		if len(validEmails) == 0 {
			skippedReasons["no_valid_email"]++
			insertJob(ctx, dbPool, &j, "skipped", "no_valid_email", "", &skipped)
			continue
		}

		primaryEmail := validEmails[0]

		// 3. Dedup check
		canSend, reason := de.CanSend(ctx, primaryEmail)
		if !canSend {
			skippedReasons["dedup"]++
			insertJob(ctx, dbPool, &j, "skipped", reason, primaryEmail, &skipped)
			continue
		}

		// 4. Mark as pending and queue for sending
		jobID, err := insertJob(ctx, dbPool, &j, "pending", "", primaryEmail, &pending)
		if err != nil {
			log.Printf("queue job %s: insert failed: %v", j.ID, err)
			continue
		}

		// Enqueue the job for the send workflow.
		skillsJSON := "[]"
		if len(j.Skills) > 0 {
			b, _ := json.Marshal(j.Skills)
			skillsJSON = string(b)
		}
		if _, err := dbPool.InsertQueueItem(ctx, jobID, &db.QueueItemRecord{
			RecipientEmail:  primaryEmail,
			Company:         j.CompanyName,
			JobTitle:        j.Title,
			JobURL:          j.JobURL,
			JobLocation:     j.Location,
			IsRemote:        j.IsRemote,
			JobType:         j.JobType,
			JobDescription:  j.Description,
			Seniority:       j.Seniority,
			CompanyIndustry: j.Industry,
			Skills:          skillsJSON,
		}); err != nil {
			log.Printf("queue job %s: enqueue failed: %v", j.ID, err)
		}

		de.MarkSent(ctx, &db.EmailRecord{
			JobID:          &jobID,
			RecipientEmail: primaryEmail,
			Subject:        j.Title,
			BodyPreview:    truncate(j.Description, 200),
			Status:         "pending",
			TrackingID:     uuid.New().String(),
		})
	}

	duration := time.Since(startTime)
	logger.Info("results: %d pending, %d skipped (%s)", pending, skipped, summarizeReasons(skippedReasons))

	recordRun(ctx, dbPool, "scrape", "completed", len(jobs), pending, skipped, 0, 0, duration, "")

	// Check for Telegram
	tgToken := cfg.TelegramBotToken
	tgChat := cfg.TelegramChatID
	if tgToken != "" && tgChat != "" {
		msg := fmt.Sprintf(
			"<b>Scrape Complete</b>\nScraped: %d\nPending: %d\nSkipped: %d\nDuration: %.0fs\n%s",
			len(jobs), pending, skipped, duration.Seconds(), summarizeReasons(skippedReasons),
		)
		_ = telegram.SendMessage(ctx, tgToken, tgChat, msg)
	}

	return 0
}

func insertJob(ctx context.Context, pool *db.Pool, j *scraper.JobResult, status, skipReason, recipientEmail string, inserted *int) (int64, error) {
	emails := j.FlatEmails()
	emailsJSON := "[]"
	if len(emails) > 0 {
		b, _ := json.Marshal(emails)
		emailsJSON = string(b)
	}

	// Extract salary from compensation
	var salaryMin, salaryMax *float64
	salaryCurrency := "USD"
	if j.Compensation != nil {
		salaryMin = j.Compensation.MinAmount
		salaryMax = j.Compensation.MaxAmount
		if j.Compensation.Currency != "" {
			salaryCurrency = j.Compensation.Currency
		}
	}

	// Skills as JSON
	skillsJSON := "[]"
	if len(j.Skills) > 0 {
		b, _ := json.Marshal(j.Skills)
		skillsJSON = string(b)
	}

	jobID, isNew, err := pool.InsertJobFull(ctx, &db.FullJobRecord{
		JobID:              j.ID,
		Title:              j.Title,
		Company:            j.CompanyName,
		CompanyURL:         j.CompanyURL,
		JobURL:             j.JobURL,
		JobURLDirect:       j.JobURLDirect,
		Location:           j.Location,
		IsRemote:           j.IsRemote,
		Description:        j.Description,
		JobType:            j.JobType,
		DatePosted:         j.DatePosted,
		Source:             j.Site,
		Seniority:          j.Seniority,
		Department:         j.Department,
		CompanyIndustry:    j.Industry,
		SalaryMin:          salaryMin,
		SalaryMax:          salaryMax,
		SalaryCurrency:     salaryCurrency,
		ExperienceRange:    j.ExperienceRange,
		QualityScore:       j.QualityScore,
		Emails:             emailsJSON,
		Skills:             skillsJSON,
		Domain:             j.Domain,
		CompanyDescription: j.CompanyDescription,
		Status:             status,
		SkipReason:         skipReason,
		RecipientEmail:     recipientEmail,
	})
	if err != nil {
		log.Printf("insert job %s: %v", j.ID, err)
		return 0, err
	}
	if isNew {
		*inserted++
	}
	return jobID, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func summarizeReasons(reasons map[string]int) string {
	var parts []string
	for k, v := range reasons {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func recordRun(ctx context.Context, pool *db.Pool, workflow, status string, scraped, pending, skipped, sent, failed int, dur time.Duration, errMsg string) {
	if pool == nil {
		return
	}
	_ = pool.RecordRunLog(ctx, workflow, status, scraped, pending, skipped, sent, failed, int(dur.Milliseconds()), errMsg)
}

