"""Storage backend protocol — defines the contract all backends must implement."""

from __future__ import annotations

from typing import Protocol


# Column definitions for each data table
SENT_EMAIL_COLUMNS: tuple[str, ...] = (
    "email",
    "domain",
    "company",
    "date_sent",
    "job_title",
    "job_url",
    "location",
    "is_remote",
)

SCRAPED_JOB_COLUMNS: tuple[str, ...] = (
    "title",
    "company",
    "location",
    "job_url",
    "email",
    "date_scraped",
    "board",
    "is_remote",
)

RUN_STATS_COLUMNS: tuple[str, ...] = (
    "date",
    "mode",
    "total_scraped",
    "emails_found",
    "emails_sent",
    "emails_failed",
    "duration_seconds",
)


class StorageBackend(Protocol):
    """Protocol for storage backends (Google Sheets, CSV, etc.)."""

    def get_sent_emails(self) -> list[dict[str, str]]:
        """Return all previously sent email records."""
        ...

    def add_sent_email(self, record: dict[str, str]) -> None:
        """Append a single sent email record."""
        ...

    def add_scraped_jobs(self, records: list[dict[str, str]]) -> None:
        """Append a batch of scraped job records."""
        ...

    def get_run_stats(self) -> list[dict[str, str]]:
        """Return all run statistics records."""
        ...

    def add_run_stats(self, stats: dict[str, str]) -> None:
        """Append a single run statistics record."""
        ...
