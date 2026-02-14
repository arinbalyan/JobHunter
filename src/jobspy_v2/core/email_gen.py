"""LLM-powered email generation with smart fallback."""

from __future__ import annotations

import logging
import re
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING

from openai import OpenAI

from jobspy_v2.config.defaults import (
    COMMON_STOP_WORDS,
    CONTACT_INFO_FOOTER,
    CONTACT_INFO_FOOTER_WITH_RESUME,
    DEFAULT_CLOSING,
    LLM_CLEANUP_PATTERNS,
    OPENROUTER_EMAIL_PROMPT,
)
from jobspy_v2.utils.text_utils import (
    clean_whitespace,
    count_words,
    extract_keywords,
    strip_html_markdown,
    truncate_to_word_limit,
)

if TYPE_CHECKING:
    from jobspy_v2.config.settings import Settings

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class EmailResult:
    """Generated email output."""

    subject: str
    body: str
    mode: str  # "llm" or "fallback"
    word_count: int


def generate_email(
    *,
    job_title: str,
    company: str,
    job_description: str,
    settings: Settings,
) -> EmailResult:
    """
    Generate a personalised cold email for a job posting.

    Tries LLM first (if mode=llm), falls back to template on failure.
    """
    if settings.email_generator_mode == "fallback":
        return _generate_fallback(
            job_title=job_title,
            company=company,
            settings=settings,
        )

    try:
        return _generate_with_llm(
            job_title=job_title,
            company=company,
            job_description=job_description,
            settings=settings,
        )
    except Exception:
        logger.exception("LLM generation failed, using fallback")
        return _generate_fallback(
            job_title=job_title,
            company=company,
            settings=settings,
        )


def _generate_with_llm(
    *,
    job_title: str,
    company: str,
    job_description: str,
    settings: Settings,
) -> EmailResult:
    """Generate email via OpenRouter LLM."""
    # Load applicant context
    context_text = _load_context(settings.context_file_path)

    # Extract keywords from job description
    keywords = extract_keywords(
        job_description,
        stop_words=COMMON_STOP_WORDS,
        top_n=15,
    )
    keyword_hint = ", ".join(keywords) if keywords else "general software role"

    # Build prompt
    prompt = OPENROUTER_EMAIL_PROMPT.format(
        context=context_text,
        job_title=job_title,
        company=company,
        job_description=job_description[:3000],  # truncate huge descriptions
        min_words=settings.min_email_words,
        max_words=settings.max_email_words,
    )

    # Call LLM
    client = OpenAI(
        base_url=settings.llm_base_url,
        api_key=settings.openrouter_api_key,
    )
    response = client.chat.completions.create(
        model=settings.llm_model,
        messages=[
            {
                "role": "system",
                "content": (
                    "You are a professional job applicant writing cold emails. "
                    "Write naturally in first person. No markdown formatting. "
                    "No HTML. No bold/italic. Plain text only."
                ),
            },
            {"role": "user", "content": prompt},
        ],
        temperature=0.7,
        max_tokens=1024,
    )

    raw_text = response.choices[0].message.content or ""
    if not raw_text.strip():
        raise ValueError("LLM returned empty response")

    # Parse subject and body
    subject, body = _parse_llm_response(raw_text)

    # Clean up
    body = _cleanup_body(body, settings)

    # Append contact footer
    body = _append_footer(body, settings)

    word_count = count_words(body)
    logger.info("LLM email: %d words, subject='%s'", word_count, subject[:50])

    return EmailResult(
        subject=subject,
        body=body,
        mode="llm",
        word_count=word_count,
    )


def _generate_fallback(
    *,
    job_title: str,
    company: str,
    settings: Settings,
) -> EmailResult:
    """Generate email from fallback template in settings."""
    replacements = _build_replacements(settings)

    subject = settings.fallback_email_subject
    for key, value in replacements.items():
        subject = subject.replace(f"{{{key}}}", value)

    body = settings.fallback_email_body
    # Handle literal \n from env var
    body = body.replace("\\n", "\n")
    for key, value in replacements.items():
        body = body.replace(f"{{{key}}}", value)

    word_count = count_words(body)
    logger.info("Fallback email: %d words, subject='%s'", word_count, subject[:50])

    return EmailResult(
        subject=subject,
        body=body,
        mode="fallback",
        word_count=word_count,
    )


def _load_context(context_path: str) -> str:
    """Read applicant profile context file."""
    path = Path(context_path)
    if not path.exists():
        logger.warning("Context file not found: %s", path)
        return ""
    return path.read_text(encoding="utf-8")


def _parse_llm_response(raw: str) -> tuple[str, str]:
    """
    Parse LLM response into (subject, body).

    Expected format:
    SUBJECT: <subject line>

    <body text>
    """
    raw = raw.strip()

    # Try to find SUBJECT: line
    subject_match = re.match(r"^SUBJECT:\s*(.+?)(?:\n\n|\n)", raw, re.IGNORECASE)
    if subject_match:
        subject = subject_match.group(1).strip()
        body = raw[subject_match.end() :].strip()
    else:
        # No SUBJECT: prefix — use first line as subject
        lines = raw.split("\n", 1)
        subject = lines[0].strip()
        body = lines[1].strip() if len(lines) > 1 else ""

    # Remove quotes around subject
    subject = subject.strip("\"'")

    return subject, body


def _cleanup_body(body: str, settings: Settings) -> str:
    """Apply cleanup pipeline to email body."""
    # Strip HTML/markdown artifacts
    body = strip_html_markdown(body)

    # Apply LLM cleanup patterns (remove signatures, personal info leaks)
    for pattern in LLM_CLEANUP_PATTERNS:
        body = re.sub(pattern, "", body, flags=re.MULTILINE | re.IGNORECASE)

    # Clean whitespace
    body = clean_whitespace(body)

    # Truncate to max words
    body = truncate_to_word_limit(body, settings.max_email_words)

    return body.strip()


def _append_footer(body: str, settings: Settings) -> str:
    """Append contact info footer, with resume link if configured."""
    replacements = _build_replacements(settings)

    if settings.resume_drive_link:
        template = CONTACT_INFO_FOOTER_WITH_RESUME
    else:
        template = CONTACT_INFO_FOOTER

    footer = template
    for key, value in replacements.items():
        footer = footer.replace(f"{{{key}}}", value)

    return body + footer


def _build_replacements(settings: Settings) -> dict[str, str]:
    """Build template replacement dict from settings."""
    return {
        "contact_name": settings.contact_name,
        "contact_phone": settings.contact_phone,
        "contact_portfolio": settings.contact_portfolio,
        "contact_github": settings.contact_github,
        "contact_codolio": settings.contact_codolio or "",
        "contact_linkedin": settings.contact_linkedin or "",
        "resume_drive_link": settings.resume_drive_link or "",
    }
