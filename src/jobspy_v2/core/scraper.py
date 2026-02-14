"""Job scraping with python-jobspy — smart param handling per board."""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

import pandas as pd
from jobspy import scrape_jobs as jobspy_scrape

from jobspy_v2.utils.email_utils import extract_emails, get_valid_recipients

if TYPE_CHECKING:
    from jobspy_v2.config.settings import Settings

logger = logging.getLogger(__name__)


# ── Indeed API limitation ──────────────────────────────────────────────
# Indeed only supports ONE of these param groups per request:
#   1. hours_old
#   2. job_type + is_remote
#   3. easy_apply
# We prioritise job_type+is_remote over hours_old to get relevant results.
INDEED_EXCLUSIVE_PARAMS = {"hours_old", "job_type", "is_remote", "easy_apply"}


@dataclass(frozen=True)
class ScrapeResult:
    """Immutable container for scrape output."""

    jobs: pd.DataFrame
    total_raw: int = 0
    total_after_email_filter: int = 0
    total_after_title_filter: int = 0
    total_deduplicated: int = 0
    boards_queried: list[str] = field(default_factory=list)


def _build_base_params(settings: Settings, mode: str) -> dict:
    """Build base params from settings for the given mode (onsite/remote)."""
    prefix = mode.lower()
    params: dict = {
        "results_wanted": getattr(settings, f"{prefix}_results_wanted"),
        "country_indeed": getattr(settings, f"{prefix}_country_indeed"),
        "verbose": 2,
    }

    job_type = getattr(settings, f"{prefix}_job_type", None)
    if job_type:
        params["job_type"] = job_type

    is_remote = getattr(settings, f"{prefix}_is_remote", False)
    if is_remote:
        params["is_remote"] = True

    if settings.proxy_list:
        params["proxies"] = settings.proxy_list

    return params


def _adapt_params_for_board(params: dict, board: str) -> dict:
    """Adjust params for board-specific limitations."""
    adapted = dict(params)

    if board == "indeed":
        # Indeed: only ONE of hours_old / (job_type+is_remote) / easy_apply
        # We keep job_type+is_remote, drop the rest
        adapted.pop("hours_old", None)
        adapted.pop("easy_apply", None)

    if board == "google":
        # Google only uses google_search_term, not search_term
        search = adapted.pop("search_term", None)
        if search:
            adapted["google_search_term"] = search

    if board == "linkedin":
        # LinkedIn: easy_apply filter no longer works
        adapted.pop("easy_apply", None)

    return adapted


def _has_valid_emails(emails_str: str | None) -> bool:
    """Check if the emails field contains at least one valid email."""
    if not emails_str or pd.isna(emails_str):
        return False
    return len(extract_emails(str(emails_str))) > 0


def _should_reject_title(title: str | None, reject_patterns: list[str]) -> bool:
    """Check if a job title matches any reject pattern (case-insensitive)."""
    if not title:
        return False
    title_lower = title.lower()
    return any(pattern.lower() in title_lower for pattern in reject_patterns)


def scrape_jobs(settings: Settings, mode: str) -> ScrapeResult:
    """
    Scrape jobs for the given mode (onsite/remote).

    Pipeline:
    1. Iterate locations × search_terms × boards
    2. Call jobspy per combination
    3. Filter: has valid email → reject titles → filter emails → dedup by job_url
    """
    prefix = mode.lower()
    search_terms: list[str] = getattr(settings, f"{prefix}_search_terms")
    boards: list[str] = getattr(settings, f"{prefix}_job_boards")

    # Locations: onsite has a list, remote has a single location
    if prefix == "onsite":
        locations: list[str] = settings.onsite_locations
    else:
        locations = [settings.remote_location]

    base_params = _build_base_params(settings, mode)
    all_frames: list[pd.DataFrame] = []
    boards_queried: list[str] = []

    for location in locations:
        for term in search_terms:
            for board in boards:
                params = _adapt_params_for_board(
                    {**base_params, "search_term": term}, board
                )
                params["site_name"] = [board]
                params["location"] = location

                logger.info(
                    "Scraping %s | location=%s | term='%s'",
                    board,
                    location,
                    term,
                )
                try:
                    df = jobspy_scrape(**params)
                    if df is not None and not df.empty:
                        all_frames.append(df)
                        if board not in boards_queried:
                            boards_queried.append(board)
                        logger.info("  → %d results from %s", len(df), board)
                    else:
                        logger.info("  → 0 results from %s", board)
                except Exception:
                    logger.exception(
                        "  → Error scraping %s for '%s' in %s",
                        board,
                        term,
                        location,
                    )

    if not all_frames:
        logger.warning("No jobs found across any board/location/term combo")
        return ScrapeResult(
            jobs=pd.DataFrame(),
            boards_queried=boards_queried,
        )

    combined = pd.concat(all_frames, ignore_index=True)
    total_raw = len(combined)
    logger.info("Total raw results: %d", total_raw)

    # ── Filter: must have valid email ──────────────────────────────────
    if "emails" in combined.columns:
        combined = combined[combined["emails"].apply(_has_valid_emails)]
    else:
        logger.warning("No 'emails' column in results — returning empty")
        return ScrapeResult(
            jobs=pd.DataFrame(),
            total_raw=total_raw,
            boards_queried=boards_queried,
        )
    total_after_email_filter = len(combined)
    logger.info("After email validation: %d", total_after_email_filter)

    # ── Filter: reject unwanted titles ─────────────────────────────────
    reject_patterns = settings.reject_titles
    if reject_patterns:
        mask = combined["title"].apply(
            lambda t: not _should_reject_title(t, reject_patterns)
        )
        rejected_count = (~mask).sum()
        if rejected_count > 0:
            logger.info("Rejected %d jobs by title filter", rejected_count)
        combined = combined[mask]
    total_after_title_filter = len(combined)

    # ── Filter: email filter patterns ──────────────────────────────────
    email_filters = settings.email_filter_patterns
    if email_filters:

        def _filter_emails_in_row(emails_str: str) -> str:
            valid = get_valid_recipients(str(emails_str), email_filters)
            return ",".join(valid)

        combined["emails"] = combined["emails"].apply(_filter_emails_in_row)
        combined = combined[combined["emails"].str.len() > 0]

    # ── Dedup by job_url ───────────────────────────────────────────────
    if "job_url" in combined.columns:
        before_dedup = len(combined)
        combined = combined.drop_duplicates(subset=["job_url"], keep="first")
        total_deduplicated = len(combined)
        dupes = before_dedup - total_deduplicated
        if dupes > 0:
            logger.info("Removed %d duplicate job URLs", dupes)
    else:
        total_deduplicated = len(combined)

    combined = combined.reset_index(drop=True)
    logger.info("Final jobs to process: %d", total_deduplicated)

    return ScrapeResult(
        jobs=combined,
        total_raw=total_raw,
        total_after_email_filter=total_after_email_filter,
        total_after_title_filter=total_after_title_filter,
        total_deduplicated=total_deduplicated,
        boards_queried=boards_queried,
    )
