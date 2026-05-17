"""Base workflow — two-phase pipeline: scrape+save, then process+email+update."""

from __future__ import annotations

import logging
import time
from datetime import datetime
from pathlib import Path
from typing import TYPE_CHECKING

from jobspy_v2.config import Settings
from jobspy_v2.core.dedup import Deduplicator
from jobspy_v2.core.email_gen import generate_email
from jobspy_v2.core.email_sender import is_gmail_quota_error, send_email
from jobspy_v2.core.reporter import send_report
from jobspy_v2.core.scraper import scrape_jobs
from jobspy_v2.storage import create_storage_backend
from jobspy_v2.utils.email_utils import filter_deliverable_emails, get_valid_recipients

if TYPE_CHECKING:
    import pandas as pd

    from jobspy_v2.storage.base import StorageBackend

logger = logging.getLogger(__name__)

CONTEXT_DIR = Path("contexts")


# ------------------------------------------------------------------
# Helpers
# ------------------------------------------------------------------


def _safe_str(value: object) -> str:
    """Convert a value to string, treating NaN/None/lists cleanly."""
    if value is None:
        return ""
    if isinstance(value, (list, tuple)):
        return ", ".join(str(v) for v in value if v is not None)
    s = str(value)
    if s in ("nan", "NaN", "None", "NaT"):
        return ""
    return s


def _reason_to_stat_key(reason: str) -> str:
    """Map a dedup.can_send() reason string to its stats-dict key."""
    if "Already sent" in reason:
        return "skipped_dedup_exact"
    if "cooldown" in reason:
        return "skipped_dedup_domain"
    if "already contacted today" in reason:
        return "skipped_dedup_company"
    return "skipped_dedup_exact"  # safe fallback


def _is_time_limit_reached(start: float, settings: Settings) -> bool:
    """Check if we're approaching the max runtime limit."""
    elapsed_minutes = (time.monotonic() - start) / 60
    buffer_minutes = 5
    return elapsed_minutes >= (settings.max_runtime_minutes - buffer_minutes)


