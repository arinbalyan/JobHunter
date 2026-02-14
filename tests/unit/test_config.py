"""Tests for the config layer (defaults + settings)."""

from __future__ import annotations

import pytest

from jobspy_v2.config.defaults import (
    COMMON_STOP_WORDS,
    CONTACT_INFO_FOOTER,
    CONTACT_INFO_FOOTER_WITH_RESUME,
    DEFAULT_CLOSING,
    DEFAULT_EMAIL_FILTER_PATTERNS,
    DEFAULT_REJECT_TITLES,
    LLM_CLEANUP_PATTERNS,
    OPENROUTER_EMAIL_PROMPT,
)
from jobspy_v2.config.settings import Settings, get_settings


# ===================================================================
# defaults.py
# ===================================================================


class TestDefaults:
    """Verify built-in defaults have expected shape and content."""

    def test_reject_titles_count(self) -> None:
        assert len(DEFAULT_REJECT_TITLES) == 37

    def test_reject_titles_are_lowercase(self) -> None:
        for title in DEFAULT_REJECT_TITLES:
            assert title == title.lower(), f"Expected lowercase: {title}"

    def test_email_filter_patterns_count(self) -> None:
        assert len(DEFAULT_EMAIL_FILTER_PATTERNS) == 6

    def test_email_filter_patterns_have_prefix(self) -> None:
        for pat in DEFAULT_EMAIL_FILTER_PATTERNS:
            assert pat.startswith("starts_with:") or pat.startswith("contains:"), (
                f"Missing prefix: {pat}"
            )

    def test_stop_words_is_frozenset(self) -> None:
        assert isinstance(COMMON_STOP_WORDS, frozenset)
        assert len(COMMON_STOP_WORDS) >= 60

    def test_llm_cleanup_patterns_count(self) -> None:
        assert len(LLM_CLEANUP_PATTERNS) == 3

    def test_email_prompt_has_placeholders(self) -> None:
        for placeholder in ("{context}", "{job_title}", "{company}", "{min_words}"):
            assert placeholder in OPENROUTER_EMAIL_PROMPT

    def test_footer_has_placeholders(self) -> None:
        for placeholder in ("{contact_name}", "{contact_phone}", "{contact_portfolio}"):
            assert placeholder in CONTACT_INFO_FOOTER
            assert placeholder in CONTACT_INFO_FOOTER_WITH_RESUME

    def test_footer_with_resume_has_resume_link(self) -> None:
        assert "{resume_drive_link}" in CONTACT_INFO_FOOTER_WITH_RESUME

    def test_default_closing_is_nonempty(self) -> None:
        assert len(DEFAULT_CLOSING) > 50


# ===================================================================
# settings.py — loading from env
# ===================================================================


class TestSettingsFromEnv:
    """Test that Settings correctly loads from environment variables."""

    def test_loads_smtp_settings(self, settings: Settings) -> None:
        assert settings.gmail_email == "test@example.com"
        assert settings.gmail_app_password == "test-app-password"
        assert settings.smtp_host == "smtp.gmail.com"
        assert settings.smtp_port == 587

    def test_loads_contact_info(self, settings: Settings) -> None:
        assert settings.contact_name == "Test User"
        assert settings.contact_phone == "+1-555-0100"

    def test_loads_storage_backend(self, settings: Settings) -> None:
        assert settings.storage_backend == "csv"
        assert settings.csv_file_path == "test_sent.csv"

    def test_loads_general_settings(self, settings: Settings) -> None:
        assert settings.dry_run is True
        assert settings.log_level == "DEBUG"

    def test_loads_email_settings(self, settings: Settings) -> None:
        assert settings.email_interval_seconds == 5
        assert settings.application_sender_name == "Test User"


# ===================================================================
# CSV list parsing
# ===================================================================


