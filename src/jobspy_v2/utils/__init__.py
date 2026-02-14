"""Utility functions for email validation, text processing, and keyword extraction."""

from jobspy_v2.utils.email_utils import (
    extract_emails,
    get_valid_recipients,
    is_filtered_email,
    matches_filter_pattern,
    validate_email,
)
from jobspy_v2.utils.text_utils import (
    clean_whitespace,
    count_words,
    extract_keywords,
    strip_html_markdown,
    truncate_to_word_limit,
)

__all__ = [
    "clean_whitespace",
    "count_words",
    "extract_emails",
    "extract_keywords",
    "get_valid_recipients",
    "is_filtered_email",
    "matches_filter_pattern",
    "strip_html_markdown",
    "truncate_to_word_limit",
    "validate_email",
]
