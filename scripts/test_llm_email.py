"""Send a real test email using LLM mode to REPORT_EMAIL.

Usage:
    uv run python scripts/test_llm_email.py
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
FAKE_JOB_TITLE = "ML Engineer"
FAKE_COMPANY = "NovaTech AI (Test)"
FAKE_DESCRIPTION = (
    "NovaTech AI is hiring an ML Engineer to design and deploy machine learning "
    "models at scale. You will work with PyTorch, build training pipelines, "
    "implement model serving with FastAPI, and optimize inference on GPU clusters. "
    "Experience with RAG systems, vector databases, and LLM fine-tuning is a plus. "
    "We use AWS SageMaker, Docker, and Kubernetes for our MLOps infrastructure."
)


def main() -> int:
    settings = get_settings().model_copy(update={"email_generator_mode": "llm"})
    to_email = settings.report_email

    # Load context (same as workflow does)
    context = ""
    context_path = Path(settings.context_file_path)
    if context_path.exists():
        context = context_path.read_text(encoding="utf-8")
        print(f"Loaded context from {context_path} ({len(context)} chars)")
    else:
        print(f"Warning: context file not found at {context_path}")

    print("=" * 60)
    print("LLM EMAIL TEST")
    print("=" * 60)
    print(f"To:    {to_email}")
    print(f"Mode:  {settings.email_generator_mode}")
    print(f"Model: {settings.llm_model}")
    print(f"Resume PDF: {settings.resume_file_path}")
    print()

    # Generate email
    result = generate_email(
        job_title=FAKE_JOB_TITLE,
        company=FAKE_COMPANY,
        job_description=FAKE_DESCRIPTION,
        settings=settings,
        context=context,
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
        subject=f"[TEST-LLM] {result.subject}",
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
