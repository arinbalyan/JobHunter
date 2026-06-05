# 003-Email Tracking

## Overview
JobHunter tracks every sent email through a multi-layered system:

1. **Delivery** — confirmed immediately after SMTP success
2. **Open** — detected via invisible tracking pixel
3. **Click** — detected via redirected links
4. **Bounce** — detected by scanning inbox for Mailer-Daemon responses
5. **Reply** — detected by scanning inbox for "Re:" messages

## Architecture

```
┌──────────────────┐       SMTP        ┌────────────┐
│   JobHunter      │ ────────────────→ │   Gmail    │
│   (cmd/sender)   │                    │   SMTP     │
│                  │                    │            │
│   Injects:       │                    │  Email has │
│   • Pixel URL    │                    │  hidden    │
│   • Click URLs   │                    │  tracking  │
│   • Message-ID   │                    │  pixel     │
└──────────────────┘                    └─────┬──────┘
                                              │
        Recipient opens email ────────────────┘
        → loads <img src="https://tracker/track?id=...">
                                              │
                                              ▼
                                  ┌──────────────────┐
                                  │  Tracking Server │
                                  │  (cmd/tracker)   │
                                  │                  │
                                  │  /track?id →     │
                                  │    logs open     │
                                  │    returns GIF   │
                                  │                  │
                                  │  /click?id&url→  │
                                  │    logs click    │
                                  │    302 redirect  │
                                  └──────────────────┘
                                              │
                                              ▼
                                  ┌──────────────────┐
                                  │  IMAP Scanner    │
                                  │  (in cmd/sender) │
                                  │                  │
                                  │  Searches inbox  │
                                  │  for bounce msgs │
                                  │  and replies     │
                                  │  → updates DB    │
                                  └──────────────────┘
```

## Tracking Pixel
A 1×1 transparent GIF is injected into every HTML email:

```html
<img src="https://your-tracker.com/track?id=uuid-here"
     width="1" height="1" alt=""
     style="display:none;" />
```

When the recipient loads the email, their mail client fetches this image.
The tracking server logs the open event and returns a 43-byte transparent GIF.

## Click Tracking
Links in emails use the tracking server as a redirect:

```
https://your-tracker.com/click?id=uuid&url=https://company.com/jobs/123
```

1. Recipient clicks → goes to tracking server
2. Tracking server logs the click
3. Server responds with HTTP 302 redirect to the original URL

## Bounce Detection
The IMAP scanner searches Gmail inbox for:

| Condition | Detection | Action |
|-----------|-----------|--------|
| From: `mailer-daemon` | Hard/soft bounce | Mark email as `bounced` |
| Subject: `Delivery Status Notification` | Bounce detail | Classify bounce type |
| Subject: `Re:` | Human reply | Mark email as `replied` |

The scanner extracts the original `Message-ID` from bounce/reply headers
and matches it against the `emails.message_id` column in the database.

## Deployment

### Option 1: All-in-One (Simple)
```bash
# Tracking server on :8080
go run ./cmd/tracker &

# Agent
go run ./cmd/sender
```

### Option 2: Separate (Production)
Deploy the tracking server to Railway/Render free tier:
```bash
# Deploy cmd/tracker as a web service
# Set TRACKING_SERVER_PORT=8080
# Set TRACKING_SERVER_URL=https://your-app.railway.app
```

## Database Schema
The `emails` table tracks everything:
```sql
emails (
    tracking_id     TEXT UNIQUE,  -- UUID for pixel/click tracking
    message_id      TEXT,         -- SMTP Message-ID for bounce matching
    status          TEXT,         -- pending | sent | failed | bounced | opened | clicked | replied
    opened          BOOLEAN,
    opened_at       TIMESTAMPTZ,
    clicked         BOOLEAN,
    clicked_at      TIMESTAMPTZ,
    replied         BOOLEAN,
    replied_at      TIMESTAMPTZ,
    bounced         BOOLEAN,
    bounced_at      TIMESTAMPTZ,
    bounce_type     TEXT          -- hard_bounce | soft_bounce | spam_blocked
)
```
