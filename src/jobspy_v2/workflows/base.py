"""Base workflow — shared pipeline logic for onsite and remote modes."""

from __future__ import annotations

import logging
import time
from datetime import datetime
from pathlib import Path
from typing import TYPE_CHECKING

from jobspy_v2.config import Settings
from jobspy_v2.core.dedup import Deduplicator
from jobspy_v2.core.email_gen import generate_email
from jobspy_v2.core.email_sender import send_email
from jobspy_v2.core.reporter import send_report
from jobspy_v2.core.scraper import scrape_jobs
from jobspy_v2.storage import create_storage_backend
from jobspy_v2.utils.email_utils import get_valid_recipients

if TYPE_CHECKING:
    from jobspy_v2.storage.base import StorageBackend

logger = logging.getLogger(__name__)

CONTEXT_DIR = Path("contexts")


class BaseWorkflow:
    """Template-method pipeline — subclasses only set ``mode``."""

    mode: str = ""  # "onsite" or "remote" — set by subclass

    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        self.storage: StorageBackend = create_storage_backend(settings)
        self.dedup = Deduplicator(self.storage)
        self._context: str = ""

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def run(self) -> int:
        """Execute the full pipeline. Returns 0 on success, 1 on fatal error."""
        start = time.monotonic()

        if self._should_skip_weekend():
            return 0

        self._context = self._load_context()

        stats = {
            "scraped": 0,
            "emailed": 0,
            "skipped": 0,
            "filtered": 0,
            "errors": 0,
            "boards": [],
        }

        try:
            result = scrape_jobs(self.settings, self.mode)
            stats["scraped"] = len(result.jobs)
            stats["boards"] = result.boards_queried

            if result.jobs.empty:
                logger.info("[%s] No jobs found after filtering.", self.mode)
                self._send_report(stats, start)
                return 0

            max_emails = self._get_max_emails()
            self._process_jobs(result.jobs, max_emails, stats)

        except Exception:
            logger.exception("[%s] Fatal error in pipeline.", self.mode)
            stats["errors"] += 1
            self._send_report(stats, start)
            return 1

        self._send_report(stats, start)
        return 0

    # ------------------------------------------------------------------
    # Internals
    # ------------------------------------------------------------------

    def _should_skip_weekend(self) -> bool:
        if self.settings.skip_weekends and datetime.now().weekday() >= 5:
            logger.info("[%s] Skipping — weekend.", self.mode)
            return True
        return False

    def _load_context(self) -> str:
        path = CONTEXT_DIR / "profile.md"
        try:
            return path.read_text(encoding="utf-8")
        except FileNotFoundError:
            logger.warning("Context file not found: %s — using empty context.", path)
            return ""

    def _get_max_emails(self) -> int:
        if self.mode == "onsite":
            return self.settings.onsite_max_emails_per_day
        return self.settings.remote_max_emails_per_day

    def _process_jobs(
        self,
        jobs: "import('pandas').DataFrame",
        max_emails: int,
        stats: dict,
    ) -> None:
        sent_count = 0

        for _, row in jobs.iterrows():
            if sent_count >= max_emails:
                logger.info("[%s] Reached max emails (%d).", self.mode, max_emails)
                break

            title = str(row.get("title", ""))
            company = str(row.get("company", ""))
            job_url = str(row.get("job_url", ""))
            description = str(row.get("description", ""))
            location = str(row.get("location", ""))
            is_remote = bool(row.get("is_remote", False))
            raw_emails = str(row.get("emails", ""))

            recipients = get_valid_recipients(
                raw_emails, self.settings.email_filter_patterns
            )
            if not recipients:
                stats["filtered"] += 1
                continue

            primary_email = recipients[0]
            domain = primary_email.split("@")[1] if "@" in primary_email else ""

            can_send, reason = self.dedup.can_send(primary_email, domain, company)
            if not can_send:
                logger.debug("Skipping %s: %s", primary_email, reason)
                stats["skipped"] += 1
                continue

            email_result = generate_email(
                job_title=title,
                company=company,
                job_description=description,
                settings=self.settings,
                context=self._context,
            )

            if self.settings.dry_run:
                logger.info(
                    "[DRY RUN] Would send to %s — %s at %s",
                    primary_email,
                    title,
                    company,
                )
            else:
                success, error = send_email(
                    to_email=primary_email,
                    subject=email_result.subject,
                    body=email_result.body,
                    settings=self.settings,
                    resume_path=self.settings.resume_file_path,
                )
                if not success:
                    logger.error("Failed to send to %s: %s", primary_email, error)
                    stats["errors"] += 1
                    continue

            self.dedup.mark_sent(
                email=primary_email,
                domain=domain,
                company=company,
                job_title=title,
                job_url=job_url,
                location=location,
                is_remote=is_remote,
            )
            sent_count += 1
            stats["emailed"] += 1

            if sent_count < max_emails and not self.settings.dry_run:
                time.sleep(self.settings.email_interval_seconds)

    def _send_report(self, stats: dict, start: float) -> None:
        duration = time.monotonic() - start
        send_report(
            settings=self.settings,
            storage=self.storage,
            mode=self.mode,
            total_scraped=stats["scraped"],
            total_emailed=stats["emailed"],
            total_errors=stats["errors"],
            total_skipped=stats["skipped"],
            total_filtered=stats["filtered"],
            duration_seconds=duration,
            boards_queried=stats["boards"],
            dry_run=self.settings.dry_run,
        )
