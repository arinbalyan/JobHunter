"""Core business logic modules."""

from jobspy_v2.core.dedup import Deduplicator
from jobspy_v2.core.email_gen import generate_email
from jobspy_v2.core.email_sender import send_email
from jobspy_v2.core.reporter import send_report
from jobspy_v2.core.scraper import scrape_jobs

__all__ = [
    "Deduplicator",
    "generate_email",
    "scrape_jobs",
    "send_email",
    "send_report",
]
