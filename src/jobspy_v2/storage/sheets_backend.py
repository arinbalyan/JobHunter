"""Google Sheets storage backend — persistent cloud storage via gspread."""

from __future__ import annotations

import base64
import json
import logging
from typing import Any

import gspread
from gspread import BackOffHTTPClient, Spreadsheet, Worksheet

from jobspy_v2.storage.base import (
    RUN_STATS_COLUMNS,
    SCRAPED_JOB_COLUMNS,
    SENT_EMAIL_COLUMNS,
)

logger = logging.getLogger(__name__)

# Worksheet names and their column schemas
_WORKSHEETS: dict[str, tuple[str, ...]] = {
    "Sent Emails": SENT_EMAIL_COLUMNS,
    "Scraped Jobs": SCRAPED_JOB_COLUMNS,
    "Run Stats": RUN_STATS_COLUMNS,
}


def _parse_credentials(raw: str) -> dict[str, Any]:
    """Parse credentials from raw JSON string or base64-encoded JSON."""
    stripped = raw.strip()

    # Try raw JSON first
    if stripped.startswith("{"):
        return json.loads(stripped)

    # Try base64 decoding
    try:
        decoded = base64.b64decode(stripped).decode("utf-8")
        return json.loads(decoded)
    except Exception as exc:
        msg = "GOOGLE_CREDENTIALS_JSON must be valid JSON or base64-encoded JSON"
        raise ValueError(msg) from exc


def _get_or_create_worksheet(
    spreadsheet: Spreadsheet,
    title: str,
    columns: tuple[str, ...],
) -> Worksheet:
    """Get an existing worksheet or create one with headers."""
    try:
        return spreadsheet.worksheet(title)
    except gspread.WorksheetNotFound:
        ws = spreadsheet.add_worksheet(title=title, rows=1000, cols=len(columns))
        ws.append_row(list(columns), value_input_option="USER_ENTERED")
        logger.info("Created worksheet: %s", title)
        return ws


class SheetsBackend:
    """Google Sheets storage backend with auto-retry on rate limits."""

    def __init__(
        self,
        credentials_json: str,
        sheet_name: str = "JobSpy Data",
    ) -> None:
        creds = _parse_credentials(credentials_json)
        self._gc = gspread.service_account_from_dict(
            creds,
            http_client=BackOffHTTPClient,
        )
        self._spreadsheet = self._open_or_create(sheet_name)
        self._worksheets: dict[str, Worksheet] = {}

        # Initialize all worksheets
        for ws_title, columns in _WORKSHEETS.items():
            self._worksheets[ws_title] = _get_or_create_worksheet(
                self._spreadsheet, ws_title, columns
            )

    def _open_or_create(self, name: str) -> Spreadsheet:
        """Open a spreadsheet by name, or create it if not found."""
        try:
            spreadsheet = self._gc.open(name)
            logger.info("Opened spreadsheet: %s", name)
            return spreadsheet
        except gspread.SpreadsheetNotFound:
            spreadsheet = self._gc.create(name)
            logger.info("Created spreadsheet: %s", name)
            return spreadsheet

    def _get_ws(self, name: str) -> Worksheet:
        """Retrieve a worksheet by its display name."""
        return self._worksheets[name]

    def _record_to_row(
        self,
        record: dict[str, str],
        columns: tuple[str, ...],
    ) -> list[str]:
        """Convert a dict record to an ordered list matching column schema."""
        return [record.get(col, "") for col in columns]

    def get_sent_emails(self) -> list[dict[str, str]]:
        """Return all previously sent email records from the sheet."""
        ws = self._get_ws("Sent Emails")
        try:
            records = ws.get_all_records()
            return [{str(k): str(v) for k, v in row.items()} for row in records]
        except gspread.exceptions.GSpreadException:
            logger.warning("Failed to read sent emails, returning empty list")
            return []

    def add_sent_email(self, record: dict[str, str]) -> None:
        """Append a single sent email record to the sheet."""
        ws = self._get_ws("Sent Emails")
        row = self._record_to_row(record, SENT_EMAIL_COLUMNS)
        ws.append_row(row, value_input_option="USER_ENTERED")
        logger.debug("Recorded sent email: %s", record.get("email", "?"))

    def add_scraped_jobs(self, records: list[dict[str, str]]) -> None:
        """Append a batch of scraped job records to the sheet."""
        if not records:
            return
        ws = self._get_ws("Scraped Jobs")
        rows = [self._record_to_row(r, SCRAPED_JOB_COLUMNS) for r in records]
        ws.append_rows(rows, value_input_option="USER_ENTERED")
        logger.info("Saved %d scraped jobs to Sheets", len(records))

    def get_run_stats(self) -> list[dict[str, str]]:
        """Return all run statistics records from the sheet."""
        ws = self._get_ws("Run Stats")
        try:
            records = ws.get_all_records()
            return [{str(k): str(v) for k, v in row.items()} for row in records]
        except gspread.exceptions.GSpreadException:
            logger.warning("Failed to read run stats, returning empty list")
            return []

    def add_run_stats(self, stats: dict[str, str]) -> None:
        """Append a single run statistics record to the sheet."""
        ws = self._get_ws("Run Stats")
        row = self._record_to_row(stats, RUN_STATS_COLUMNS)
        ws.append_row(row, value_input_option="USER_ENTERED")
        logger.info("Saved run stats for %s", stats.get("date", "?"))
