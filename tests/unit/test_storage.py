"""Tests for the storage layer — CsvBackend, SheetsBackend factory, Protocol."""

from __future__ import annotations

import csv
import json
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from jobspy_v2.storage.base import (
    RUN_STATS_COLUMNS,
    SCRAPED_JOB_COLUMNS,
    SENT_EMAIL_COLUMNS,
)
from jobspy_v2.storage.csv_backend import CsvBackend

# ---------------------------------------------------------------------------
# Column schema tests
# ---------------------------------------------------------------------------


class TestColumnSchemas:
    """Verify column tuple definitions are correct."""

    def test_sent_email_columns_count(self) -> None:
        assert len(SENT_EMAIL_COLUMNS) == 12

    def test_sent_email_columns_has_email(self) -> None:
        assert "email" in SENT_EMAIL_COLUMNS

    def test_sent_email_columns_has_date_sent(self) -> None:
        assert "date_sent" in SENT_EMAIL_COLUMNS

    def test_scraped_job_columns_count(self) -> None:
        assert len(SCRAPED_JOB_COLUMNS) == 23

    def test_scraped_job_columns_has_title(self) -> None:
        assert "title" in SCRAPED_JOB_COLUMNS

    def test_run_stats_columns_count(self) -> None:
        assert len(RUN_STATS_COLUMNS) == 19

    def test_run_stats_columns_has_mode(self) -> None:
        assert "mode" in RUN_STATS_COLUMNS

    def test_all_columns_are_strings(self) -> None:
        for col in (*SENT_EMAIL_COLUMNS, *SCRAPED_JOB_COLUMNS, *RUN_STATS_COLUMNS):
            assert isinstance(col, str)


# ---------------------------------------------------------------------------
# CSV Backend tests
# ---------------------------------------------------------------------------


@pytest.fixture()
def csv_backend(tmp_path: Path) -> CsvBackend:
    """Create a CsvBackend rooted in a temp directory."""
    return CsvBackend(base_dir=tmp_path)


@pytest.fixture()
def sample_sent_email() -> dict[str, str]:
    return {
        "email": "hr@example.com",
        "domain": "example.com",
        "company": "Example Inc",
        "date_sent": "2026-02-14",
        "job_title": "ML Engineer",
        "job_url": "https://example.com/job/1",
        "location": "Bengaluru",
        "is_remote": "false",
        "subject": "Application for ML Engineer",
        "body_preview": "I am interested in the ML Engineer role...",
        "mode": "fallback",
        "word_count": "150",
    }


@pytest.fixture()
def sample_scraped_jobs() -> list[dict[str, str]]:
    return [
        {
            "date_scraped": "2026-02-14",
            "board": "indeed",
            "title": "Backend Developer",
            "company": "Acme Corp",
            "company_url": "https://acme.com",
            "location": "Remote",
            "is_remote": "true",
            "job_url": "https://acme.com/job/1",
            "job_type": "fulltime",
            "date_posted": "2026-02-13",
            "emails": "jobs@acme.com",
            "salary_min": "80000",
            "salary_max": "120000",
            "salary_currency": "INR",
            "salary_interval": "yearly",
            "skills": "Python, Django",
            "experience_range": "2-5",
            "job_level": "mid",
            "company_industry": "Technology",
            "email_sent": "Pending",
            "skip_reason": "",
            "email_recipient": "",
        },
        {
            "date_scraped": "2026-02-14",
            "board": "linkedin",
            "title": "Data Scientist",
            "company": "DataCo",
            "company_url": "https://dataco.com",
            "location": "Hyderabad",
            "is_remote": "false",
            "job_url": "https://dataco.com/job/2",
            "job_type": "fulltime",
            "date_posted": "2026-02-12",
            "emails": "careers@dataco.com",
            "salary_min": "",
            "salary_max": "",
            "salary_currency": "",
            "salary_interval": "",
            "skills": "ML, Python",
            "experience_range": "3-7",
            "job_level": "senior",
            "company_industry": "Data Analytics",
            "email_sent": "Pending",
            "skip_reason": "",
            "email_recipient": "",
        },
    ]


@pytest.fixture()
def sample_run_stats() -> dict[str, str]:
    return {
        "date": "2026-02-14",
        "mode": "onsite",
        "total_scraped": "150",
        "jobs_with_emails": "42",
        "emails_sent": "38",
        "emails_failed": "4",
        "skipped_dedup_exact": "2",
        "skipped_dedup_domain": "1",
        "skipped_dedup_company": "0",
        "skipped_no_recipients": "3",
        "filtered_title": "10",
        "filtered_email": "5",
        "boards_queried": "indeed,linkedin",
        "duration_seconds": "345",
        "dry_run": "False",
    }


