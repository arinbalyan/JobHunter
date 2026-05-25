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
}

func TestBuildUserPrompt(t *testing.T) {
	context := "Software Engineer with 3 years of Go experience"
	jobTitle := "Senior Golang Developer"
	company := "TestCorp"
	description := "We are looking for a Senior Golang Developer with experience in microservices, distributed systems, and cloud infrastructure."

	p := prompt.BuildUserPrompt(context, jobTitle, company, description, 500)
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
	if !strings.Contains(p, "microservices") {
		t.Error("expected description content in prompt")
	}
}

func TestBuildUserPrompt_Truncation(t *testing.T) {
	description := string(make([]byte, 10000)) // Very long description
	p := prompt.BuildUserPrompt("context", "title", "company", description, 100)
	if len(p) > 500 {
		t.Errorf("prompt too long (%d chars) after truncation", len(p))
	}
	if !strings.Contains(p, "...") {
		t.Error("expected truncation indicator")
	}
}
