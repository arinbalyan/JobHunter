package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/email/sender"
	"github.com/arinbalyan/jobhunter/internal/job"
	"github.com/arinbalyan/jobhunter/internal/logging"
	"github.com/arinbalyan/jobhunter/internal/plugin/sdk"
	"github.com/arinbalyan/jobhunter/internal/scraper"
	"github.com/google/uuid"
)

// JobHunterPlugin is the core email outreach agent.
// It scrapes jobs, matches them, generates personalized emails, and sends them.
type JobHunterPlugin struct {
	sdk.BasePlugin
}

func init() {
	// Metadata set here so it's available at registration time
}

// NewJobHunterPlugin creates the core job hunter plugin.
func NewJobHunterPlugin() *JobHunterPlugin {
	return &JobHunterPlugin{
		BasePlugin: sdk.BasePlugin{
			PluginID:   "jobhunter",
			PluginName: "Job Hunter Agent",
			PluginDesc: "Scrapes job boards, matches jobs to user profile, sends personalized outreach emails",
		},
	}
}

// Execute runs the job hunter pipeline.
func (p *JobHunterPlugin) Execute(ctx context.Context, env sdk.Env) (*sdk.Result, error) {
	log := env.Logger()
	log.Info("starting job hunter pipeline...")

	// Get config from env
	cfg, ok := env.Config().(*config.Config)
	if !ok {
		return sdk.ErrorResult("failed to get config"), fmt.Errorf("config type assertion failed")
	}

	// ── Step 1: Scrape jobs ──────────────────────────────────────────────
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
		return sdk.SimpleResult("no jobs found, nothing to do"), nil
	}

	// ── Step 2: Filter and match jobs ────────────────────────────────────
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

	// ── Step 3: Determine email template per job ─────────────────────────
	log.Info("generating emails...")
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
		templateName := p.selectTemplate(match.ExperienceMatch)

		// Generate tracking ID
		trackingID := uuid.New().String()
		messageID := fmt.Sprintf("<%s@jobhunter>", uuid.New().String())

		// Build email content
		subject := fmt.Sprintf("Interested in %s role at %s", j.Title, j.Company)

		body := p.buildEmailBody(j, match, templateName)
		htmlBody := fmt.Sprintf("<html><body><p>%s</p></body></html>", body)

		// Inject tracking pixel
		htmlBody = sender.InjectTrackingPixel(htmlBody, cfg.TrackingServerURL, trackingID)

		// Send with delay
		if cfg.EmailDelay > 0 && i > 0 {
			log.Info("waiting %v before next email...", cfg.EmailDelay)
			select {
			case <-ctx.Done():
				return sdk.SimpleResult(fmt.Sprintf("interrupted after %d sent", sentCount)), ctx.Err()
			case <-time.After(cfg.EmailDelay):
			}
		}

		msg := &sender.EmailMessage{
			To:         j.ID, // This would be the actual email — need to resolve from job
			Subject:    subject,
			HTMLBody:   htmlBody,
			PlainBody:  body,
			TrackingID: trackingID,
			MessageID:  messageID,
		}

		// For now, use a placeholder recipient (real email extraction TBD)
		msg.To = "candidate@" + p.extractDomain(j.CompanyURL)

		if err := emailSender.Send(ctx, msg); err != nil {
			log.Error("failed to send email for %s at %s: %v", j.Title, j.Company, err)
			errors++
			continue
		}

		sentCount++
		log.Info("sent email for %s at %s (tracking: %s)", j.Title, j.Company, trackingID)

		// Record in DB via stats collector
	}

	return &sdk.Result{
		Success: true,
		Message: fmt.Sprintf("sent %d emails, %d errors", sentCount, errors),
		Metrics: map[string]float64{
			"jobs_scraped":   float64(len(jobs)),
			"jobs_matched":   float64(len(filteredJobs)),
			"emails_sent":    float64(sentCount),
			"email_errors":   float64(errors),
		},
	}, nil
}

// selectTemplate picks the right template based on experience match.
func (p *JobHunterPlugin) selectTemplate(expMatch string) string {
	switch expMatch {
	case "qualified":
		return "qualified"
	case "underqualified":
		return "experience_gap"
	case "overqualified":
		return "overqualified"
	default:
		return "qualified"
	}
}

// buildEmailBody constructs the email plaintext body.
func (p *JobHunterPlugin) buildEmailBody(j job.JobResult, match job.MatchResult, template string) string {
	switch template {
	case "experience_gap":
		return fmt.Sprintf(
			"Hi %s team,\n\n"+
				"I'm reaching out about the %s role. While I'm early in my career with relevant project experience, "+
				"I'm deeply interested in this space and have been building with similar technologies. "+
				"I'd love a chance to discuss how my skills could contribute to your team.\n\n"+
				"Best,\n%s",
			j.Company, j.Title, "Applicant",
		)
	case "overqualified":
		return fmt.Sprintf(
			"Hi %s team,\n\n"+
				"I'm writing about the %s position. My background aligns well with what you're looking for, "+
				"and I'm specifically drawn to %s because of [specific reason]. "+
				"I'm looking for a role where I can make a deep impact, and I believe this could be a great fit.\n\n"+
				"Would you be open to a conversation?\n\n"+
				"Best,\n%s",
			j.Company, j.Title, j.Company, "Applicant",
		)
	default: // qualified
		return fmt.Sprintf(
			"Hi %s team,\n\n"+
				"I came across your %s opening and wanted to reach out. "+
				"My experience aligns well with what you're looking for, and I'm excited about the work you're doing at %s.\n\n"+
				"I'd love to connect and discuss how I can contribute.\n\n"+
				"Best,\n%s",
			j.Company, j.Title, j.Company, "Applicant",
		)
	}
}

// extractDomain extracts a domain from a URL for email purposes.
func (p *JobHunterPlugin) extractDomain(url string) string {
	if url == "" {
		return "unknown.com"
	}
	// Simple extraction — would be more robust in production
	for _, prefix := range []string{"https://", "http://", "www."} {
		if len(url) > len(prefix) && url[:len(prefix)] == prefix {
			url = url[len(prefix):]
		}
	}
	// Take just the domain
	for i, c := range url {
		if c == '/' || c == ':' {
			return url[:i]
		}
	}
	return url
}