class TestCsvBackendInit:
    """Test CSV backend initialization and file creation."""

    def test_creates_sent_emails_csv_on_first_read(
        self, csv_backend: CsvBackend, tmp_path: Path
    ) -> None:
        csv_backend.get_sent_emails()
        assert (tmp_path / "sent_emails.csv").exists()

    def test_creates_csv_with_correct_headers(
        self, csv_backend: CsvBackend, tmp_path: Path
    ) -> None:
        csv_backend.get_sent_emails()
        with (tmp_path / "sent_emails.csv").open() as f:
            reader = csv.reader(f)
            headers = next(reader)
        assert tuple(headers) == SENT_EMAIL_COLUMNS

    def test_empty_backend_returns_empty_list(self, csv_backend: CsvBackend) -> None:
        assert csv_backend.get_sent_emails() == []


class TestCsvBackendSentEmails:
    """Test sent email CRUD operations."""

    def test_add_and_retrieve_sent_email(
        self,
        csv_backend: CsvBackend,
        sample_sent_email: dict[str, str],
    ) -> None:
        csv_backend.add_sent_email(sample_sent_email)
        records = csv_backend.get_sent_emails()
        assert len(records) == 1
        assert records[0]["email"] == "hr@example.com"

    def test_add_multiple_sent_emails(
        self,
        csv_backend: CsvBackend,
        sample_sent_email: dict[str, str],
    ) -> None:
        csv_backend.add_sent_email(sample_sent_email)
        second = {**sample_sent_email, "email": "hr2@example.com"}
        csv_backend.add_sent_email(second)
        records = csv_backend.get_sent_emails()
        assert len(records) == 2

    def test_preserves_all_fields(
        self,
        csv_backend: CsvBackend,
        sample_sent_email: dict[str, str],
    ) -> None:
        csv_backend.add_sent_email(sample_sent_email)
        record = csv_backend.get_sent_emails()[0]
        for key, value in sample_sent_email.items():
            assert record[key] == value

    def test_ignores_extra_fields(
        self,
        csv_backend: CsvBackend,
        sample_sent_email: dict[str, str],
    ) -> None:
        extended = {**sample_sent_email, "extra_field": "should_be_ignored"}
        csv_backend.add_sent_email(extended)
        record = csv_backend.get_sent_emails()[0]
        assert "extra_field" not in record

    def test_missing_fields_default_to_empty(self, csv_backend: CsvBackend) -> None:
        csv_backend.add_sent_email({"email": "test@test.com"})
        record = csv_backend.get_sent_emails()[0]
        assert record["email"] == "test@test.com"
        assert record["domain"] == ""


class TestCsvBackendScrapedJobs:
    """Test scraped jobs batch operations."""

    def test_add_batch_creates_file(
        self,
        csv_backend: CsvBackend,
        sample_scraped_jobs: list[dict[str, str]],
        tmp_path: Path,
    ) -> None:
        csv_backend.add_scraped_jobs(sample_scraped_jobs)
        assert (tmp_path / "scraped_jobs.csv").exists()

    def test_add_batch_writes_all_records(
        self,
        csv_backend: CsvBackend,
        sample_scraped_jobs: list[dict[str, str]],
        tmp_path: Path,
    ) -> None:
        csv_backend.add_scraped_jobs(sample_scraped_jobs)
        with (tmp_path / "scraped_jobs.csv").open() as f:
            reader = csv.DictReader(f)
            rows = list(reader)
        assert len(rows) == 2

    def test_empty_batch_is_noop(self, csv_backend: CsvBackend, tmp_path: Path) -> None:
        start_row = csv_backend.add_scraped_jobs([])
        assert isinstance(start_row, int)
        # File may be created with headers, but no data rows
        csv_path = tmp_path / "scraped_jobs.csv"
        if csv_path.exists():
            with csv_path.open() as f:
                reader = csv.DictReader(f)
                rows = list(reader)
            assert len(rows) == 0


class TestCsvBackendScrapedJobsUpdate:
    """Test scraped job status update operations."""

    def test_add_scraped_jobs_returns_start_row(
        self,
        csv_backend: CsvBackend,
        sample_scraped_jobs: list[dict[str, str]],
    ) -> None:
        start_row = csv_backend.add_scraped_jobs(sample_scraped_jobs)
        assert isinstance(start_row, int)
        assert start_row == 2  # Row 1 is header, data starts at row 2

    def test_update_scraped_job_status(
        self,
        csv_backend: CsvBackend,
        sample_scraped_jobs: list[dict[str, str]],
        tmp_path: Path,
    ) -> None:
        start_row = csv_backend.add_scraped_jobs(sample_scraped_jobs)
        csv_backend.update_scraped_job_status(
            row_number=start_row,
            email_sent="Yes",
            skip_reason="",
            email_recipient="jobs@acme.com",
        )
        with (tmp_path / "scraped_jobs.csv").open() as f:
            reader = csv.DictReader(f)
            rows = list(reader)
        assert rows[0]["email_sent"] == "Yes"
        assert rows[0]["email_recipient"] == "jobs@acme.com"
        # Second row should be unchanged
        assert rows[1]["email_sent"] == "Pending"

    def test_update_scraped_job_status_out_of_range(
        self,
        csv_backend: CsvBackend,
        sample_scraped_jobs: list[dict[str, str]],
    ) -> None:
        csv_backend.add_scraped_jobs(sample_scraped_jobs)
        # Should not crash on out-of-range row number
        csv_backend.update_scraped_job_status(
            row_number=999,
            email_sent="Yes",
            skip_reason="",
            email_recipient="nobody@example.com",
        )


