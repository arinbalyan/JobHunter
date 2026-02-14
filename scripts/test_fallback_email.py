"""Send a real test email using FALLBACK mode to REPORT_EMAIL.

Usage:
    uv run python scripts/test_fallback_email.py
"""

from __future__ import annotations

import sys
from pathlib import Path

# Ensure src/ is on sys.path when running as a script
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "src"))

from jobspy_v2.config.settings import get_settings  # noqa: E402
from jobspy_v2.core.email_gen import generate_email  # noqa: E402
from jobspy_v2.core.email_sender import send_email  # noqa: E402

# ── Fake job posting to simulate real usage ──────────────────────────────────
FAKE_JOB_TITLE = "Backend Python Developer"
FAKE_COMPANY = "Acme Corp (Test)"
FAKE_DESCRIPTION = (
    "We are looking for a Backend Python Developer to join our team. "
    "You will build REST APIs with FastAPI, work with PostgreSQL, "
    "and deploy services on AWS. Experience with Docker and CI/CD is a plus."
)


def main() -> int:
    settings = get_settings().model_copy(update={"email_generator_mode": "fallback"})
    to_email = settings.report_email

    print("=" * 60)
    print("FALLBACK EMAIL TEST")
    print("=" * 60)
    print(f"To:   {to_email}")
    print(f"Mode: {settings.email_generator_mode}")
    print(f"Resume PDF: {settings.resume_file_path}")
    print()

    # Generate email
    result = generate_email(
        job_title=FAKE_JOB_TITLE,
        company=FAKE_COMPANY,
        job_description=FAKE_DESCRIPTION,
        settings=settings,
    )

    print(f"Subject: {result.subject}")
    print(f"Word count: {result.word_count}")
    print(f"Mode used: {result.mode}")
    print("-" * 60)
    print(result.body)
    print("-" * 60)

    # Send email
    print(f"\nSending to {to_email}...")
    success, error = send_email(
        to_email=to_email,
        subject=f"[TEST-FALLBACK] {result.subject}",
        body=result.body,
        settings=settings,
        resume_path=settings.resume_file_path,
    )

    if success:
        print(f"✓ Email sent successfully to {to_email}")
        return 0
    else:
        print(f"✗ Failed to send: {error}")
        return 1


if __name__ == "__main__":
    sys.exit(main())
