package prompt_test

import (
	"strings"
	"testing"

	"github.com/arinbalyan/jobhunter/internal/llm/prompt"
)

func TestBuildSystemPrompt(t *testing.T) {
	p := prompt.BuildSystemPrompt(120, 300)
	if p == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !strings.Contains(p, "120") {
		t.Error("expected min_words=120 in prompt")
	}
	if !strings.Contains(p, "300") {
		t.Error("expected max_words=300 in prompt")
	}
	if !strings.Contains(p, "SUBJECT:") {
		t.Error("expected SUBJECT: format instruction")
	}
	if !strings.Contains(p, "first-person") && !strings.Contains(p, "first person") {
		t.Error("expected first person instruction")
	}
	if !strings.Contains(p, "NO URLs") {
		t.Error("expected URL prohibition")
	}
	// New: experience match strategies
	if !strings.Contains(p, "UNDERQUALIFIED") {
		t.Error("expected UNDERQUALIFIED strategy section")
	}
	if !strings.Contains(p, "OVERQUALIFIED") {
		t.Error("expected OVERQUALIFIED strategy section")
	}
	if !strings.Contains(p, "QUALIFIED") {
		t.Error("expected QUALIFIED strategy section")
	}
	// New: honesty directive
	if !strings.Contains(p, "fabricated") {
		t.Error("expected anti-fabrication instruction")
	}
}

func TestBuildUserPrompt(t *testing.T) {
	context := "Software Engineer with 3 years of Go experience"
	jobTitle := "Senior Golang Developer"
	company := "TestCorp"
	description := "We need a Senior Golang Developer with microservices experience."
	seniority := "senior"
	location := "Remote"
	jobType := "fulltime"
	salary := "$120k-$150k"
	skills := "Go, Kubernetes, PostgreSQL"
	industry := "SaaS"
	expMatch := "underqualified"
	roleMatch := "yes"
	yearsExp := 3

	p := prompt.BuildUserPrompt(
		context, jobTitle, company, description,
		seniority, location, jobType, salary, skills, industry,
		expMatch, roleMatch, yearsExp, 500,
	)
	if p == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !strings.Contains(p, context) {
		t.Error("expected context in prompt")
	}
	if !strings.Contains(p, jobTitle) {
		t.Error("expected job title in prompt")
	}
	if !strings.Contains(p, company) {
		t.Error("expected company in prompt")
	}
	if !strings.Contains(p, expMatch) {
		t.Error("expected experience match assessment in prompt")
	}
	if !strings.Contains(p, "3") {
		t.Error("expected years of experience")
	}
	if !strings.Contains(p, skills) {
		t.Error("expected skills listed")
	}
	if !strings.Contains(p, salary) {
		t.Error("expected salary info")
	}
}

func TestBuildUserPrompt_Truncation(t *testing.T) {
	description := string(make([]byte, 10000))
	_ = description
	// Just verify no panic with large description
	p := prompt.BuildUserPrompt(
		"ctx", "title", "company", string(make([]byte, 10000)),
		"senior", "Remote", "fulltime", "$100k", "Go", "Tech",
		"qualified", "yes", 3, 100,
	)
	if len(p) > 1000 {
		t.Errorf("prompt too long (%d chars) after truncation", len(p))
	}
	if !strings.Contains(p, "...") {
		t.Error("expected truncation indicator")
	}
}