class TestCsvBackendRunStats:
    """Test run statistics recording."""

    def test_add_run_stats_creates_file(
        self,
        csv_backend: CsvBackend,
        sample_run_stats: dict[str, str],
        tmp_path: Path,
    ) -> None:
        csv_backend.add_run_stats(sample_run_stats)
        assert (tmp_path / "run_stats.csv").exists()

    def test_add_run_stats_preserves_values(
        self,
        csv_backend: CsvBackend,
        sample_run_stats: dict[str, str],
        tmp_path: Path,
    ) -> None:
        csv_backend.add_run_stats(sample_run_stats)
        with (tmp_path / "run_stats.csv").open() as f:
            reader = csv.DictReader(f)
            row = next(reader)
        assert row["mode"] == "onsite"
        assert row["emails_sent"] == "38"


class TestCsvBackendPersistence:
    """Test data persists across backend instances."""

    def test_data_persists_across_instances(
        self,
        tmp_path: Path,
        sample_sent_email: dict[str, str],
    ) -> None:
        backend1 = CsvBackend(base_dir=tmp_path)
        backend1.add_sent_email(sample_sent_email)

        backend2 = CsvBackend(base_dir=tmp_path)
        records = backend2.get_sent_emails()
        assert len(records) == 1
        assert records[0]["email"] == "hr@example.com"


# ---------------------------------------------------------------------------
# Storage factory tests
# ---------------------------------------------------------------------------


class TestStorageFactory:
    """Test create_storage_backend factory function."""

    def test_csv_backend_when_storage_is_csv(self) -> None:
        from jobspy_v2.storage import create_storage_backend

        settings = MagicMock()
        settings.storage_backend = "csv"
        settings.csv_file_path = "sent_emails.csv"
        settings.google_credentials_json = None

        backend = create_storage_backend(settings)
        assert isinstance(backend, CsvBackend)

    def test_csv_backend_when_sheets_with_no_credentials(self) -> None:
        from jobspy_v2.storage import create_storage_backend

        settings = MagicMock()
        settings.storage_backend = "sheets"
        settings.google_credentials_json = ""
        settings.csv_file_path = "sent_emails.csv"

        backend = create_storage_backend(settings)
        assert isinstance(backend, CsvBackend)

    def test_csv_fallback_when_sheets_init_fails(self) -> None:
        from jobspy_v2.storage import create_storage_backend

        settings = MagicMock()
        settings.storage_backend = "sheets"
        settings.google_credentials_json = '{"invalid": "creds"}'
        settings.google_sheet_name = "Test"
        settings.csv_file_path = "sent_emails.csv"

        # SheetsBackend init will fail with invalid creds → fallback to CSV
        backend = create_storage_backend(settings)
        assert isinstance(backend, CsvBackend)

    @patch("jobspy_v2.storage.sheets_backend.gspread")
    def test_sheets_backend_when_configured(self, mock_gspread: MagicMock) -> None:
        from jobspy_v2.storage import create_storage_backend

        # Mock the gspread chain
        mock_gc = MagicMock()
        mock_gspread.service_account_from_dict.return_value = mock_gc
        mock_spreadsheet = MagicMock()
        mock_gc.open.return_value = mock_spreadsheet
        mock_ws = MagicMock()
        mock_spreadsheet.worksheet.return_value = mock_ws

        settings = MagicMock()
        settings.storage_backend = "sheets"
        settings.google_credentials_json = json.dumps({"type": "service_account"})
        settings.google_sheet_name = "Test Sheet"
        settings.csv_file_path = "sent_emails.csv"

        backend = create_storage_backend(settings)

        from jobspy_v2.storage.sheets_backend import SheetsBackend

        assert isinstance(backend, SheetsBackend)


# ---------------------------------------------------------------------------
# SheetsBackend credential parsing tests
# ---------------------------------------------------------------------------


class TestCredentialParsing:
    """Test _parse_credentials handles JSON and base64."""

    def test_raw_json_string(self) -> None:
        from jobspy_v2.storage.sheets_backend import _parse_credentials

        creds = '{"type": "service_account", "project_id": "test"}'
        result = _parse_credentials(creds)
        assert result["type"] == "service_account"

    def test_base64_encoded_json(self) -> None:
        import base64

        from jobspy_v2.storage.sheets_backend import _parse_credentials

        original = '{"type": "service_account"}'
        encoded = base64.b64encode(original.encode()).decode()
        result = _parse_credentials(encoded)
        assert result["type"] == "service_account"

    def test_invalid_credentials_raises(self) -> None:
        from jobspy_v2.storage.sheets_backend import _parse_credentials

        with pytest.raises(ValueError, match="must be valid JSON"):
            _parse_credentials("not-json-not-base64!!!")

    def test_whitespace_stripped(self) -> None:
        from jobspy_v2.storage.sheets_backend import _parse_credentials

        creds = '  {"type": "service_account"}  '
        result = _parse_credentials(creds)
        assert result["type"] == "service_account"
