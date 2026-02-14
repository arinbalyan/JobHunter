"""Deduplication logic against storage backend."""

from __future__ import annotations

import logging
from datetime import date, datetime, timedelta
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from jobspy_v2.storage.base import StorageBackend

logger = logging.getLogger(__name__)

# Cooldown periods
DOMAIN_COOLDOWN_DAYS = 5
COMPANY_COOLDOWN_DAYS = 1


class Deduplicator:
    """Check and record sent emails to prevent duplicate outreach."""

    def __init__(self, storage: StorageBackend) -> None:
        self._storage = storage
        self._cache: list[dict[str, str]] | None = None

    def _get_entries(self) -> list[dict[str, str]]:
        """Fetch and cache sent email entries from storage."""
        if self._cache is None:
            self._cache = self._storage.get_sent_emails()
        return self._cache

    def invalidate_cache(self) -> None:
        """Force re-fetch from storage on next check."""
        self._cache = None

    def can_send(
        self,
        email: str,
        domain: str,
        company: str,
    ) -> tuple[bool, str]:
        """
        Check if we can send to this recipient.

        Rules:
        1. Exact email already sent → reject
        2. Same domain within DOMAIN_COOLDOWN_DAYS → reject
        3. Same company contacted today → reject

        Returns (can_send, reason) where reason is empty string if allowed.
        """
        entries = self._get_entries()
        email_lower = email.lower().strip()
        domain_lower = domain.lower().strip()
        company_lower = company.lower().strip()
        today = date.today()

        for entry in entries:
            entry_email = entry.get("email", "").lower().strip()
            entry_domain = entry.get("domain", "").lower().strip()
            entry_company = entry.get("company", "").lower().strip()
            entry_date_str = entry.get("date_sent", "")

            # Rule 1: exact email match
            if entry_email == email_lower:
                return False, f"Already sent to {email}"

            # Parse date for cooldown checks
            entry_date = _parse_date(entry_date_str)
            if entry_date is None:
                continue

            # Rule 2: domain cooldown
            if entry_domain and entry_domain == domain_lower:
                days_since = (today - entry_date).days
                if days_since < DOMAIN_COOLDOWN_DAYS:
                    return (
                        False,
                        f"Domain {domain} contacted {days_since}d ago "
                        f"(cooldown: {DOMAIN_COOLDOWN_DAYS}d)",
                    )

            # Rule 3: same company today
            if entry_company and entry_company == company_lower:
                if entry_date == today:
                    return (
                        False,
                        f"Company {company} already contacted today",
                    )

        return True, ""

    def mark_sent(
        self,
        *,
        email: str,
        domain: str,
        company: str,
        job_title: str = "",
        job_url: str = "",
        location: str = "",
        is_remote: bool = False,
    ) -> None:
        """Record a sent email in storage and update local cache."""
        row = {
            "email": email,
            "domain": domain,
            "company": company,
            "date_sent": date.today().isoformat(),
            "job_title": job_title,
            "job_url": job_url,
            "location": location,
            "is_remote": str(is_remote),
        }
        self._storage.add_sent_email(row)

        # Update cache in-place to avoid re-fetching
        if self._cache is not None:
            self._cache.append(row)

        logger.debug("Marked sent: %s @ %s", email, company)


def _parse_date(date_str: str) -> date | None:
    """Parse ISO date string, returning None on failure."""
    if not date_str:
        return None
    try:
        return datetime.fromisoformat(date_str).date()
    except (ValueError, TypeError):
        return None
