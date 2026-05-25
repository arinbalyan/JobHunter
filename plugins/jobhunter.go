package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/email/sender"
	"github.com/arinbalyan/jobhunter/internal/job"
	"github.com/arinbalyan/jobhunter/internal/plugin/sdk"
	"github.com/arinbalyan/jobhunter/internal/scraper"
	"github.com/google/uuid"
)

// JobHunterPlugin is the core email outreach agent.
type JobHunterPlugin struct {
	sdk.BasePlugin
}

// NewJobHunterPlugin creates the core job hunter plugin.
func NewJobHunterPlugin() *JobHunterPlugin {
	return &JobHunterPlugin{
		BasePlugin: sdk.BasePlugin{
			PluginID:   "jobhunter",
			PluginName: "Job Hunter Agent",
			PluginDesc: "Scrapes job boards, matches jobs to user profile, sends outreach emails",
		},
	}
}

// Execute runs the job hunter pipeline.
func (p *JobHunterPlugin) Execute(ctx context.Context, env sdk.Env) (*sdk.Result, error) {
	log := env.Logger()
	log.Info("starting job hunter pipeline...")

	cfg, ok := env.Config().(*config.Config)
	if !ok {
		return sdk.ErrorResult("config type assertion failed"), fmt.Errorf("config type assertion failed")
	}

	// ── Step 1: Scrape jobs ──
	log.Info("scraping job boards...")
	scr := scraper.New(scraper.Config{
		Sites:          cfg.JobSites,
		SearchTerms:    cfg.JobSearchTerms,
		Locations:      cfg.JobLocations,
		ResultsPerSite: cfg.JobResultsPerSite,
		HoursOld:       cfg.JobHoursOld,
		RemoteOnly:     cfg.JobRemoteOnly,
		JobType:        cfg.JobType,
		MemoryCapMB:    cfg.ScrapyMemoryCapMB,
		Proxy:          cfg.ScrapyProxy,
	})

	jobs, err := scr.Scrape(ctx)
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("scrape failed: %v", err)), fmt.Errorf("scrape: %w", err)
	}
	log.Info("scraped %d jobs", len(jobs))

	if len(jobs) == 0 {
		return sdk.SimpleResult("no jobs found"), nil
	}

	// ── Step 2: Filter & match ──
	log.Info("filtering and matching jobs...")
	filter := job.JobFilter{
		YearsExperience: cfg.UserYearsExperience,
		TargetRoles:     cfg.UserTargetRoles,
		RemoteOnly:      cfg.JobRemoteOnly,
	}

	filteredJobs, matches := job.FilterJobs(jobs, filter)
	log.Info("matched %d/%d jobs", len(filteredJobs), len(jobs))

	if len(filteredJobs) == 0 {
		return sdk.SimpleResult("no jobs matched filters"), nil
	}

	// ── Step 3: Send emails ──
	log.Info("generating and sending emails...")
	emailSender := sender.New(sender.SMTPConfig{
		User:     cfg.GmailUser,
		Password: cfg.GmailAppPass,
		FromName: cfg.EmailFromName,
		FromAddr: cfg.GmailUser,
	})

	sentCount := 0
	errors := 0

	for i, j := range filteredJobs {
		if i >= cfg.MaxEmailsPerRun {
			log.Info("reached max emails per run (%d)", cfg.MaxEmailsPerRun)
			break
		}

		match := matches[i]
		trackingID := uuid.New().String()
		messageID := fmt.Sprintf("<%s@jobhunter>", uuid.New().String())

		subject := fmt.Sprintf("Interested in %s role at %s", j.Title, j.Company)
		body := p.buildEmailBody(j, match.ExperienceMatch)
		htmlBody := fmt.Sprintf("<html><body><p>%s</p></body></html>", body)
		htmlBody = sender.InjectTrackingPixel(htmlBody, cfg.TrackingServerURL, trackingID)

		// Delay between emails
		if cfg.EmailDelay > 0 && i > 0 {
			log.Info("waiting %v...", cfg.EmailDelay)
			select {
			case <-ctx.Done():
				return sdk.SimpleResult(fmt.Sprintf("interrupted after %d sent", sentCount)), ctx.Err()
			case <-time.After(cfg.EmailDelay):
			}
		}

		recipient := fmt.Sprintf("careers@%s", p.extractDomain(j.CompanyURL))
		if len(j.Emails) > 0 {
			recipient = j.Emails[0]
		}

		msg := &sender.EmailMessage{
			To:         recipient,
			Subject:    subject,
			HTMLBody:   htmlBody,
			PlainBody:  body,
			TrackingID: trackingID,
			MessageID:  messageID,
		}

		if err := emailSender.Send(ctx, msg); err != nil {
			log.Error("failed to send for %s at %s: %v", j.Title, j.Company, err)
			errors++
			continue
		}

		sentCount++
		log.Info("sent: %s at %s (tracking: %s)", j.Title, j.Company, trackingID)
	}

	return &sdk.Result{
		Success: true,
		Message: fmt.Sprintf("sent %d emails, %d errors", sentCount, errors),
		Metrics: map[string]float64{
			"jobs_scraped": float64(len(jobs)),
			"jobs_matched": float64(len(filteredJobs)),
			"emails_sent":  float64(sentCount),
			"email_errors": float64(errors),
		},
	}, nil
}

func (p *JobHunterPlugin) buildEmailBody(j scraper.JobResult, expMatch string) string {
	switch expMatch {
	case "underqualified":
		return fmt.Sprintf(
			"Hi %s team,\n\nI'm reaching out about the %s role. While I'm early in my career, "+
				"I have hands-on experience with the relevant technologies and I'm deeply interested in this space. "+
				"I'd love a chance to discuss how I can contribute.\n\nBest,\n%s",
			j.Company, j.Title, "Applicant",
		)
	case "overqualified":
		return fmt.Sprintf(
			"Hi %s team,\n\nI'm writing about the %s position. My background aligns well, "+
				"and I'm specifically drawn to %s because of the work you're doing. "+
				"I'd love to discuss how I can make an impact.\n\nBest,\n%s",
			j.Company, j.Title, j.Company, "Applicant",
		)
	default:
		return fmt.Sprintf(
			"Hi %s team,\n\nI came across your %s opening and wanted to reach out. "+
				"My experience aligns well with what you're looking for. "+
				"I'd love to connect and discuss how I can contribute.\n\nBest,\n%s",
			j.Company, j.Title, "Applicant",
		)
	}
}

func (p *JobHunterPlugin) extractDomain(url string) string {
	if url == "" {
		return "unknown.com"
	}
	for _, prefix := range []string{"https://", "http://", "www."} {
		if len(url) > len(prefix) && url[:len(prefix)] == prefix {
			url = url[len(prefix):]
		}
	}
	for i, c := range url {
		if c == '/' || c == ':' {
			return url[:i]
		}
	}
	return url
}
