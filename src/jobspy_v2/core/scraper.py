"""Job scraping with python-jobspy — smart param handling per board.

Uses ``concurrent.futures.ThreadPoolExecutor`` to scrape multiple
board×location×term combinations in parallel (I/O-bound HTTP calls).
The ``SCRAPE_MAX_WORKERS`` env var (default 5) controls concurrency.
"""

from __future__ import annotations

import logging
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

import pandas as pd
from jobspy import scrape_jobs as jobspy_scrape
from jobspy.model import Country

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


def _get_countries_indeed(settings: Settings, mode: str) -> list[str]:
    """Get list of countries to scrape for Indeed."""
    prefix = mode.lower()
    countries = getattr(settings, f"{prefix}_countries_indeed", [])
    if not countries and prefix == "remote":
        countries = [getattr(settings, f"{prefix}_country_indeed", "USA")]
    elif not countries:
        countries = [getattr(settings, f"{prefix}_country_indeed", "")]
    return [c for c in countries if c]


def _is_glassdoor_country_supported(country: str) -> bool:
    """Return True if python-jobspy supports this country for Glassdoor."""
    country_enum = Country.__members__.get(country.upper())
    if country_enum is None:
        logger.debug("Skipping unknown Glassdoor country code: %s", country)
        return False
    # Country.value tuple:
    #   (aliases_csv, indeed_domain[, glassdoor_tld_or_subdomain])
    # Glassdoor support exists only when the 3rd element is present/non-empty.
    return len(country_enum.value) >= 3 and bool(country_enum.value[2])


def _get_remote_location_country_pairs(
    settings: Settings, countries_indeed: list[str]
) -> list[tuple[str, str]]:
    """Build remote location-country pairs without Cartesian explosion."""
    remote_location = settings.remote_location or "Remote"

    if settings.remote_is_remote:
        # Remote mode should not depend on region-specific locations.
        # Keep one stable location label while iterating countries only.
        if countries_indeed:
            return [("Remote", country) for country in countries_indeed]
        return [("Remote", "")]

    locations = [loc for loc in settings.remote_locations if loc]

    if locations and countries_indeed:
        if len(locations) == len(countries_indeed):
            # IMPORTANT: use zip() to preserve intended 1:1 location-country mapping.
            # Nested loops create a Cartesian product with invalid pairs.
            return list(zip(locations, countries_indeed))
        logger.warning(
            "REMOTE_LOCATIONS and REMOTE_COUNTRIES_INDEED length mismatch "
            "(locations=%d, countries=%d). Falling back to safe mode: "
            "location='Remote', iterating countries only.",
            len(locations),
            len(countries_indeed),
        )
        return [("Remote", country) for country in countries_indeed]

    if countries_indeed:
        return [(remote_location, country) for country in countries_indeed]
    if locations:
        return [(location, "") for location in locations]
    return [(remote_location, "")]


def _build_base_params(settings: Settings, mode: str, country_indeed: str = "") -> dict:
    """Build base params from settings for the given mode (onsite/remote)."""
    prefix = mode.lower()
    params: dict = {
        "results_wanted": getattr(settings, f"{prefix}_results_wanted"),
        "linkedin_fetch_description": True,
        "verbose": 0,
    }

    if not country_indeed:
        country_indeed = getattr(settings, f"{prefix}_country_indeed", "")

    if country_indeed:
        params["country_indeed"] = country_indeed

    hours_old = getattr(settings, f"{prefix}_hours_old", None)
    if hours_old:
        params["hours_old"] = hours_old

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


def _scrape_single(
    params: dict, board: str, location: str, term: str, country: str = ""
) -> pd.DataFrame | None:
    """Execute a single jobspy scrape call (designed to run in a thread)."""
    logger.debug(
        "Scraping board=%s location=%s term='%s' country=%s",
        board,
        location,
        term,
        country,
    )
    try:
        df = jobspy_scrape(**params)
        if df is not None and not df.empty:
            logger.debug("  → %d results from %s", len(df), board)
            return df
        logger.debug("  → 0 results from %s", board)
    except Exception:
        logger.warning(
            "  → Error scraping board=%s term='%s' location=%s country=%s",
            board,
            term,
            location,
            country,
        )
    return None