# ------------------------------------------------------------------
# Workflow
# ------------------------------------------------------------------


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
        """Execute the full two-phase pipeline.

        Returns 0 on success, 1 on fatal error.
        """
        start = time.monotonic()

        if self._should_skip_weekend():
            return 0

        self._context = self._load_context()

        stats: dict = {
            "total_scraped": 0,
            "jobs_with_emails": 0,
            "emails_sent": 0,
            "emails_failed": 0,
            "skipped_dedup_exact": 0,
            "skipped_dedup_domain": 0,
            "skipped_dedup_company": 0,
            "skipped_no_recipients": 0,
            "skipped_timeout": 0,
            "skipped_daily_quota": 0,
            "invalid_emails_filtered": 0,
            "filtered_title": 0,
            "filtered_email": 0,
            "boards_queried": [],
            "pending_carried_over": 0,
            "run_stop_reason": "",
        }

        try:
            # ── Phase 0: Get pending jobs count (for reporting) ───────────
            pending_jobs = self._get_pending_jobs()
            stats["pending_carried_over"] = len(pending_jobs)

            # ── Phase 1: Scrape + Save ────────────────────────────────
            result = scrape_jobs(self.settings, self.mode)
            stats["total_scraped"] = len(result.jobs)
            stats["boards_queried"] = result.boards_queried
            stats["filtered_email"] = result.total_raw - result.total_after_email_filter
            stats["filtered_title"] = (
                result.total_after_email_filter - result.total_after_title_filter
            )

            if result.jobs.empty:
                logger.info("[%s] No jobs found after filtering.", self.mode)
                if pending_jobs and not self.settings.dry_run:
                    self._process_pending_after_new_jobs(pending_jobs, stats, start)
                self._send_report(stats, start)
                return 0

            start_row = self._save_scraped_jobs(result.jobs)

            # ── Phase 2: Process + Email + Update ─────────────────────
            if _is_time_limit_reached(start, self.settings):
                stats["run_stop_reason"] = "timeout"
                logger.warning(
                    "[%s] Time limit reached, stopping gracefully.", self.mode
                )
            else:
                max_emails = self._get_max_emails()
                if max_emails > 0:
                    self._process_jobs(result.jobs, start_row, max_emails, stats, start)
                else:
                    stats["run_stop_reason"] = "daily_quota"
                    logger.info(
                        "[%s] No email quota remaining, skipping new jobs.", self.mode
                    )

            logger.info(
                "[%s] Email phase complete: sent=%d, failed=%d, skipped_no_email=%d, "
                "skipped_dedup=%d, jobs_with_emails=%d",
                self.mode,
                stats["emails_sent"],
                stats["emails_failed"],
                stats["skipped_no_recipients"],
                stats["skipped_dedup_exact"]
                + stats["skipped_dedup_domain"]
                + stats["skipped_dedup_company"],
                stats["jobs_with_emails"],
            )

            # ── Phase 3: Process pending jobs if under daily limit ─────
            if (
                pending_jobs
                and not self.settings.dry_run
                and not stats["run_stop_reason"]
            ):
                self._process_pending_after_new_jobs(pending_jobs, stats, start)

        except Exception:
            logger.exception("[%s] Fatal error in pipeline.", self.mode)
            stats["emails_failed"] = int(stats["emails_failed"]) + 1
            self._send_report(stats, start)
            return 1

        self._send_report(stats, start)
        return 0

    # ------------------------------------------------------------------
    # Phase 1: Save scraped jobs to storage
    # ------------------------------------------------------------------

    def _save_scraped_jobs(self, jobs_df: pd.DataFrame) -> int:
        """Batch-write all scraped jobs to storage.

        Returns the 1-indexed starting row number of the newly written block.
        """
        now = datetime.now().isoformat(timespec="seconds")
        records: list[dict[str, str]] = []

        for _, row in jobs_df.iterrows():
            records.append(
                {
                    "date_scraped": now,
                    "board": _safe_str(row.get("site")),
                    "title": _safe_str(row.get("title")),
                    "company": _safe_str(row.get("company")),
                    "company_url": _safe_str(row.get("company_url")),
                    "location": _safe_str(row.get("location")),
                    "is_remote": _safe_str(row.get("is_remote")),
                    "job_url": _safe_str(row.get("job_url")),
                    "job_type": _safe_str(row.get("job_type")),
                    "date_posted": _safe_str(row.get("date_posted")),
                    "emails": _safe_str(row.get("emails")),
                    "salary_min": _safe_str(row.get("min_amount")),
                    "salary_max": _safe_str(row.get("max_amount")),
                    "salary_currency": _safe_str(row.get("currency")),
                    "salary_interval": _safe_str(row.get("interval")),
                    "skills": _safe_str(row.get("skills")),
                    "experience_range": _safe_str(row.get("experience_range")),
                    "job_level": _safe_str(row.get("job_level")),
                    "company_industry": _safe_str(row.get("company_industry")),
                    "email_sent": "Pending",
                    "skip_reason": "",
                    "email_recipient": "",
                }
            )

        start_row = self.storage.add_scraped_jobs(records)
        logger.info(
            "[%s] Phase 1 complete: %d jobs saved to storage (rows %d-%d)",
            self.mode,
            len(records),
            start_row,
            start_row + len(records) - 1,
        )
        return start_row

    # ------------------------------------------------------------------
    # Phase 2: Process each job — email + update row status
    # ------------------------------------------------------------------

    def _process_jobs(
        self,
        jobs_df: pd.DataFrame,
        start_row: int,
        max_emails: int,
        stats: dict,
        start_time: float = 0,
    ) -> None:
        """Iterate jobs sequentially: validate -> dedup -> email -> update status."""
        sent_count = 0

        for i, (_, row) in enumerate(jobs_df.iterrows()):
            if sent_count >= max_emails:
                stats["run_stop_reason"] = "daily_quota"
                logger.info("[%s] Reached max emails (%d).", self.mode, max_emails)
                break

            if start_time and _is_time_limit_reached(start_time, self.settings):
                stats["run_stop_reason"] = "timeout"
                stats["skipped_timeout"] += 1
                logger.warning(
                    "[%s] Time limit reached, stopping job processing.", self.mode
                )
                break

            row_number = start_row + i
            title = _safe_str(row.get("title"))
            company = _safe_str(row.get("company"))
            job_url = _safe_str(row.get("job_url"))
            description = _safe_str(row.get("description"))
            location = _safe_str(row.get("location"))
            is_remote = bool(row.get("is_remote", False))
            raw_emails = _safe_str(row.get("emails"))

            # ── Step 1: Extract valid recipients ──────────────────────
            recipients = get_valid_recipients(
                raw_emails, self.settings.email_filter_patterns
            )
            if not recipients:
                self._update_row_status(
                    row_number, "Skipped", "no_valid_recipients", ""
                )
                stats["skipped_no_recipients"] += 1
                continue

            # ── Step 1b: DNS/MX deliverability filter (emval) ─────────
            recipients, dns_invalid = filter_deliverable_emails(recipients)
            if dns_invalid:
                stats["invalid_emails_filtered"] += len(dns_invalid)
                logger.debug(
                    "Filtered %d undeliverable email(s) for %s: %s",
                    len(dns_invalid),
                    company,
                    dns_invalid,
                )
            if not recipients:
                self._update_row_status(row_number, "Skipped", "invalid_email_dns", "")
                stats["skipped_no_recipients"] += 1
                continue

            stats["jobs_with_emails"] += 1
            primary_email = recipients[0]
            domain = primary_email.split("@")[1] if "@" in primary_email else ""

            # ── Step 2: Dedup check ───────────────────────────────────
            can_send, reason = self.dedup.can_send(primary_email, domain, company)
            if not can_send:
                stat_key = _reason_to_stat_key(reason)
                self._update_row_status(row_number, "Skipped", reason, primary_email)
                stats[stat_key] += 1
                logger.debug("Skipping %s: %s", primary_email, reason)
                continue

            # ── Step 3: Generate email ────────────────────────────────
            email_result = generate_email(
                job_title=title,
                company=company,
                job_description=description,
                settings=self.settings,
                context=self._context,
                workflow_mode=self.mode,
            )

            # ── Step 4: Send (or dry-run) ─────────────────────────────
            if self.settings.dry_run:
                logger.debug(
                    "[DRY RUN] Would send to %s — %s at %s",
                    primary_email,
                    title,
                    company,
                )
                self._update_row_status(row_number, "DryRun", "", primary_email)
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
                    if is_gmail_quota_error(error):
                        stats["run_stop_reason"] = "gmail_quota_exceeded"
                        stats["skipped_daily_quota"] += 1
                        logger.error(
                            "[%s] Gmail quota exceeded, stopping email sending.",
                            self.mode,
                        )
                        break
                    self._update_row_status(
                        row_number,
                        "Failed",
                        f"smtp_error: {error}",
                        primary_email,
                    )
                    stats["emails_failed"] += 1
                    continue

                # Live success
                self._update_row_status(row_number, "Yes", "", primary_email)

            # ── Step 5: Record success (both live and dry-run) ────────
            self.dedup.mark_sent(
                email=primary_email,
                domain=domain,
                company=company,
                job_title=title,
                job_url=job_url,
                location=location,
                is_remote=is_remote,
                subject=email_result.subject,
                body_preview=email_result.body,
                mode=email_result.mode,
                word_count=email_result.word_count,
            )
            sent_count += 1
            stats["emails_sent"] += 1

            if sent_count < max_emails and not self.settings.dry_run:
                time.sleep(self.settings.email_interval_seconds)

    # ------------------------------------------------------------------
    # Internals
    # ------------------------------------------------------------------

    def _update_row_status(
        self,
        row_number: int,
        email_sent: str,
        skip_reason: str,
        email_recipient: str,
    ) -> None:
        """Update a single row's status — errors logged, pipeline continues."""
        try:
            self.storage.update_scraped_job_status(
                row_number, email_sent, skip_reason, email_recipient
            )
        except Exception:
            logger.exception(
                "Failed to update row %d status (email_sent=%s)",
                row_number,
                email_sent,
            )

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

    def _send_report(self, stats: dict, start: float) -> None:
        # Guard against time.monotonic() side_effect list exhaustion in tests.
        # In Python <3.12, StopIteration is raised directly by MagicMock
        # when its configured side_effect list is exhausted.
        # In Python >=3.12, PEP 479 converts this into RuntimeError.
        # Neither should occur in production (time.monotonic is a C builtin),
        # but catching both keeps the test suite reliable across Python versions.
        try:
            now = time.monotonic()
        except (StopIteration, RuntimeError):
            now = start
        duration = now - start
        send_report(
            settings=self.settings,
            storage=self.storage,
            mode=self.mode,
            stats=stats,
            duration_seconds=duration,
        )

    # ------------------------------------------------------------------
    # Carry-over: Process pending jobs from previous runs
    # ------------------------------------------------------------------

    def _get_pending_jobs(self) -> list[dict[str, str]]:
        """Get pending jobs from storage (carry-over from previous runs).

        These are jobs that were scraped but couldn't be processed due to
        daily email limit in previous runs.
        """
        try:
            pending = self.storage.get_pending_jobs()
            return pending
        except Exception:
            logger.exception("[%s] Failed to get pending jobs", self.mode)
            return []

    def _process_pending_jobs(
        self,
        pending_jobs: list[dict[str, str]],
        max_emails: int,
        stats: dict,
        start_time: float = 0,
    ) -> int:
        """Process pending jobs from previous runs.

        These jobs already exist in storage with status 'Pending'.
        We process them first before new jobs to ensure carry-over works.

        Returns the remaining email quota after processing.
        """
        if not pending_jobs or max_emails <= 0:
            return max_emails

        sent_count = 0

        for job in pending_jobs:
            if sent_count >= max_emails:
                if not stats["run_stop_reason"]:
                    stats["run_stop_reason"] = "daily_quota"
                logger.info(
                    "[%s] Carried-over: reached max emails (%d)",
                    self.mode,
                    max_emails,
                )
                break

            if start_time and _is_time_limit_reached(start_time, self.settings):
                stats["run_stop_reason"] = "timeout"
                stats["skipped_timeout"] += 1
                logger.warning(
                    "[%s] Time limit reached during pending jobs.", self.mode
                )
                break

            # Get row_number from the job (added by storage backend)
            try:
                row_number = int(job.get("row_number", 0))
            except (ValueError, TypeError):
                row_number = 0

            if row_number <= 0:
                logger.warning(
                    "[%s] Skipping pending job without row_number", self.mode
                )
                continue

            title = job.get("title", "")
            company = job.get("company", "")
            job_url = job.get("job_url", "")
            location = job.get("location", "")
            is_remote = job.get("is_remote", "") == "True"
            raw_emails = job.get("emails", "")

            # Extract valid recipients
            recipients = get_valid_recipients(
                raw_emails, self.settings.email_filter_patterns
            )
            if not recipients:
                self._update_row_status(
                    row_number, "Skipped", "no_valid_recipients", ""
                )
                stats["skipped_no_recipients"] += 1
                continue

            # DNS/MX deliverability filter (emval)
            recipients, dns_invalid = filter_deliverable_emails(recipients)
            if dns_invalid:
                stats["invalid_emails_filtered"] += len(dns_invalid)
                logger.debug(
                    "Filtered %d undeliverable email(s) for %s: %s",
                    len(dns_invalid),
                    company,
                    dns_invalid,
                )
            if not recipients:
                self._update_row_status(row_number, "Skipped", "invalid_email_dns", "")
                stats["skipped_no_recipients"] += 1
                continue

            stats["jobs_with_emails"] += 1
            primary_email = recipients[0]
            domain = primary_email.split("@")[1] if "@" in primary_email else ""

            # Dedup check
            can_send, reason = self.dedup.can_send(primary_email, domain, company)
            if not can_send:
                stat_key = _reason_to_stat_key(reason)
                self._update_row_status(row_number, "Skipped", reason, primary_email)
                stats[stat_key] += 1
                continue

            # Generate email
            description = job.get("description", "")
            email_result = generate_email(
                job_title=title,
                company=company,
                job_description=description,
                settings=self.settings,
                context=self._context,
                workflow_mode=self.mode,
            )

            # Send or dry-run
            if self.settings.dry_run:
                logger.debug(
                    "[DRY RUN] Would send to %s — %s at %s",
                    primary_email,
                    title,
                    company,
                )
                self._update_row_status(row_number, "DryRun", "", primary_email)
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
                    if is_gmail_quota_error(error):
                        stats["run_stop_reason"] = "gmail_quota_exceeded"
                        stats["skipped_daily_quota"] += 1
                        logger.error(
                            "[%s] Gmail quota exceeded, stopping email sending.",
                            self.mode,
                        )
                        break
                    self._update_row_status(
                        row_number, "Failed", f"smtp_error: {error}", primary_email
                    )
                    stats["emails_failed"] += 1
                    continue

                self._update_row_status(row_number, "Yes", "", primary_email)

            # Record success
            self.dedup.mark_sent(
                email=primary_email,
                domain=domain,
                company=company,
                job_title=title,
                job_url=job_url,
                location=location,
                is_remote=is_remote,
                subject=email_result.subject,
                body_preview=email_result.body,
                mode=email_result.mode,
                word_count=email_result.word_count,
            )
            sent_count += 1
            stats["emails_sent"] += 1
            stats["pending_carried_over"] += 1

            if sent_count < max_emails and not self.settings.dry_run:
                time.sleep(self.settings.email_interval_seconds)

        logger.info(
            "[%s] Carried-over: processed %d pending jobs, sent %d",
            self.mode,
            len(pending_jobs),
            sent_count,
        )

        # Return remaining quota
        return max_emails - sent_count

    # ------------------------------------------------------------------
    # Process pending jobs AFTER new jobs (daily limit aware)
    # ------------------------------------------------------------------

    def _process_pending_after_new_jobs(
        self,
        pending_jobs: list[dict[str, str]],
        stats: dict,
        start_time: float = 0,
    ) -> None:
        """Process pending jobs only if total emails sent today is under limit.

        This runs AFTER processing new scraped jobs. It checks the total emails
        sent today (both remote and onsite) and only processes pending jobs
        if we're under the daily limit.
        """
        if not pending_jobs:
            return

        total_daily_limit = self.settings.daily_total_emails_limit
        today_sent = self.storage.get_today_sent_emails_count()

        # Guard: MagicMock comparisons with int raise TypeError in Python 3.13+
        if not isinstance(today_sent, int):
            return

        if today_sent >= total_daily_limit:
            stats["run_stop_reason"] = "daily_quota"
            logger.info(
                "[%s] Daily email limit reached (%d/%d). Skipping pending jobs.",
                self.mode,
                today_sent,
                total_daily_limit,
            )
            return

        remaining_quota = total_daily_limit - today_sent
        logger.info(
            "[%s] Processing pending jobs: %d sent today, %d remaining quota",
            self.mode,
            today_sent,
            remaining_quota,
        )

        self._process_pending_jobs(pending_jobs, remaining_quota, stats, start_time)
