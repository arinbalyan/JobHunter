package job

import (
	"strings"
	"time"

	"github.com/arinbalyan/jobhunter/internal/scraper"
)

// JobFilter filters jobs based on user preferences.
type JobFilter struct {
	YearsExperience int
	TargetRoles     []string
	RemoteOnly      bool
}

// MatchResult holds the result of matching a job against user profile.
type MatchResult struct {
	Score           int      // 0-100
	ExperienceMatch string   // qualified | underqualified | overqualified | unknown
	RoleMatch       bool
	Reasons         []string
}

// FilterJobs filters and scores jobs based on user preferences.
func FilterJobs(jobs []scraper.JobResult, filter JobFilter) ([]scraper.JobResult, []MatchResult) {
	var filtered []scraper.JobResult
	var matches []MatchResult

	for _, job := range jobs {
		if filter.RemoteOnly && !job.IsRemote {
			continue
		}

		match := scoreJob(job, filter)
		if match.Score >= 30 {
			filtered = append(filtered, job)
			matches = append(matches, match)
		}
	}

	return filtered, matches
}

// scoreJob calculates how well a job matches the user profile.
func scoreJob(job scraper.JobResult, filter JobFilter) MatchResult {
	result := MatchResult{}
	score := 50

	// Role match
	for _, target := range filter.TargetRoles {
		if strings.Contains(strings.ToLower(job.Title), strings.ToLower(target)) {
			result.RoleMatch = true
			score += 20
			result.Reasons = append(result.Reasons, "Role matches target: "+target)
			break
		}
	}

	// Experience matching
	result.ExperienceMatch = assessExperience(job.Seniority, filter.YearsExperience)
	switch result.ExperienceMatch {
	case "qualified":
		score += 15
		result.Reasons = append(result.Reasons, "Experience level matches")
	case "underqualified":
		score -= 10
		result.Reasons = append(result.Reasons, "Experience gap")
	case "overqualified":
		score -= 5
		result.Reasons = append(result.Reasons, "Overqualified for role")
	}

	// Company has direct email = higher response chance
	if len(job.Emails) > 0 {
		score += 10
		result.Reasons = append(result.Reasons, "Direct email available")
	}

	// Recently posted
	if job.DatePosted != nil && time.Since(*job.DatePosted) < 24*time.Hour {
		score += 10
		result.Reasons = append(result.Reasons, "Recently posted")
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	result.Score = score
	return result
}

// assessExperience compares job seniority to user experience.
func assessExperience(jobSeniority string, userYears int) string {
	switch jobSeniority {
	case "entry":
		if userYears <= 2 {
			return "qualified"
		}
		return "overqualified"
	case "mid":
		if userYears >= 2 && userYears <= 5 {
			return "qualified"
		}
		if userYears < 2 {
			return "underqualified"
		}
		return "overqualified"
	case "senior":
		if userYears >= 5 && userYears <= 10 {
			return "qualified"
		}
		if userYears < 5 {
			return "underqualified"
		}
		return "overqualified"
	case "lead":
		if userYears >= 8 {
			return "qualified"
		}
		return "underqualified"
	default:
		if userYears <= 2 {
			return "underqualified"
		}
		return "qualified"
	}
}