def scrape_jobs(settings: Settings, mode: str) -> ScrapeResult:
    """
    Scrape jobs for the given mode (onsite/remote).

    Pipeline:
    1. Build param combos for locations × search_terms × boards
    2. Dispatch all combos to a ThreadPoolExecutor (parallel I/O)
    3. Filter: has valid email → reject titles → filter emails → dedup by job_url
    """
    prefix = mode.lower()
    search_terms: list[str] = getattr(settings, f"{prefix}_search_terms")
    boards: list[str] = getattr(settings, f"{prefix}_job_boards")

    # Locations: both onsite and remote support lists
    if prefix == "onsite":
        locations: list[str] = settings.onsite_locations
    else:
        locations: list[str] = settings.remote_locations

    # Get countries for Indeed (remote mode supports multiple)
    countries_indeed = _get_countries_indeed(settings, mode)
    if countries_indeed and "indeed" in boards:
        logger.info(
            "[%s] Indeed will scrape %d countries: %s",
            mode,
            len(countries_indeed),
            ", ".join(countries_indeed),
        )

    location_country_pairs: list[tuple[str, str]]
    if prefix == "remote":
        location_country_pairs = _get_remote_location_country_pairs(
            settings, countries_indeed
        )
    else:
        location_country_pairs = [(location, "") for location in locations]

    logger.info(
        "[%s] Starting scrape: %d terms × %d boards × %d location-country pairs",
        mode,
        len(search_terms),
        len(boards),
        len(location_country_pairs),
    )
    logger.debug("Terms: %s", search_terms)
    logger.debug("Boards: %s", boards)
    logger.debug("Location-country pairs: %s", location_country_pairs)

    # ── Build all param combinations ───────────────────────────────────
    tasks: list[tuple[dict, str, str, str, str]] = []
    for location, country in location_country_pairs:
        for term in search_terms:
            for board in boards:
                if (
                    board == "glassdoor"
                    and country
                    and not _is_glassdoor_country_supported(country)
                ):
                    continue

                include_country = board in ("indeed", "glassdoor")
                country_for_request = country if include_country else ""
                base_params = _build_base_params(settings, mode, country_for_request)
                if not include_country:
                    base_params.pop("country_indeed", None)
                params = _adapt_params_for_board(
                    {**base_params, "search_term": term}, board
                )
                params["site_name"] = [board]
                params["location"] = location
                tasks.append((params, board, location, term, country))

    # ── Parallel scraping ──────────────────────────────────────────────
    all_frames: list[pd.DataFrame] = []
    boards_queried: list[str] = []
    max_workers = min(settings.scrape_max_workers, len(tasks)) if tasks else 1

    logger.info(
        "Dispatching %d scrape tasks across %d workers", len(tasks), max_workers
    )

    with ThreadPoolExecutor(max_workers=max_workers) as pool:
        future_to_board = {
            pool.submit(_scrape_single, params, board, loc, term, country): board
            for params, board, loc, term, country in tasks
        }
        for future in as_completed(future_to_board):
            board = future_to_board[future]
            try:
                df = future.result()
                if df is not None:
                    all_frames.append(df)
                    if board not in boards_queried:
                        boards_queried.append(board)
            except Exception:
                logger.exception("Unexpected error collecting result for %s", board)

    if not all_frames:
        logger.warning("No jobs found across any board/location/term combo")
        return ScrapeResult(
            jobs=pd.DataFrame(),
            boards_queried=boards_queried,
        )

    combined = pd.concat(all_frames, ignore_index=True)
    total_raw = len(combined)
    logger.info("[%s] Raw scrape results: %d jobs", mode, total_raw)

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
    filtered_email = total_raw - total_after_email_filter
    logger.info(
        "[%s] After email filter: %d (removed %d no-email)",
        mode,
        total_after_email_filter,
        filtered_email,
    )

    # ── Filter: reject unwanted titles ─────────────────────────────────
    reject_patterns = settings.reject_titles
    if reject_patterns:
        mask = combined["title"].apply(
            lambda t: not _should_reject_title(t, reject_patterns)
        )
        rejected_count = (~mask).sum()
        if rejected_count > 0:
            logger.info("[%s] Title filter rejected: %d", mode, rejected_count)
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
            logger.info("[%s] Dedup removed: %d duplicate URLs", mode, dupes)
    else:
        total_deduplicated = len(combined)

    combined = combined.reset_index(drop=True)
    logger.info("[%s] Final jobs to process: %d", mode, total_deduplicated)

    return ScrapeResult(
        jobs=combined,
        total_raw=total_raw,
        total_after_email_filter=total_after_email_filter,
        total_after_title_filter=total_after_title_filter,
        total_deduplicated=total_deduplicated,
        boards_queried=boards_queried,
    )
