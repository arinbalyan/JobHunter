"""Tests for core modules: scraper, dedup, email_gen, email_sender, reporter."""

from __future__ import annotations

import smtplib
from datetime import date, timedelta
from pathlib import Path
from unittest.mock import MagicMock, Mock, patch, call

import pandas as pd
import pytest

from jobspy_v2.config.settings import Settings
from jobspy_v2.core.dedup import Deduplicator
from jobspy_v2.core.email_gen import EmailResult, generate_email
from jobspy_v2.core.email_sender import send_email
from jobspy_v2.core.reporter import send_report
from jobspy_v2.core.scraper import (
    ScrapeResult,
    _adapt_params_for_board,
    _build_base_params,
    scrape_jobs,
)
from jobspy_v2.storage.csv_backend import CsvBackend


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture()
def settings(env_vars: None) -> Settings:
    """Create Settings from env_vars fixture (conftest.py)."""
    from jobspy_v2.config.settings import get_settings

    get_settings.cache_clear()
    s = Settings(_env_file=None)
    yield s
    get_settings.cache_clear()


@pytest.fixture()
def csv_storage(tmp_path: Path) -> CsvBackend:
    """Create a temporary CSV storage backend."""
    return CsvBackend(base_dir=tmp_path)


@pytest.fixture()
def dedup(csv_storage: CsvBackend) -> Deduplicator:
    """Create a Deduplicator backed by CSV storage."""
    return Deduplicator(storage=csv_storage)


# ---------------------------------------------------------------------------
# Scraper: param building
# ---------------------------------------------------------------------------


class TestBuildBaseParams:
    """Tests for _build_base_params."""

    def test_onsite_mode_uses_onsite_settings(self, settings: Settings) -> None:
        params = _build_base_params(settings, mode="onsite")
        assert params["results_wanted"] == settings.onsite_results_wanted
        assert params["job_type"] == settings.onsite_job_type
        assert params["country_indeed"] == settings.onsite_country_indeed

    def test_remote_mode_uses_remote_settings(self, settings: Settings) -> None:
        params = _build_base_params(settings, mode="remote")
        assert params["results_wanted"] == settings.remote_results_wanted
        assert params["is_remote"] is True
        assert params["country_indeed"] == settings.remote_country_indeed

    def test_verbose_is_two(self, settings: Settings) -> None:
        params = _build_base_params(settings, mode="onsite")
        assert params["verbose"] == 2

    def test_proxy_list_included_when_set(self, settings: Settings) -> None:
        settings.proxy_list = ["http://proxy1:8080", "http://proxy2:8080"]
        params = _build_base_params(settings, mode="onsite")
        assert params["proxies"] == settings.proxy_list


class TestAdaptParamsForBoard:
    """Tests for _adapt_params_for_board (Indeed/Google/LinkedIn limitations)."""

    def test_indeed_drops_conflicting_params(self) -> None:
        params = {
            "hours_old": 72,
            "job_type": "fulltime",
            "is_remote": True,
            "easy_apply": True,
        }
        adapted = _adapt_params_for_board(params.copy(), "indeed")
        # Indeed: keeps job_type+is_remote, drops hours_old and easy_apply
        assert "hours_old" not in adapted
        assert "easy_apply" not in adapted
        assert adapted["job_type"] == "fulltime"
        assert adapted["is_remote"] is True

    def test_google_uses_google_search_term(self) -> None:
        params = {"search_term": "ML engineer", "location": "Bengaluru"}
        adapted = _adapt_params_for_board(params.copy(), "google")
        assert "google_search_term" in adapted

    def test_non_indeed_keeps_all_params(self) -> None:
        params = {"hours_old": 72, "job_type": "fulltime", "is_remote": False}
        adapted = _adapt_params_for_board(params.copy(), "glassdoor")
        assert adapted["hours_old"] == 72
        assert adapted["job_type"] == "fulltime"


