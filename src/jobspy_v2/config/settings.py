"""Pydantic-based settings loaded entirely from environment variables.

All configuration is read from ``.env`` (or real env vars). No hardcoded
values — every field has a sensible default or is validated as required.

Usage::

    from jobspy_v2.config import get_settings
    settings = get_settings()
"""

from __future__ import annotations

from functools import lru_cache
from typing import Annotated, Literal

from pydantic import Field, field_validator, model_validator
from pydantic_settings import BaseSettings, NoDecode, SettingsConfigDict

from jobspy_v2.config.defaults import (
    DEFAULT_EMAIL_FILTER_PATTERNS,
    DEFAULT_REJECT_TITLES,
)

# ---------------------------------------------------------------------------
# Type alias: env var string "a,b,c" → list[str]
# ---------------------------------------------------------------------------
CsvList = Annotated[list[str], NoDecode]


class Settings(BaseSettings):
    """Central configuration — every field maps to an UPPER_SNAKE env var."""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
        case_sensitive=False,
        env_ignore_empty=True,
    )

    # -- SMTP ---------------------------------------------------------------
    gmail_email: str = ""
    gmail_app_password: str = ""
    smtp_host: str = "smtp.gmail.com"
    smtp_port: int = 587

    # -- LLM ----------------------------------------------------------------
    openrouter_api_key: str = ""
    llm_base_url: str = "https://openrouter.ai/api/v1"
    llm_model: str = "qwen/qwen3-next-80b-a3b-instruct:free"
    email_generator_mode: Literal["llm", "fallback"] = "llm"

    # -- Contact Info -------------------------------------------------------
    contact_name: str = ""
    contact_email: str = ""
    contact_phone: str = ""
    contact_portfolio: str = ""
    contact_github: str = ""
    contact_linkedin: str = ""
    contact_codolio: str = ""
    resume_drive_link: str = ""

    # -- Context & Resume ---------------------------------------------------
    context_file_path: str = "contexts/profile.md"
    resume_file_path: str = "ArinBalyan.pdf"

    # -- Storage ------------------------------------------------------------
    storage_backend: Literal["sheets", "csv"] = "sheets"
    google_credentials_json: str = ""
    google_sheet_name: str = "JobSpy Data"
    csv_file_path: str = "sent_emails.csv"

    # -- Report -------------------------------------------------------------
    report_email: str = ""

    # -- Proxy --------------------------------------------------------------
    proxy_list: CsvList = Field(default_factory=list)

    # -- General ------------------------------------------------------------
    skip_weekends: bool = False
    dry_run: bool = False
    log_level: str = "INFO"
    scrape_max_workers: int = 150  # Increased for faster scraping

    # -- Email Settings -----------------------------------------------------
    min_email_words: int = 120
    max_email_words: int = 300
    email_interval_seconds: int = 30
    application_sender_name: str = ""

    # -- Fallback Email Template --------------------------------------------
    fallback_email_subject: str = "Software Engineer - Exploring Opportunities"
    # Workflow-specific fallback bodies (preferred); fallback_email_body kept for
    # backward-compatibility — used when the workflow-specific field is empty.
    onsite_fallback_email_body: str = ""
    remote_fallback_email_body: str = ""
    fallback_email_body: str = ""  # legacy / generic fallback

    # -- Job Filter ---------------------------------------------------------
    reject_titles: CsvList = Field(default_factory=list)
    email_filter_patterns: CsvList = Field(default_factory=list)

    # -- Scheduler / Deployment ---------------------------------------------
    scheduler_enabled: bool = False
    scheduler_onsite_cron: str = "30 2 * * *"
    scheduler_remote_cron: str = "0 13 * * *"
    health_check_enabled: bool = True
    health_check_port: int = 10000
    health_check_path: str = "/health"

    # -- Onsite Settings ----------------------------------------------------
    onsite_search_terms: CsvList = Field(default_factory=list)
    onsite_locations: CsvList = Field(default_factory=list)
    onsite_job_type: str = "fulltime"
    onsite_job_boards: CsvList = Field(default_factory=list)
    onsite_country_indeed: str = "India"
    onsite_results_wanted: int = 1000
    onsite_hours_old: int = 4382
    onsite_max_emails_per_day: int = 500

    # -- Pending Jobs Processing ---------------------------------------------
    # Max total emails (remote + onsite) per day before stopping pending jobs
    daily_total_emails_limit: int = 500

    # -- Time Limits ---------------------------------------------------------
    # Max runtime in minutes before gracefully stopping (5h 50min = 10min buffer)
    max_runtime_minutes: int = 350

    # -- Remote Settings ----------------------------------------------------
    remote_search_terms: CsvList = Field(default_factory=list)
    remote_locations: CsvList = Field(default_factory=list)
    remote_is_remote: bool = True
    remote_job_type: str = "fulltime"
    remote_job_boards: CsvList = Field(default_factory=list)
    remote_countries_indeed: CsvList = Field(default_factory=list)
    remote_results_wanted: int = 1000
    remote_hours_old: int = 4382
    remote_max_emails_per_day: int = 250

    # -- CSV field parsing --------------------------------------------------
    @field_validator(
        "proxy_list",
        "reject_titles",
        "email_filter_patterns",
        "onsite_search_terms",
        "onsite_locations",
        "onsite_job_boards",
        "remote_search_terms",
        "remote_locations",
        "remote_countries_indeed",
        "remote_job_boards",
        mode="before",
    )
    @classmethod
    def split_csv(cls, value: object) -> list[str]:
        """Convert comma-separated env string to list."""
        if isinstance(value, str):
            return [item.strip() for item in value.split(",") if item.strip()]
        if isinstance(value, list):
            return value
        return []

    # -- Apply built-in defaults when ENV is empty --------------------------
    @model_validator(mode="after")
    def apply_defaults(self) -> Settings:
        """Fill empty list fields with built-in defaults from defaults.py."""
        if not self.reject_titles:
            self.reject_titles = list(DEFAULT_REJECT_TITLES)
        if not self.email_filter_patterns:
            self.email_filter_patterns = list(DEFAULT_EMAIL_FILTER_PATTERNS)
        return self


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    """Return a cached singleton Settings instance.

    Call ``get_settings.cache_clear()`` in tests to reset.
    """
    return Settings()