class TestCsvListParsing:
    """Verify comma-separated ENV strings become Python lists."""

    def test_onsite_search_terms_parsed(self, settings: Settings) -> None:
        assert settings.onsite_search_terms == [
            "python developer",
            "backend engineer",
        ]

    def test_onsite_locations_parsed(self, settings: Settings) -> None:
        assert settings.onsite_locations == ["TestCity", "OtherCity"]

    def test_onsite_job_boards_parsed(self, settings: Settings) -> None:
        assert settings.onsite_job_boards == ["indeed", "google"]

    def test_remote_search_terms_parsed(self, settings: Settings) -> None:
        assert settings.remote_search_terms == ["remote python", "remote ml"]

    def test_single_value_csv(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A single value (no comma) should still produce a one-item list."""
        monkeypatch.setenv("ONSITE_SEARCH_TERMS", "python")
        monkeypatch.setenv("STORAGE_BACKEND", "csv")
        get_settings.cache_clear()
        s = Settings()
        assert s.onsite_search_terms == ["python"]
        get_settings.cache_clear()

    def test_empty_csv_returns_empty_list(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Empty string → empty list (env_ignore_empty fills default)."""
        monkeypatch.setenv("PROXY_LIST", "")
        monkeypatch.setenv("STORAGE_BACKEND", "csv")
        get_settings.cache_clear()
        s = Settings()
        assert s.proxy_list == []
        get_settings.cache_clear()

    def test_csv_strips_whitespace(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Spaces around commas should be stripped."""
        monkeypatch.setenv("ONSITE_LOCATIONS", "  City A , City B  ,  City C  ")
        monkeypatch.setenv("STORAGE_BACKEND", "csv")
        get_settings.cache_clear()
        s = Settings()
        assert s.onsite_locations == ["City A", "City B", "City C"]
        get_settings.cache_clear()


# ===================================================================
# Default fallback behavior
# ===================================================================


class TestDefaultFallbacks:
    """When ENV is empty, built-in defaults from defaults.py apply."""

    def test_empty_reject_titles_gets_defaults(self, settings: Settings) -> None:
        """The env_vars fixture does NOT set REJECT_TITLES → defaults apply."""
        assert len(settings.reject_titles) == 37
        assert "teacher" in settings.reject_titles
        assert "mechanic" in settings.reject_titles

    def test_custom_reject_titles_override(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Setting REJECT_TITLES in ENV overrides defaults completely."""
        monkeypatch.setenv("REJECT_TITLES", "spam,scam")
        monkeypatch.setenv("STORAGE_BACKEND", "csv")
        get_settings.cache_clear()
        s = Settings()
        assert s.reject_titles == ["spam", "scam"]
        assert "teacher" not in s.reject_titles
        get_settings.cache_clear()

    def test_empty_email_filter_patterns_gets_defaults(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """No EMAIL_FILTER_PATTERNS in ENV → defaults apply."""
        monkeypatch.delenv("EMAIL_FILTER_PATTERNS", raising=False)
        monkeypatch.setenv("STORAGE_BACKEND", "csv")
        get_settings.cache_clear()
        s = Settings()
        assert len(s.email_filter_patterns) == 6
        assert "contains:noreply" in s.email_filter_patterns
        get_settings.cache_clear()


# ===================================================================
# Type coercion
# ===================================================================


class TestTypeCoercion:
    """Verify string env vars are coerced to correct Python types."""

    def test_bool_true(self, settings: Settings) -> None:
        assert settings.dry_run is True

    def test_bool_false_default(self, settings: Settings) -> None:
        assert settings.scheduler_enabled is False

    def test_int_coercion(self, settings: Settings) -> None:
        assert settings.onsite_results_wanted == 10
        assert isinstance(settings.onsite_results_wanted, int)

    def test_email_interval_int(self, settings: Settings) -> None:
        assert settings.email_interval_seconds == 5
        assert isinstance(settings.email_interval_seconds, int)


# ===================================================================
# Singleton (get_settings)
# ===================================================================


class TestGetSettingsSingleton:
    """The get_settings() function should return a cached singleton."""

    def test_same_instance_returned(self, env_vars: dict[str, str]) -> None:
        get_settings.cache_clear()
        s1 = get_settings()
        s2 = get_settings()
        assert s1 is s2
        get_settings.cache_clear()

    def test_cache_clear_creates_new_instance(self, env_vars: dict[str, str]) -> None:
        get_settings.cache_clear()
        s1 = get_settings()
        get_settings.cache_clear()
        s2 = get_settings()
        assert s1 is not s2
        get_settings.cache_clear()


# ===================================================================
# Literal validation
# ===================================================================


class TestLiteralValidation:
    """Literal-typed fields reject invalid values."""

    def test_invalid_storage_backend_raises(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("STORAGE_BACKEND", "mongodb")
        get_settings.cache_clear()
        with pytest.raises(Exception):  # noqa: B017 — ValidationError
            Settings()
        get_settings.cache_clear()

    def test_invalid_email_generator_mode_raises(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("EMAIL_GENERATOR_MODE", "magic")
        monkeypatch.setenv("STORAGE_BACKEND", "csv")
        get_settings.cache_clear()
        with pytest.raises(Exception):  # noqa: B017 — ValidationError
            Settings()
        get_settings.cache_clear()