class TestScrapeJobs:
    """Tests for scrape_jobs (integration with mocked jobspy)."""

    @patch("jobspy_v2.core.scraper.jobspy_scrape")
    def test_returns_scrape_result(self, mock_scrape: Mock, settings: Settings) -> None:
        mock_scrape.return_value = pd.DataFrame(
            {
                "title": ["ML Engineer"],
                "company": ["TechCo"],
                "job_url": ["https://example.com/job1"],
                "location": ["Bengaluru"],
                "emails": ["hr@techco.com"],
                "description": ["Build ML models"],
                "is_remote": [False],
            }
        )
        result = scrape_jobs(settings, mode="onsite")
        assert isinstance(result, ScrapeResult)
        assert len(result.jobs) >= 0  # May be filtered

    @patch("jobspy_v2.core.scraper.jobspy_scrape")
    def test_filters_rejected_titles(
        self, mock_scrape: Mock, settings: Settings
    ) -> None:
        mock_scrape.return_value = pd.DataFrame(
            {
                "title": ["Teacher", "ML Engineer"],
                "company": ["School", "TechCo"],
                "job_url": [
                    "https://example.com/j1",
                    "https://example.com/j2",
                ],
                "location": ["Delhi", "Bengaluru"],
                "emails": ["hr@school.com", "hr@techco.com"],
                "description": ["Teach math", "Build ML"],
                "is_remote": [False, False],
            }
        )
        result = scrape_jobs(settings, mode="onsite")
        titles = [row.get("title", "") for _, row in result.jobs.iterrows()]
        assert "Teacher" not in titles

    @patch("jobspy_v2.core.scraper.jobspy_scrape")
    def test_filters_invalid_emails(
        self, mock_scrape: Mock, settings: Settings
    ) -> None:
        mock_scrape.return_value = pd.DataFrame(
            {
                "title": ["Dev1", "Dev2"],
                "company": ["A", "B"],
                "job_url": ["https://a.com/1", "https://b.com/2"],
                "location": ["Pune", "Pune"],
                "emails": ["", "valid@b.com"],
                "description": ["Desc1", "Desc2"],
                "is_remote": [False, False],
            }
        )
        result = scrape_jobs(settings, mode="onsite")
        # Only rows with valid emails should remain
        assert len(result.jobs) <= 1

    @patch("jobspy_v2.core.scraper.jobspy_scrape")
    def test_deduplicates_by_job_url(
        self, mock_scrape: Mock, settings: Settings
    ) -> None:
        mock_scrape.return_value = pd.DataFrame(
            {
                "title": ["Dev", "Dev"],
                "company": ["A", "A"],
                "job_url": ["https://a.com/1", "https://a.com/1"],
                "location": ["Pune", "Pune"],
                "emails": ["hr@a.com", "hr@a.com"],
                "description": ["Same", "Same"],
                "is_remote": [False, False],
            }
        )
        result = scrape_jobs(settings, mode="onsite")
        assert len(result.jobs) <= 1

    @patch("jobspy_v2.core.scraper.jobspy_scrape")
    def test_handles_scrape_exception(
        self, mock_scrape: Mock, settings: Settings
    ) -> None:
        mock_scrape.side_effect = Exception("Rate limited")
        result = scrape_jobs(settings, mode="onsite")
        # Should return empty result, not crash
        assert isinstance(result, ScrapeResult)
        assert len(result.jobs) == 0


# ---------------------------------------------------------------------------
# Dedup
# ---------------------------------------------------------------------------


class TestDeduplicator:
    """Tests for Deduplicator."""

    def test_new_email_can_send(self, dedup: Deduplicator) -> None:
        can, reason = dedup.can_send("new@example.com", "example.com", "Example")
        assert can is True
        assert reason == ""

    def test_same_email_rejected(self, dedup: Deduplicator) -> None:
        dedup.mark_sent(
            email="hr@acme.com",
            domain="acme.com",
            company="Acme",
            job_title="Dev",
            job_url="https://acme.com/j1",
            location="Delhi",
            is_remote=False,
        )
        can, reason = dedup.can_send("hr@acme.com", "acme.com", "Acme")
        assert can is False
        assert "already sent" in reason.lower()

    def test_domain_cooldown_30_days(self, dedup: Deduplicator) -> None:
        dedup.mark_sent(
            email="hr@acme.com",
            domain="acme.com",
            company="Acme",
            job_title="Dev",
            job_url="https://acme.com/j1",
            location="Delhi",
            is_remote=False,
        )
        # Different email, same domain — should be rejected
        can, reason = dedup.can_send("jobs@acme.com", "acme.com", "Acme2")
        assert can is False
        assert "domain" in reason.lower()

    def test_company_same_day_cooldown(self, dedup: Deduplicator) -> None:
        dedup.mark_sent(
            email="hr@acme.com",
            domain="acme.com",
            company="Acme Inc",
            job_title="Dev",
            job_url="https://acme.com/j1",
            location="Delhi",
            is_remote=False,
        )
        # Different domain, same company, same day
        can, reason = dedup.can_send("jobs@acme-inc.com", "acme-inc.com", "Acme Inc")
        assert can is False
        assert "company" in reason.lower()

    def test_different_everything_can_send(self, dedup: Deduplicator) -> None:
        dedup.mark_sent(
            email="hr@acme.com",
            domain="acme.com",
            company="Acme",
            job_title="Dev",
            job_url="https://acme.com/j1",
            location="Delhi",
            is_remote=False,
        )
        can, _ = dedup.can_send("hr@other.com", "other.com", "Other Corp")
        assert can is True


