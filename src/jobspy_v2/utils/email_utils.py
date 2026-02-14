"""Pure email validation and filtering utilities.

No SMTP, no network calls — just regex matching and filter pattern evaluation.
"""

from __future__ import annotations

import re
from collections.abc import Sequence

# RFC-5322 simplified: local@domain.tld (2+ char TLD, no consecutive dots)
EMAIL_REGEX: re.Pattern[str] = re.compile(
    r"^[a-zA-Z0-9._%+-]+@(?!.*\.\.)[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$"
)


def validate_email(email: str) -> bool:
    """Return True if *email* matches the simplified RFC-5322 pattern."""
    if not email or not isinstance(email, str):
        return False
    return EMAIL_REGEX.match(email.strip()) is not None


def extract_emails(emails_str: str) -> list[str]:
    """Split a comma-separated string into a list of validated emails.

    Invalid entries are silently dropped.
    """
    if not emails_str or not isinstance(emails_str, str):
        return []
    seen: set[str] = set()
    result: list[str] = []
    for raw in emails_str.split(","):
        email = raw.strip().lower()
        if email and email not in seen and validate_email(email):
            seen.add(email)
            result.append(email)
    return result


def matches_filter_pattern(email: str, pattern: str) -> bool:
    """Check if *email* matches a single filter pattern.

    Supported prefixes:
        ``starts_with:<prefix>``  — email starts with <prefix>
        ``contains:<substring>``  — email contains <substring>

    Unknown prefix formats are treated as ``contains:`` for safety.
    """
    if not email or not pattern:
        return False

    email_lower = email.lower()
    pattern = pattern.strip()

    if pattern.startswith("starts_with:"):
        prefix = pattern[len("starts_with:") :].lower()
        return email_lower.startswith(prefix)

    if pattern.startswith("contains:"):
        substring = pattern[len("contains:") :].lower()
        return substring in email_lower

    # Unknown prefix — treat as substring match (safe default)
    return pattern.lower() in email_lower


def is_filtered_email(email: str, filter_patterns: Sequence[str]) -> bool:
    """Return True if *email* matches ANY of the filter patterns (should be rejected)."""
    if not email or not filter_patterns:
        return False
    return any(matches_filter_pattern(email, p) for p in filter_patterns)


def get_valid_recipients(
    emails_str: str, filter_patterns: Sequence[str] = ()
) -> list[str]:
    """Full pipeline: extract → validate → filter.

    Returns only emails that are valid AND not caught by any filter pattern.
    """
    valid = extract_emails(emails_str)
    if not filter_patterns:
        return valid
    return [e for e in valid if not is_filtered_email(e, filter_patterns)]
