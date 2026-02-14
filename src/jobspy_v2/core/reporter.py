"""End-of-run report: summary email + storage stats."""

from __future__ import annotations

import logging
import smtplib
from datetime import datetime
from email.mime.text import MIMEText
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from jobspy_v2.config.settings import Settings
    from jobspy_v2.storage.base import StorageBackend

logger = logging.getLogger(__name__)


def send_report(
    *,
    settings: Settings,
    storage: StorageBackend,
    mode: str,
    total_scraped: int = 0,
    total_emailed: int = 0,
    total_errors: int = 0,
    total_skipped: int = 0,
    total_filtered: int = 0,
    duration_seconds: float = 0.0,
    boards_queried: list[str] | None = None,
    dry_run: bool = False,
) -> bool:
    """
    Send a summary report email and record run stats.

    Returns True if report email was sent successfully.
    """
    timestamp = datetime.now().isoformat(timespec="seconds")
    boards_str = ", ".join(boards_queried) if boards_queried else "none"

    # ── Record run stats in storage ────────────────────────────────────
    stats_row = {
        "timestamp": timestamp,
        "mode": mode,
        "total_scraped": str(total_scraped),
        "total_emailed": str(total_emailed),
        "total_errors": str(total_errors),
        "total_skipped": str(total_skipped),
        "total_filtered": str(total_filtered),
        "duration_seconds": f"{duration_seconds:.1f}",
        "boards_queried": boards_str,
        "dry_run": str(dry_run),
    }
    try:
        storage.add_run_stats(stats_row)
        logger.info("Run stats recorded to storage")
    except Exception:
        logger.exception("Failed to record run stats")

    # ── Build report email ─────────────────────────────────────────────
    subject = _build_subject(mode, total_emailed, dry_run)
    body = _build_body(
        mode=mode,
        timestamp=timestamp,
        total_scraped=total_scraped,
        total_filtered=total_filtered,
        total_skipped=total_skipped,
        total_emailed=total_emailed,
        total_errors=total_errors,
        duration_seconds=duration_seconds,
        boards_str=boards_str,
        dry_run=dry_run,
    )

    # ── Send report email ──────────────────────────────────────────────
    if not settings.report_email:
        logger.warning("No REPORT_EMAIL configured — skipping report email")
        return False

    return _send_report_email(
        to_email=settings.report_email,
        subject=subject,
        body=body,
        settings=settings,
    )


def _build_subject(mode: str, total_emailed: int, dry_run: bool) -> str:
    """Build report email subject line."""
    prefix = "[DRY RUN] " if dry_run else ""
    return f"{prefix}JobSpy-V2 Report: {mode.upper()} — {total_emailed} emails sent"


def _build_body(
    *,
    mode: str,
    timestamp: str,
    total_scraped: int,
    total_filtered: int,
    total_skipped: int,
    total_emailed: int,
    total_errors: int,
    duration_seconds: float,
    boards_str: str,
    dry_run: bool,
) -> str:
    """Build report email body with stats summary."""
    minutes = duration_seconds / 60
    status = "DRY RUN" if dry_run else "LIVE"

    return f"""JobSpy-V2 Run Report
{"=" * 40}

Mode:            {mode.upper()} ({status})
Timestamp:       {timestamp}
Duration:        {minutes:.1f} minutes ({duration_seconds:.0f}s)
Boards queried:  {boards_str}

Pipeline Summary
{"-" * 40}
Total scraped:   {total_scraped}
Filtered out:    {total_filtered}
Skipped (dedup): {total_skipped}
Emails sent:     {total_emailed}
Errors:          {total_errors}

Success rate:    {_calc_rate(total_emailed, total_emailed + total_errors)}
"""


def _calc_rate(success: int, total: int) -> str:
    """Calculate success rate as percentage string."""
    if total == 0:
        return "N/A"
    return f"{(success / total) * 100:.1f}%"


def _send_report_email(
    *,
    to_email: str,
    subject: str,
    body: str,
    settings: Settings,
) -> bool:
    """Send report email via SMTP (no retry — best effort)."""
    try:
        msg = MIMEText(body, "plain", "utf-8")
        msg["From"] = f"JobSpy-V2 <{settings.gmail_email}>"
        msg["To"] = to_email
        msg["Subject"] = subject

        with smtplib.SMTP(settings.smtp_host, settings.smtp_port, timeout=30) as server:
            server.ehlo()
            server.starttls()
            server.ehlo()
            server.login(settings.gmail_email, settings.gmail_app_password)
            server.send_message(msg)

        logger.info("Report email sent to %s", to_email)
        return True
    except Exception:
        logger.exception("Failed to send report email")
        return False