# ---------------------------------------------------------------------------
# Email Generator
# ---------------------------------------------------------------------------


class TestEmailGenerator:
    """Tests for generate_email."""

    def test_fallback_mode_generates_email(
        self, settings: Settings, tmp_path: Path
    ) -> None:
        settings.email_generator_mode = "fallback"
        settings.fallback_email_subject = "Test Subject {contact_name}"
        settings.fallback_email_body = (
            "Hello, I am {contact_name}. "
            "I have experience in software development. "
            "My phone is {contact_phone}. "
            "Check my portfolio at {contact_portfolio}. "
            "GitHub: {contact_github}. "
        )
        settings.contact_name = "Arin"
        settings.contact_phone = "+91 123"
        settings.contact_portfolio = "https://example.com"
        settings.contact_github = "https://github.com/test"
        settings.min_email_words = 5  # Low threshold for test

        result = generate_email(
            settings=settings,
            job_description="Build ML models at TechCo",
            job_title="ML Engineer",
            company="TechCo",
        )
        assert isinstance(result, EmailResult)
        assert result.subject != ""
        assert "Arin" in result.subject or "Arin" in result.body

    @patch("jobspy_v2.core.email_gen.OpenAI")
    def test_llm_mode_parses_subject(
        self, mock_openai_cls: Mock, settings: Settings, tmp_path: Path
    ) -> None:
        settings.email_generator_mode = "llm"
        settings.openrouter_api_key = "test-key"
        settings.min_email_words = 5
        settings.max_email_words = 500

        # Create context file
        ctx = tmp_path / "profile.md"
        ctx.write_text("I am a developer with ML experience.")
        settings.context_file_path = str(ctx)

        # Mock OpenAI response
        mock_client = MagicMock()
        mock_openai_cls.return_value = mock_client
        mock_response = MagicMock()
        mock_response.choices = [MagicMock()]
        mock_response.choices[
            0
        ].message.content = "SUBJECT: ML Role Application\n\nI am writing to express my interest in the ML Engineer position at TechCo. I have extensive experience building machine learning systems and deploying them to production. My background includes deep learning, NLP, and computer vision applications."
        mock_client.chat.completions.create.return_value = mock_response

        result = generate_email(
            settings=settings,
            job_description="Build ML models at TechCo",
            job_title="ML Engineer",
            company="TechCo",
        )
        assert result.subject == "ML Role Application"
        assert "ML Engineer" in result.body or "interest" in result.body

    @patch("jobspy_v2.core.email_gen.OpenAI")
    def test_llm_failure_falls_back(
        self, mock_openai_cls: Mock, settings: Settings, tmp_path: Path
    ) -> None:
        settings.email_generator_mode = "llm"
        settings.openrouter_api_key = "test-key"
        settings.min_email_words = 5
        settings.fallback_email_subject = "Fallback Subject"
        settings.fallback_email_body = "Fallback body with enough words to pass the minimum word count requirement for the email generator."

        ctx = tmp_path / "profile.md"
        ctx.write_text("Developer profile")
        settings.context_file_path = str(ctx)

        # Make LLM fail
        mock_client = MagicMock()
        mock_openai_cls.return_value = mock_client
        mock_client.chat.completions.create.side_effect = Exception("API error")

        result = generate_email(
            settings=settings,
            job_description="Build things",
            job_title="Dev",
            company="Co",
        )
        # Should fall back, not crash
        assert isinstance(result, EmailResult)
        assert result.subject != ""

    def test_email_result_is_frozen(self) -> None:
        r = EmailResult(subject="Test", body="Body text", mode="test", word_count=2)
        with pytest.raises(AttributeError):
            r.subject = "Changed"  # type: ignore[misc]


# ---------------------------------------------------------------------------
# Email Sender
# ---------------------------------------------------------------------------


