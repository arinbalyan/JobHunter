"""Storage package — factory for backend selection."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from jobspy_v2.storage.base import (
    RUN_STATS_COLUMNS,
    SCRAPED_JOB_COLUMNS,
    SENT_EMAIL_COLUMNS,
    StorageBackend,
)

if TYPE_CHECKING:
    from jobspy_v2.config.settings import Settings

logger = logging.getLogger(__name__)

__all__ = [
    "RUN_STATS_COLUMNS",
    "SCRAPED_JOB_COLUMNS",
    "SENT_EMAIL_COLUMNS",
    "StorageBackend",
    "create_storage_backend",
]


def create_storage_backend(settings: Settings) -> StorageBackend:
    """Create the appropriate storage backend based on settings.

    Returns SheetsBackend when STORAGE_BACKEND=sheets and credentials exist,
    otherwise falls back to CsvBackend.
    """
    if settings.storage_backend == "sheets" and settings.google_credentials_json:
        try:
            from jobspy_v2.storage.sheets_backend import SheetsBackend

            backend = SheetsBackend(
                credentials_json=settings.google_credentials_json,
                sheet_name=settings.google_sheet_name,
            )
            logger.info("Using Google Sheets storage backend")
            return backend
        except Exception:
            logger.warning(
                "Failed to initialize Google Sheets, falling back to CSV",
                exc_info=True,
            )

    from jobspy_v2.storage.csv_backend import CsvBackend

    backend = CsvBackend(base_dir=".")
    logger.info("Using CSV storage backend (file: %s)", settings.csv_file_path)
    return backend
