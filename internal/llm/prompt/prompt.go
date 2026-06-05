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
// 1. Role assignment — "You are a professional job applicant..."
// 2. Conditional logic — different behavior for under/over/qualified
// 3. Explicit constraints — word count, no markdown, no URLs
// 4. Structured format — SUBJECT: + body sections
// 5. Persona consistency — always first person, never third person
// 6. Context window management — truncates long job descriptions
// 7. Output guardrails — explicit forbidden elements
// 8. Honesty directive — never fabricate experience
// ─────────────────────────────────────────────────────────────

// SYSTEM_PROMPT is the behavioral instruction given to the LLM.
// It defines the AI's role, constraints, and conditional strategies
// for different experience match levels.
const SYSTEM_PROMPT = `You are a professional job applicant writing a cold outreach email. You write in a natural, confident first-person voice. Your goal is to get a response from the hiring team.

## Core Rules
1. Write in FIRST PERSON as the applicant. Never refer to yourself in third person.
2. Keep the email between {min_words} and {max_words} words.
3. Structure: 4-5 short paragraphs (2-3 sentences each). Tight and scannable.
4. Be specific — mention a concrete skill, project, or achievement from the context that matches the job description.
5. Sound natural and confident — not generic, not salesy, not desperate.
6. The closing and signature will be added by the system. Do NOT include sign-off blocks.

## Experience Match Strategies

### When UNDERQUALIFIED (user has less experience than the role requires)
- Lead with honesty: acknowledge the gap directly but frame it positively.
- Immediately pivot to relevant projects, skills, or quick learning ability.
- Emphasize transferable skills and enthusiasm for the specific domain.
- Suggest a conversation to discuss fit rather than making a hard ask.
- NEVER apologize excessively or sound insecure. Frame it as "early but accelerating."

### When OVERQUALIFIED (user has more experience than the role requires)
- Show genuine interest in THIS specific role and company — avoid sounding like you're settling.
- Explain WHY you want this particular role: specific technology, industry, company stage, etc.
- Address retention concerns implicitly: show you're looking for impact, not just a title.
- Emphasize what unique value you bring beyond the listed requirements.
- Keep it humble — don't list credentials, focus on what you can DO for them.

### When QUALIFIED (good match)
- Be direct and confident. Lead with your strongest relevant skill.
- Show you've done your research on the company — reference their product, tech stack, or mission.
- Keep it tight: why you, why this role, why now.
- End with a clear, low-friction call to action (e.g., "I'd love a quick chat").

## Forbidden
- NO URLs, links, phone numbers, or contact info in the body.
- NO markdown formatting (no bold, italic, headers, bullet points, asterisks).
- NO HTML tags.
- NO emojis.
- NO placeholders like [Your Name] or [Company].
- NO subject line prefixes like "Re:" or "Fwd:".
- NO fabricated experience. Never claim a skill or project you don't have.
- NO apologies for lack of experience — reframe as a positive instead.

## Output Format
Return EXACTLY in this format — nothing before or after:

SUBJECT: Your subject line here

Your email body here — plain text only, no formatting.`

// BuildSystemPrompt fills in the word count constraints.
func BuildSystemPrompt(minWords, maxWords int) string {
	s := SYSTEM_PROMPT
	s = strings.ReplaceAll(s, "{min_words}", fmt.Sprintf("%d", minWords))
	s = strings.ReplaceAll(s, "{max_words}", fmt.Sprintf("%d", maxWords))
	return s
}

// USER_PROMPT_TEMPLATE is filled with job-specific and user-specific data.
// Includes experience match level so the LLM knows which strategy to use.
const USER_PROMPT_TEMPLATE = `## Applicant Context
{context}

## Target Position
- Title: {job_title}
- Company: {company}
- Description: {job_description}
- Seniority: {seniority}
- Location: {location}
- Job Type: {job_type}
- Salary: {salary}
- Skills Listed: {skills}
- Industry: {industry}

## Match Assessment
- Experience Match: {experience_match}
- User's Years of Experience: {years_exp}
- Role Match: {role_match}

## Instructions
Write a cold outreach email following the strategy for "{experience_match}" candidates. Be specific about skill matches between the applicant's context and this job description. Keep it concise.`

// BuildUserPrompt fills in the job and user details.
// experienceMatch is one of: qualified, underqualified, overqualified.
func BuildUserPrompt(
	context, jobTitle, company, jobDescription, seniority, location, jobType,
	salary, skills, industry, experienceMatch, roleMatch string,
	yearsExp int,
	maxDescLen int,
) string {
	if len(jobDescription) > maxDescLen {
		jobDescription = jobDescription[:maxDescLen] + "..."
	}
	s := USER_PROMPT_TEMPLATE
	s = strings.ReplaceAll(s, "{context}", context)
	s = strings.ReplaceAll(s, "{job_title}", jobTitle)
	s = strings.ReplaceAll(s, "{company}", company)
	s = strings.ReplaceAll(s, "{job_description}", jobDescription)
	s = strings.ReplaceAll(s, "{seniority}", seniority)
	s = strings.ReplaceAll(s, "{location}", location)
	s = strings.ReplaceAll(s, "{job_type}", jobType)
	s = strings.ReplaceAll(s, "{salary}", salary)
	s = strings.ReplaceAll(s, "{skills}", skills)
	s = strings.ReplaceAll(s, "{industry}", industry)
	s = strings.ReplaceAll(s, "{experience_match}", experienceMatch)
	s = strings.ReplaceAll(s, "{years_exp}", fmt.Sprintf("%d", yearsExp))
	s = strings.ReplaceAll(s, "{role_match}", roleMatch)
	return s
}

// CleanupPatterns are applied to LLM output to strip hallucinated signatures.
type CleanupRule struct {
	Pattern     string
	Replacement string
}

var CleanupPatterns = []CleanupRule{
	{`(?i)best regards,?\s*[-–—]?\s*\n?\s*\S.*$`, ""},
	{`(?i)sincerely,?\s*[-–—]?\s*\n?\s*\S.*$`, ""},
	{`(?i)thanks?(?: you)?,?\s*[-–—]?\s*\n?\s*\S.*$`, ""},
	{`(?i)cheers,?\s*[-–—]?\s*\n?\s*\S.*$`, ""},
	{`\+?\d[\d\s\-\(\)]{7,}\d`, ""},
	{`https?://\S+`, ""},
	{`\S+@\S+\.\S+`, ""},
}
