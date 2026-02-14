"""Tests for jobspy_v2.utils — email_utils and text_utils."""

from __future__ import annotations

import pytest

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


# ── email_utils: validate_email ──────────────────────────────────────────────


class TestValidateEmail:
    """Tests for validate_email — RFC-5322 simplified."""

    @pytest.mark.parametrize(
        "email",
        [
            "user@example.com",
            "first.last@company.co.uk",
            "user+tag@gmail.com",
            "a@b.cd",
            "user_name@domain.org",
            "test123@sub.domain.com",
        ],
    )
    def test_valid_emails(self, email: str) -> None:
        assert validate_email(email) is True

    @pytest.mark.parametrize(
        "email",
        [
            "",
            "not-an-email",
            "@no-local.com",
            "no-domain@",
            "no-tld@domain",
            "spaces in@email.com",
            "user@.com",
            "user@domain..com",
        ],
    )
    def test_invalid_emails(self, email: str) -> None:
        assert validate_email(email) is False

    def test_none_returns_false(self) -> None:
        assert validate_email(None) is False  # type: ignore[arg-type]

    def test_whitespace_only_returns_false(self) -> None:
        assert validate_email("   ") is False


# ── email_utils: extract_emails ──────────────────────────────────────────────


class TestExtractEmails:
    """Tests for extract_emails — CSV string to validated list."""

    def test_single_email(self) -> None:
        assert extract_emails("user@example.com") == ["user@example.com"]

    def test_multiple_emails(self) -> None:
        result = extract_emails("a@b.com, c@d.com, e@f.org")
        assert result == ["a@b.com", "c@d.com", "e@f.org"]

    def test_filters_invalid(self) -> None:
        result = extract_emails("good@email.com, bad-email, another@ok.com")
        assert result == ["good@email.com", "another@ok.com"]

    def test_lowercases(self) -> None:
        result = extract_emails("USER@EXAMPLE.COM")
        assert result == ["user@example.com"]

    def test_strips_whitespace(self) -> None:
        result = extract_emails("  user@example.com  ,  test@test.com  ")
        assert result == ["user@example.com", "test@test.com"]

    def test_empty_string(self) -> None:
        assert extract_emails("") == []

    def test_all_invalid(self) -> None:
        assert extract_emails("bad, also-bad, @nope") == []

    def test_deduplicates(self) -> None:
        result = extract_emails("a@b.com, A@B.COM, a@b.com")
        assert result == ["a@b.com"]


# ── email_utils: matches_filter_pattern ──────────────────────────────────────


class TestMatchesFilterPattern:
    """Tests for matches_filter_pattern — starts_with: and contains: prefixes."""

    def test_starts_with_match(self) -> None:
        assert (
            matches_filter_pattern("noreply@company.com", "starts_with:noreply") is True
        )

    def test_starts_with_no_match(self) -> None:
        assert (
            matches_filter_pattern("user@company.com", "starts_with:noreply") is False
        )

    def test_contains_match(self) -> None:
        assert (
            matches_filter_pattern("team-noreply-bot@co.com", "contains:noreply")
            is True
        )

    def test_contains_no_match(self) -> None:
        assert matches_filter_pattern("user@company.com", "contains:noreply") is False

    def test_case_insensitive(self) -> None:
        assert matches_filter_pattern("NoReply@Company.COM", "contains:noreply") is True
        assert matches_filter_pattern("NOREPLY@test.com", "starts_with:noreply") is True

    def test_unknown_prefix_defaults_to_contains(self) -> None:
        assert matches_filter_pattern("noreply@test.com", "noreply") is True

    def test_empty_pattern(self) -> None:
        assert matches_filter_pattern("user@test.com", "") is False


# ── email_utils: is_filtered_email ───────────────────────────────────────────


class TestIsFilteredEmail:
    """Tests for is_filtered_email — multiple patterns."""

    def test_matches_one_pattern(self) -> None:
        patterns = ["starts_with:noreply", "contains:accommodation"]
        assert is_filtered_email("noreply@co.com", patterns) is True

    def test_no_match(self) -> None:
        patterns = ["starts_with:noreply", "contains:accommodation"]
        assert is_filtered_email("hiring@company.com", patterns) is False

    def test_empty_patterns(self) -> None:
        assert is_filtered_email("anything@test.com", []) is False


# ── email_utils: get_valid_recipients ────────────────────────────────────────


class TestGetValidRecipients:
    """Tests for get_valid_recipients — full pipeline."""

    def test_full_pipeline(self) -> None:
        csv = "hiring@co.com, noreply@co.com, bad-email, jobs@co.com"
        filters = ["starts_with:noreply"]
        result = get_valid_recipients(csv, filters)
        assert result == ["hiring@co.com", "jobs@co.com"]

    def test_all_filtered(self) -> None:
        csv = "noreply@co.com, do-not-reply@co.com"
        filters = ["contains:noreply", "contains:do-not-reply"]
        result = get_valid_recipients(csv, filters)
        assert result == []

    def test_no_filters(self) -> None:
        csv = "a@b.com, c@d.com"
        result = get_valid_recipients(csv, [])
        assert result == ["a@b.com", "c@d.com"]

    def test_empty_csv(self) -> None:
        result = get_valid_recipients("", ["contains:noreply"])
        assert result == []


