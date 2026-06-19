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

	"net"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
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

	emailOnly := true
	for _, arg := range os.Args[1:] {
		if arg == "--email=false" || arg == "--email=0" || arg == "--email=no" {
			emailOnly = false
		}
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
	os.Exit(run(cfg, yamlCfg, logger, emailOnly))
}

func run(cfg *config.Config, yamlCfg *config.YAMLConfig, logger *logging.Logger, emailOnly bool) int {
	startTime := time.Now()

	logger.Info("Scrape workflow starting...")

	maxRuntime := time.Duration(cfg.MaxRuntimeMinutes) * time.Minute
	if maxRuntime <= 0 {
		maxRuntime = 350 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), maxRuntime)
	defer cancel()
	// dbCtx outlives the timeout so DB inserts still work after max runtime.
	dbCtx := context.WithoutCancel(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			logger.Info("shutting down (signal)...")
			cancel()
		case <-ctx.Done():
			logger.Info("shutting down (max runtime reached)...")
		}
	}()

	// Database
	dbPool, err := db.Connect(dbCtx, cfg.DatabaseURL)
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
		EmailOnly:     emailOnly,
	}

	scr := scraper.New(scrCfg)
	jobs, err := scr.Scrape(ctx)
	if err != nil {
		logger.Error("scrape failed: %v", err)
		// Record the failed run
		recordRun(dbCtx, dbPool, "scrape", "failed", 0, 0, 0, 0, 0, time.Since(startTime), err.Error())
		return 1
	}

	logger.Info("scraped %d jobs from %d sites", len(jobs), len(cfg.JobSites))

	if len(jobs) == 0 {
		logger.Info("no jobs found")
		recordRun(dbCtx, dbPool, "scrape", "completed", 0, 0, 0, 0, 0, time.Since(startTime), "")
		return 0
	}

	// Evaluates each job and insert into DB
	pending := 0
	skipped := 0
	skippedReasons := make(map[string]int)

	for _, j := range jobs {
		// 1. Title rejection
		if yamlCfg.RejectTitle(j.Title) {
			skippedReasons["title_rejected"]++
			insertJob(dbCtx, dbPool, &j, "skipped", "title_rejected", "", &skipped)
			continue
		}

		// 2. Extract and filter emails
		emails := j.PreferredEmails()
		if len(emails) == 0 {
			skippedReasons["no_email"]++
			insertJob(dbCtx, dbPool, &j, "skipped", "no_email", "", &skipped)
			continue
		}

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
			insertJob(dbCtx, dbPool, &j, "skipped", "no_valid_email", "", &skipped)
			continue
		}

		primaryEmail := validEmails[0]

		// 3. MX verify: check domain can receive email
		if !verifyEmailMX(primaryEmail) {
			skippedReasons["mx_invalid"]++
			skipped++
			continue
		}

		// 4. Reserve email (atomic cooldown + bounce check).
		// If NOT reserved, skip without touching jobs/queue tables.
		trackingID := uuid.New().String()
		reserved, err := dbPool.ReserveEmail(dbCtx, &db.EmailRecord{
			RecipientEmail: primaryEmail,
			Subject:        j.Title,
			BodyPreview:    truncate(j.Description, 200),
			Status:         "pending",
			TrackingID:     trackingID,
		}, yamlCfg.Dedup.EmailCooldownDays)
		if err != nil {
			log.Printf("reserve email for %s: %v", primaryEmail, err)
		}
		if !reserved {
			skippedReasons["dedup"]++
			skipped++
			continue
		}

		// 5. Reserve succeeded — insert job and queue
		jobID, err := insertJob(dbCtx, dbPool, &j, "pending", "", primaryEmail, &pending)
		if err != nil {
			log.Printf("queue job %s: insert failed: %v", j.ID, err)
			continue
		}

		// Link the email record to the job
		dbPool.Exec(dbCtx, `UPDATE emails SET job_id = $1 WHERE tracking_id = $2`, jobID, trackingID)

		skillsJSON := "[]"
		if len(j.Skills) > 0 {
			b, _ := json.Marshal(j.Skills)
			skillsJSON = string(b)
		}
		if _, err := dbPool.InsertQueueItem(dbCtx, jobID, &db.QueueItemRecord{
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
	}

	duration := time.Since(startTime)
	logger.Info("results: %d pending, %d skipped (%s)", pending, skipped, summarizeReasons(skippedReasons))

	recordRun(dbCtx, dbPool, "scrape", "completed", len(jobs), pending, skipped, 0, 0, duration, "")

	// Check for Telegram
	tgToken := cfg.TelegramBotToken
	tgChat := cfg.TelegramChatID
	if tgToken != "" && tgChat != "" {
		// Get total queue count
		var queueTotal int
		dbPool.QueryRow(dbCtx, "SELECT COUNT(*) FROM email_queue WHERE status='pending'").Scan(&queueTotal)

		// Get time-windowed stats
		stats, _ := dbPool.GetTimeWindowStats(dbCtx)
		statsBlock := ""
		if stats != nil {
			statsBlock = "\n" + stats.FormatStatsBlock("📊 Email Stats")
		}

		msg := fmt.Sprintf(
			"<b>🕷️ Scrape Complete</b>\n\n"+
				"Scraped: %d\n"+
				"Pending: %d | Skipped: %d\n"+
				"Sites: %d/%d\n"+
				"Queue total: %d\n"+
				"Duration: %.0fs\n\n"+
				"<i>%s</i>%s",
			len(jobs), pending, skipped,
			func() int {
				unique := make(map[string]bool)
				for _, j := range jobs {
					unique[j.Site] = true
				}
				return len(unique)
			}(), len(cfg.JobSites),
			queueTotal,
			duration.Seconds(),
			summarizeReasons(skippedReasons),
			statsBlock,
		)
		_ = telegram.SendMessage(dbCtx, tgToken, tgChat, msg)
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

// verifyEmailMX checks that the email's domain has MX records (can receive mail).
func verifyEmailMX(email string) bool {
	_, domain, ok := strings.Cut(email, "@")
	if !ok || domain == "" {
		return false
	}
	mxs, err := net.LookupMX(domain)
	if err != nil || len(mxs) == 0 {
		return false
	}
	return true
}

func recordRun(ctx context.Context, pool *db.Pool, workflow, status string, scraped, pending, skipped, sent, failed int, dur time.Duration, errMsg string) {
	if pool == nil {
		return
	}
	_ = pool.RecordRunLog(ctx, workflow, status, scraped, pending, skipped, sent, failed, int(dur.Milliseconds()), errMsg)
}

