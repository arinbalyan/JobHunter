"""SMTP email sender with retry logic."""

from __future__ import annotations

import logging
import smtplib
import time
from email.mime.application import MIMEApplication
from email.mime.multipart import MIMEMultipart
from email.mime.text import MIMEText
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from jobspy_v2.config.settings import Settings

logger = logging.getLogger(__name__)

MAX_RETRIES = 3
RETRY_DELAY_SECONDS = 5


def send_email(
    *,
    to_email: str,
    subject: str,
    body: str,
    settings: Settings,
    resume_path: str | None = None,
) -> tuple[bool, str]:
    """
    Send an email via SMTP with retry logic.

    Returns (success, error_message). error_message is empty on success.
    """
    msg = _build_message(
        from_email=settings.gmail_email,
        to_email=to_email,
        subject=subject,
        body=body,
        sender_name=settings.application_sender_name,
        resume_path=resume_path,
    )

    for attempt in range(1, MAX_RETRIES + 1):
        try:
            _send_via_smtp(
                msg=msg,
                host=settings.smtp_host,
                port=settings.smtp_port,
                email=settings.gmail_email,
                password=settings.gmail_app_password,
            )
            logger.info("Email sent to %s (attempt %d)", to_email, attempt)
            return True, ""

        except smtplib.SMTPAuthenticationError as exc:
            error = f"Authentication failed: {exc}"
            logger.error(error)
            return False, error  # No retry for auth failures

        except smtplib.SMTPServerDisconnected as exc:
            error = f"Server disconnected (attempt {attempt}): {exc}"
            logger.warning(error)
            if attempt < MAX_RETRIES:
                time.sleep(RETRY_DELAY_SECONDS)

        except smtplib.SMTPException as exc:
            error = f"SMTP error (attempt {attempt}): {exc}"
            logger.warning(error)
            if attempt < MAX_RETRIES:
                time.sleep(RETRY_DELAY_SECONDS)

        except Exception as exc:
            error = f"Unexpected error (attempt {attempt}): {exc}"
            logger.warning(error)
            if attempt < MAX_RETRIES:
                time.sleep(RETRY_DELAY_SECONDS)

    final_error = f"Failed to send to {to_email} after {MAX_RETRIES} attempts"
    logger.error(final_error)
    return False, final_error


def _build_message(
    *,
    from_email: str,
    to_email: str,
    subject: str,
    body: str,
    sender_name: str,
    resume_path: str | None,
) -> MIMEMultipart:
    """Build MIME message with optional PDF attachment."""
    msg = MIMEMultipart()
    msg["From"] = f"{sender_name} <{from_email}>"
    msg["To"] = to_email
    msg["Subject"] = subject
    msg.attach(MIMEText(body, "plain", "utf-8"))

    if resume_path:
        _attach_pdf(msg, resume_path)

    return msg


def _attach_pdf(msg: MIMEMultipart, resume_path: str) -> None:
    """Attach a PDF file to the message."""
    path = Path(resume_path)
    if not path.exists():
        logger.warning("Resume file not found: %s", path)
        return

    try:
        with open(path, "rb") as f:
            pdf = MIMEApplication(f.read(), _subtype="pdf")
            pdf.add_header("Content-Disposition", "attachment", filename=path.name)
            msg.attach(pdf)
            logger.debug("Attached resume: %s", path.name)
    except Exception:
        logger.exception("Failed to attach resume: %s", path)


def _send_via_smtp(
    *,
    msg: MIMEMultipart,
    host: str,
    port: int,
    email: str,
    password: str,
) -> None:
    """Send message through SMTP with STARTTLS."""
    with smtplib.SMTP(host, port, timeout=30) as server:
        server.ehlo()
        server.starttls()
        server.ehlo()
        server.login(email, password)
        server.send_message(msg)