# ── text_utils: strip_html_markdown ──────────────────────────────────────────


class TestStripHtmlMarkdown:
    """Tests for strip_html_markdown — HTML + Markdown removal."""

    def test_strips_html_tags(self) -> None:
        assert strip_html_markdown("<p>Hello</p>") == "Hello"
        assert strip_html_markdown("<b>Bold</b> text") == "Bold text"

    def test_strips_nested_html(self) -> None:
        result = strip_html_markdown("<div><p><strong>Deep</strong></p></div>")
        assert result == "Deep"

    def test_strips_self_closing_tags(self) -> None:
        result = strip_html_markdown("Line one<br/>Line two")
        assert result == "Line one Line two"

    def test_strips_bold_markdown(self) -> None:
        assert strip_html_markdown("This is **bold** text") == "This is bold text"

    def test_strips_italic_markdown(self) -> None:
        assert strip_html_markdown("This is *italic* text") == "This is italic text"

    def test_strips_inline_code(self) -> None:
        assert strip_html_markdown("Use `print()` here") == "Use print() here"

    def test_strips_markdown_links(self) -> None:
        result = strip_html_markdown("Visit [Google](https://google.com) now")
        assert result == "Visit Google now"

    def test_strips_heading_hashes(self) -> None:
        result = strip_html_markdown("## Section Title")
        assert result == "Section Title"

    def test_collapses_whitespace(self) -> None:
        result = strip_html_markdown("  too   many    spaces  ")
        assert result == "too many spaces"

    def test_preserves_paragraph_breaks(self) -> None:
        result = strip_html_markdown("Para one.\n\nPara two.")
        assert "Para one." in result
        assert "Para two." in result

    def test_empty_string(self) -> None:
        assert strip_html_markdown("") == ""

    def test_plain_text_unchanged(self) -> None:
        assert strip_html_markdown("Just plain text") == "Just plain text"


# ── text_utils: clean_whitespace ─────────────────────────────────────────────


class TestCleanWhitespace:
    """Tests for clean_whitespace."""

    def test_collapses_spaces(self) -> None:
        assert clean_whitespace("a   b   c") == "a b c"

    def test_strips_edges(self) -> None:
        assert clean_whitespace("  hello  ") == "hello"

    def test_empty_string(self) -> None:
        assert clean_whitespace("") == ""


# ── text_utils: count_words ──────────────────────────────────────────────────


class TestCountWords:
    """Tests for count_words."""

    def test_simple_sentence(self) -> None:
        assert count_words("Hello world foo bar") == 4

    def test_empty_string(self) -> None:
        assert count_words("") == 0

    def test_whitespace_only(self) -> None:
        assert count_words("   ") == 0

    def test_single_word(self) -> None:
        assert count_words("hello") == 1

    def test_with_extra_whitespace(self) -> None:
        assert count_words("  hello   world  ") == 2


# ── text_utils: truncate_to_word_limit ───────────────────────────────────────


class TestTruncateToWordLimit:
    """Tests for truncate_to_word_limit."""

    def test_under_limit_unchanged(self) -> None:
        text = "short sentence here"
        assert truncate_to_word_limit(text, 10) == text

    def test_at_limit_unchanged(self) -> None:
        text = "one two three"
        assert truncate_to_word_limit(text, 3) == text

    def test_over_limit_truncated(self) -> None:
        text = "one two three four five"
        result = truncate_to_word_limit(text, 3)
        assert result == "one two three"

    def test_zero_limit(self) -> None:
        assert truncate_to_word_limit("hello world", 0) == ""

    def test_empty_string(self) -> None:
        assert truncate_to_word_limit("", 5) == ""


# ── text_utils: extract_keywords ─────────────────────────────────────────────


class TestExtractKeywords:
    """Tests for extract_keywords — frequency-based keyword extraction."""

    def test_basic_extraction(self) -> None:
        text = "python developer python engineer python machine learning"
        result = extract_keywords(text, frozenset())
        assert result[0] == "python"  # most frequent

    def test_filters_stop_words(self) -> None:
        stop = frozenset({"the", "and", "for"})
        text = "the best python and java for the web"
        result = extract_keywords(text, stop)
        assert "the" not in result
        assert "and" not in result
        assert "for" not in result

    def test_filters_short_words(self) -> None:
        text = "we do ml and ai in the us at ny"
        result = extract_keywords(text, frozenset(), min_length=3)
        # all words <= 2 chars should be filtered
        for kw in result:
            assert len(kw) >= 3

    def test_top_n_limit(self) -> None:
        text = "a1234 b1234 c1234 d1234 e1234 f1234 g1234 h1234"
        result = extract_keywords(text, frozenset(), top_n=3)
        assert len(result) <= 3

    def test_returns_lowercase(self) -> None:
        text = "Python JAVA JavaScript TypeScript"
        result = extract_keywords(text, frozenset())
        for kw in result:
            assert kw == kw.lower()

    def test_empty_text(self) -> None:
        assert extract_keywords("", frozenset()) == []

    def test_all_stop_words(self) -> None:
        stop = frozenset({"the", "and", "is"})
        text = "the and is"
        assert extract_keywords(text, stop) == []
