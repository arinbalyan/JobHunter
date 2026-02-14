"""CSV storage backend — local file fallback when Google Sheets is unavailable."""

from __future__ import annotations

import csv
import logging
from pathlib import Path

from jobspy_v2.storage.base import (
    RUN_STATS_COLUMNS,
    SCRAPED_JOB_COLUMNS,
    SENT_EMAIL_COLUMNS,
)

logger = logging.getLogger(__name__)


def _ensure_csv(path: Path, columns: tuple[str, ...]) -> None:
    """Create the CSV file with headers if it doesn't exist."""
    if path.exists():
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as f:
        csv.DictWriter(f, fieldnames=columns).writeheader()
    logger.info("Created CSV file: %s", path)


def _read_csv(path: Path, columns: tuple[str, ...]) -> list[dict[str, str]]:
    """Read all rows from a CSV file as list of dicts."""
    _ensure_csv(path, columns)
    with path.open("r", newline="", encoding="utf-8") as f:
        return list(csv.DictReader(f))


def _append_rows(
    path: Path,
    columns: tuple[str, ...],
    rows: list[dict[str, str]],
) -> None:
    """Append rows to a CSV file, creating it if needed."""
    _ensure_csv(path, columns)
    with path.open("a", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=columns, extrasaction="ignore")
        for row in rows:
            writer.writerow(row)


class CsvBackend:
    """CSV-based storage backend with three separate files."""

    def __init__(self, base_dir: str | Path = ".") -> None:
        self._base = Path(base_dir)
        self._sent_path = self._base / "sent_emails.csv"
        self._jobs_path = self._base / "scraped_jobs.csv"
        self._stats_path = self._base / "run_stats.csv"

    def get_sent_emails(self) -> list[dict[str, str]]:
        """Return all previously sent email records."""
        return _read_csv(self._sent_path, SENT_EMAIL_COLUMNS)

    def add_sent_email(self, record: dict[str, str]) -> None:
        """Append a single sent email record."""
        _append_rows(self._sent_path, SENT_EMAIL_COLUMNS, [record])
        logger.debug("Recorded sent email: %s", record.get("email", "?"))

    def add_scraped_jobs(self, records: list[dict[str, str]]) -> None:
        """Append a batch of scraped job records."""
        if not records:
            return
        _append_rows(self._jobs_path, SCRAPED_JOB_COLUMNS, records)
        logger.info("Saved %d scraped jobs to CSV", len(records))

    def add_run_stats(self, stats: dict[str, str]) -> None:
        """Append a single run statistics record."""
        _append_rows(self._stats_path, RUN_STATS_COLUMNS, [stats])
        logger.info("Saved run stats for %s", stats.get("date", "?"))
