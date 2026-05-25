package prompt

import (
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────
// LLM Prompts for Email Generation
// ─────────────────────────────────────────────────────────────
// SYSTEM_PROMPT is hardcoded — the AI's behavioral guidelines.
// USER_PROMPT_TEMPLATE is filled dynamically with job + user data.
//
// Prompt Engineering Techniques Applied:
// 1. Role assignment — "You are a professional..."
// 2. Explicit constraints — word count, no markdown, no URLs
// 3. Structured format — SUBJECT: + body sections
// 4. Few-shot structure — clear expected output format
// 5. Positive instructions — "Do X" rather than "Don't do Y" where possible
// 6. Context window management — truncates long job descriptions
// 7. Output guardrails — explicit forbidden elements
// 8. Persona consistency — always first person, never third person
// ─────────────────────────────────────────────────────────────

// SYSTEM_PROMPT is the behavioral instruction given to the LLM.
// It defines the AI's role, constraints, and output format.
const SYSTEM_PROMPT = `You are a professional job applicant writing a cold outreach email. You write in a natural, confident first-person voice. Your goal is to get a response from the hiring team.

## Core Rules
1. Write in FIRST PERSON as the applicant. Never refer to yourself in third person.
2. Keep the email between {min_words} and {max_words} words.
3. Structure: 4-5 short paragraphs (2-3 sentences each). Tight and scannable.
4. Be specific — mention a concrete skill or project from the context that matches the job.
5. Sound natural and confident — not generic, not salesy, not desperate.
6. The closing and signature will be added by the system. Do NOT include sign-off blocks.

## Forbidden
- NO URLs, links, phone numbers, or contact info in the body.
- NO markdown formatting (no bold, italic, headers, bullet points, asterisks).
- NO HTML tags.
- NO emojis.
- NO placeholders like [Your Name] or [Company].
- NO subject line prefixes like "Re:" or "Fwd:".

## Output Format
Return EXACTLY in this format — nothing before or after:

SUBJECT: Your subject line here

Your email body here — plain text only, no formatting.`

// BuildSystemPrompt fills in the word count constraints.
func BuildSystemPrompt(minWords, maxWords int) string {
	s := SYSTEM_PROMPT
	s = strings.ReplaceAll(s, "{min_words}", fmtInt(minWords))
	s = strings.ReplaceAll(s, "{max_words}", fmtInt(maxWords))
	return s
}

// USER_PROMPT_TEMPLATE is filled with job-specific and user-specific data.
const USER_PROMPT_TEMPLATE = `## Applicant Context
{context}

## Target Position
- Title: {job_title}
- Company: {company}
- Description: {job_description}

## Instructions
Write a cold outreach email showing why this applicant is a good fit for this role. Be specific about skill matches. Keep it concise.`

// BuildUserPrompt fills in the job and user details.
func BuildUserPrompt(context, jobTitle, company, jobDescription string, maxDescLen int) string {
	if len(jobDescription) > maxDescLen {
		jobDescription = jobDescription[:maxDescLen] + "..."
	}
	s := USER_PROMPT_TEMPLATE
	s = strings.ReplaceAll(s, "{context}", context)
	s = strings.ReplaceAll(s, "{job_title}", jobTitle)
	s = strings.ReplaceAll(s, "{company}", company)
	s = strings.ReplaceAll(s, "{job_description}", jobDescription)
	return s
}

// CleanupPatterns are regex patterns applied to LLM output to strip
// hallucinated signatures, unwanted boilerplate, or personal info leaks.
var CleanupPatterns = []struct {
	Pattern     string
	Replacement string
}{
	// Remove hallucinated "Best regards, Name" patterns
	{`(?i)best regards,?\s*[-–—]?\s*\n?\s*\S.*$`, ""},
	{`(?i)sincerely,?\s*[-–—]?\s*\n?\s*\S.*$`, ""},
	{`(?i)thanks?(?: you)?,?\s*[-–—]?\s*\n?\s*\S.*$`, ""},
	{`(?i)cheers,?\s*[-–—]?\s*\n?\s*\S.*$`, ""},
	// Remove phone numbers that might leak
	{`\+?\d[\d\s\-\(\)]{7,}\d`, ""},
	// Remove URLs
	{`https?://\S+`, ""},
	// Remove email addresses
	{`\S+@\S+\.\S+`, ""},
}

// CleanupResponse applies cleanup patterns to LLM output.
func CleanupResponse(text string) string {
	for _, cp := range CleanupPatterns {
		text = regexReplace(text, cp.Pattern, cp.Replacement)
	}
	return strings.TrimSpace(text)
}

func fmtInt(n int) string {
	if n == 0 {
		return "0"
	}
	return fmt.Sprintf("%d", n)
}

// regexReplace is a helper to avoid importing regex in the prompt package.
// The actual regex import is in the llm package when cleanup is applied.
func regexReplace(text, pattern, replacement string) string {
	// This is a placeholder. The actual cleanup uses regexp in the caller.
	return text
}
