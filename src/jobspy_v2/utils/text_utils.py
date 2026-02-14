"""Pure text manipulation utilities.

HTML/Markdown stripping, word counting, truncation, and keyword extraction.
No external dependencies — stdlib only.
"""

from __future__ import annotations

import re
from collections import Counter

# Pre-compiled patterns for strip_html_markdown
_HTML_TAG = re.compile(r"<[^>]+>")
_MD_BOLD = re.compile(r"\*\*(.+?)\*\*")
_MD_ITALIC = re.compile(r"\*(.+?)\*")
_MD_CODE = re.compile(r"`(.+?)`")
_MD_LINK = re.compile(r"\[([^\]]+)\]\([^)]+\)")
_MD_HEADING = re.compile(r"^#{1,6}\s+", re.MULTILINE)
_MULTI_SPACE = re.compile(r"[ \t]+")
_MULTI_NEWLINE = re.compile(r"\n{3,}")


def strip_html_markdown(text: str) -> str:
    """Remove HTML tags and Markdown formatting, collapse whitespace.

    Preserves paragraph breaks (double newlines) but collapses triple+.
    """
    if not text or not isinstance(text, str):
        return ""

    result = _HTML_TAG.sub(" ", text)
    result = _MD_BOLD.sub(r"\1", result)
    result = _MD_ITALIC.sub(r"\1", result)
    result = _MD_CODE.sub(r"\1", result)
    result = _MD_LINK.sub(r"\1", result)
    result = _MD_HEADING.sub("", result)
    result = _MULTI_SPACE.sub(" ", result)
    result = _MULTI_NEWLINE.sub("\n\n", result)
    return result.strip()


def clean_whitespace(text: str) -> str:
    """Collapse multiple spaces/tabs to single space and strip."""
    if not text or not isinstance(text, str):
        return ""
    return _MULTI_SPACE.sub(" ", text).strip()


def count_words(text: str) -> int:
    """Count words in text (split on whitespace)."""
    if not text or not isinstance(text, str):
        return 0
    return len(text.split())


def truncate_to_word_limit(text: str, max_words: int) -> str:
    """Truncate text at a word boundary to fit within *max_words*.

    Returns the original text unchanged if already within limit.
    """
    if not text or not isinstance(text, str):
        return ""
    if max_words < 1:
        return ""

    words = text.split()
    if len(words) <= max_words:
        return text
    return " ".join(words[:max_words])


def extract_keywords(
    text: str,
    stop_words: frozenset[str],
    *,
    min_length: int = 3,
    top_n: int = 15,
) -> list[str]:
    """Extract top keywords by frequency, filtering stop words and short tokens.

    Returns up to *top_n* keywords sorted by descending frequency.
    Tokens are lowercased and must contain only alphanumeric characters.
    """
    if not text or not isinstance(text, str):
        return []

    # Tokenize: extract alphanumeric sequences
    tokens = re.findall(r"[a-zA-Z0-9]+", text.lower())

    # Filter: length threshold + stop words
    filtered = [t for t in tokens if len(t) >= min_length and t not in stop_words]

    if not filtered:
        return []

    # Rank by frequency, return top N
    counts = Counter(filtered)
    return [word for word, _ in counts.most_common(top_n)]