class TestEmailSender:
    """Tests for send_email."""

    @patch("jobspy_v2.core.email_sender.smtplib.SMTP")
    def test_successful_send(self, mock_smtp_cls: Mock, settings: Settings) -> None:
        mock_smtp = MagicMock()
        mock_smtp_cls.return_value.__enter__ = Mock(return_value=mock_smtp)
        mock_smtp_cls.return_value.__exit__ = Mock(return_value=False)

        success, error = send_email(
            settings=settings,
            to_email="recipient@example.com",
            subject="Test",
            body="Hello world",
        )
        assert success is True
        assert error == ""

    @patch("jobspy_v2.core.email_sender.smtplib.SMTP")
    def test_auth_error_no_retry(self, mock_smtp_cls: Mock, settings: Settings) -> None:
        mock_smtp = MagicMock()
        mock_smtp.login.side_effect = smtplib.SMTPAuthenticationError(
            535, b"Auth failed"
        )
        mock_smtp_cls.return_value.__enter__ = Mock(return_value=mock_smtp)
        mock_smtp_cls.return_value.__exit__ = Mock(return_value=False)

        success, error = send_email(
            settings=settings,
            to_email="recipient@example.com",
            subject="Test",
            body="Hello world",
        )
        assert success is False
        assert "auth" in error.lower()
        # Should only try once (no retry on auth error)
        assert mock_smtp.login.call_count == 1

    @patch("jobspy_v2.core.email_sender.time.sleep")
    @patch("jobspy_v2.core.email_sender.smtplib.SMTP")
    def test_disconnect_retries(
        self, mock_smtp_cls: Mock, mock_sleep: Mock, settings: Settings
    ) -> None:
        mock_smtp = MagicMock()
        mock_smtp.send_message.side_effect = [
            smtplib.SMTPServerDisconnected("Lost connection"),
            smtplib.SMTPServerDisconnected("Lost connection"),
            None,  # Third attempt succeeds
        ]
        mock_smtp_cls.return_value.__enter__ = Mock(return_value=mock_smtp)
        mock_smtp_cls.return_value.__exit__ = Mock(return_value=False)

        success, error = send_email(
            settings=settings,
            to_email="recipient@example.com",
            subject="Test",
            body="Hello world",
        )
        assert success is True
        assert mock_sleep.call_count == 2  # Slept between retries


# ---------------------------------------------------------------------------
# Reporter
# ---------------------------------------------------------------------------


class TestReporter:
    """Tests for send_report."""

    @patch("jobspy_v2.core.reporter._send_report_email")
    def test_sends_report_email(
        self,
        mock_send: Mock,
        settings: Settings,
        csv_storage: CsvBackend,
    ) -> None:
        mock_send.return_value = (True, "")

        send_report(
            settings=settings,
            storage=csv_storage,
            mode="onsite",
            total_scraped=50,
            total_emailed=10,
            total_errors=2,
            duration_seconds=120.5,
        )
        mock_send.assert_called_once()
        call_kwargs = mock_send.call_args
        # Verify report email goes to report_email
        assert settings.report_email in str(call_kwargs)

    @patch("jobspy_v2.core.reporter._send_report_email")
    def test_records_run_stats(
        self,
        mock_send: Mock,
        settings: Settings,
        csv_storage: CsvBackend,
    ) -> None:
        mock_send.return_value = (True, "")

        send_report(
            settings=settings,
            storage=csv_storage,
            mode="onsite",
            total_scraped=50,
            total_emailed=10,
            total_errors=2,
            duration_seconds=120.5,
        )
        # Verify run stats were recorded
        stats = csv_storage.get_run_stats()
        assert len(stats) == 1
        assert stats[0]["mode"] == "onsite"
        assert stats[0]["total_scraped"] == "50"

    @patch("jobspy_v2.core.reporter._send_report_email")
    def test_report_does_not_crash_on_send_failure(
        self,
        mock_send: Mock,
        settings: Settings,
        csv_storage: CsvBackend,
    ) -> None:
        mock_send.return_value = (False, "SMTP error")

        # Should not raise
        send_report(
            settings=settings,
            storage=csv_storage,
            mode="remote",
            total_scraped=0,
            total_emailed=0,
            total_errors=0,
            duration_seconds=0,
        )
        # Stats should still be recorded even if email fails
        stats = csv_storage.get_run_stats()
        assert len(stats) == 1
