"""Default constants for jobSpy-V2.

All defaults are overridable via environment variables. These serve as
sensible out-of-the-box values so the system works without exhaustive
ENV configuration.
"""

from __future__ import annotations

# ---------------------------------------------------------------------------
# Job Title Reject Patterns (37 patterns from V1)
# Case-insensitive substring match against job titles.
# Override via REJECT_TITLES env var (comma-separated).
# ---------------------------------------------------------------------------
DEFAULT_REJECT_TITLES: tuple[str, ...] = (
    "teacher",
    "professor",
    "instructor",
    "tutor",
    "nurse",
    "nursing",
    "medical assistant",
    "healthcare",
    "cashier",
    "retail associate",
    "sales associate",
    "customer service",
    "call center",
    "receptionist",
    "driver",
    "delivery driver",
    "truck driver",
    "security guard",
    "cleaning",
    "janitor",
    "marketing coordinator",
    "social media manager",
    "hr manager",
    "hr coordinator",
    "recruiter",
    "talent acquisition",
    "accountant",
    "auditor",
    "lawyer",
    "paralegal",
    "chef",
    "cook",
    "restaurant",
    "warehouse",
    "electrician",
    "plumber",
    "mechanic",
)

# ---------------------------------------------------------------------------
# Email Filter Patterns (6 patterns from V1)
# Format: "starts_with:<pattern>" or "contains:<pattern>"
# Override via EMAIL_FILTER_PATTERNS env var (comma-separated).
# ---------------------------------------------------------------------------
DEFAULT_EMAIL_FILTER_PATTERNS: tuple[str, ...] = (
    "starts_with:accommodation@",
    "contains:accessibility",
    "contains:accommodation",
    "contains:no-reply",
    "contains:noreply",
    "contains:do-not-reply",
)

# ---------------------------------------------------------------------------
# LLM Cleanup Patterns
# Regex patterns applied to LLM-generated email text to strip hardcoded
# signatures or unwanted boilerplate the model may inject.
# ---------------------------------------------------------------------------
LLM_CLEANUP_PATTERNS: tuple[str, ...] = (
    r"Best regards,?\s*{contact_name}\s*\+?\d*\s*",
    r"{contact_name}\s*\+?\d*\s*$",
    r"I'm comfortable taking products from idea.*$",
)

# ---------------------------------------------------------------------------
# Email Footer Templates
# Placeholders: {contact_name}, {contact_phone}, {contact_portfolio},
#   {contact_github}, {contact_codolio}, {contact_linkedin},
#   {resume_drive_link}
# ---------------------------------------------------------------------------
CONTACT_INFO_FOOTER: str = """
{contact_name}
{contact_phone}
Portfolio: {contact_portfolio}
GitHub: {contact_github}"""

CONTACT_INFO_FOOTER_WITH_RESUME: str = """
Resume: {resume_drive_link}
{contact_name}
{contact_phone}
Portfolio: {contact_portfolio}
GitHub: {contact_github}"""

# ---------------------------------------------------------------------------
# Default Closing Paragraph (appended before footer when LLM omits one)
# ---------------------------------------------------------------------------
DEFAULT_CLOSING: str = (
    "I'm actively looking for full-time opportunities in Software Development "
    "(Python) or AI/ML. I'd welcome the chance to discuss how my background "
    "aligns with your team's needs."
)

# ---------------------------------------------------------------------------
# LLM Email Generation Prompt
# Placeholders: {context}, {job_title}, {company}, {job_description},
#   {min_words}, {max_words}
# ---------------------------------------------------------------------------
OPENROUTER_EMAIL_PROMPT: str = """\
You are writing a professional cold outreach email on behalf of a job applicant.

## Applicant Context
{context}

## Target Position
- Title: {job_title}
- Company: {company}
- Description: {job_description}

## Rules
1. Write in FIRST PERSON as the applicant — never refer to yourself in third person.
2. Keep the email between {min_words} and {max_words} words.
3. Structure: 5-6 short paragraphs (2-3 sentences each).
4. Do NOT include any URLs, links, phone numbers, or contact info in the body.
5. Do NOT include a sign-off block (no "Best regards, Name" — the system adds it).
6. Be specific about how the applicant's skills match this role.
7. Sound natural, confident, and concise — not generic or salesy.
8. Do NOT use any markdown formatting (no bold, italic, headers, or bullet points).

## Response Format
Return EXACTLY in this format (no extra text):

SUBJECT: [Your subject line here]

[Email body here — plain text only, no formatting]
"""

# ---------------------------------------------------------------------------
# Common Stop Words (for keyword extraction from job descriptions)
# ---------------------------------------------------------------------------
COMMON_STOP_WORDS: frozenset[str] = frozenset(
    {
        "the",
        "a",
        "an",
        "is",
        "are",
        "was",
        "were",
        "to",
        "of",
        "in",
        "for",
        "and",
        "or",
        "with",
        "on",
        "at",
        "by",
        "this",
        "that",
        "you",
        "your",
        "we",
        "our",
        "team",
        "role",
        "job",
        "work",
        "looking",
        "seeking",
        "experience",
        "years",
        "plus",
        "must",
        "required",
        "qualification",
        "skill",
        "skills",
        "will",
        "can",
        "should",
        "would",
        "could",
        "may",
        "about",
        "from",
        "into",
        "through",
        "during",
        "before",
        "after",
        "above",
        "below",
        "between",
        "under",
        "again",
        "further",
        "then",
        "once",
        "here",
        "there",
        "when",
        "where",
        "why",
        "how",
        "all",
        "each",
        "other",
        "some",
        "such",
        "no",
        "nor",
        "not",
        "only",
        "own",
        "same",
        "so",
        "than",
        "too",
        "very",
        "just",
        "am",
    }
)
