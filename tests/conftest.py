"""Shared pytest fixtures for jobSpy-V2 tests."""

from __future__ import annotations

from typing import Generator

import pytest

from jobspy_v2.config.settings import Settings, get_settings


@pytest.fixture()
def env_vars(monkeypatch: pytest.MonkeyPatch) -> dict[str, str]:
    """Set minimal required env vars and return them as a dict.

    Every test that needs a ``Settings`` instance should use this fixture
    (or ``settings``) to avoid leaking real ``.env`` values into tests.
    """
    values: dict[str, str] = {
        # SMTP
        "GMAIL_EMAIL": "test@example.com",
        "GMAIL_APP_PASSWORD": "test-app-password",
        # LLM
        "OPENROUTER_API_KEY": "sk-test-key",
        # Contact
        "CONTACT_NAME": "Test User",
        "CONTACT_EMAIL": "test@example.com",
        "CONTACT_PHONE": "+1-555-0100",
        "CONTACT_PORTFOLIO": "https://test.dev",
        "CONTACT_GITHUB": "https://github.com/testuser",
        # Storage
        "STORAGE_BACKEND": "csv",
        "CSV_FILE_PATH": "test_sent.csv",
        # Report
        "REPORT_EMAIL": "report@example.com",
        # General
        "DRY_RUN": "true",
        "LOG_LEVEL": "DEBUG",
        # Email
        "APPLICATION_SENDER_NAME": "Test User",
        "EMAIL_INTERVAL_SECONDS": "5",
        # Onsite
        "ONSITE_SEARCH_TERMS": "python developer,backend engineer",
        "ONSITE_LOCATIONS": "TestCity,OtherCity",
        "ONSITE_JOB_BOARDS": "indeed,google",
        "ONSITE_RESULTS_WANTED": "10",
        # Remote
        "REMOTE_SEARCH_TERMS": "remote python,remote ml",
        "REMOTE_JOB_BOARDS": "indeed,linkedin",
        "REMOTE_RESULTS_WANTED": "10",
    }
    for key, val in values.items():
        monkeypatch.setenv(key, val)
    return values


@pytest.fixture()
def settings(env_vars: dict[str, str]) -> Generator[Settings, None, None]:
    """Return a fresh ``Settings`` loaded from mocked env vars.

    Clears the ``get_settings`` LRU cache before and after the test so
    singleton state never leaks between tests.
    """
    get_settings.cache_clear()
    yield Settings(_env_file=None)
    get_settings.cache_clear()
