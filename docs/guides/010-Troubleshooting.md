# 010-Troubleshooting

## Common Issues and Fixes

---

### 1. "scrappy not found"

**Error:**
```
[10] scrappy binary ......... ⚠ not in PATH — install from github.com/arinbalyan/scrappy
```
or scrappy workflow produces no results.

**Cause:** The scrappy binary is not in `$PATH` or the scrappy config path is not set.

**Solutions:**

**Option A: Build scrappy from source (recommended for local dev)**
```bash
# Clone scrappy alongside JobHunter
git clone https://github.com/arinbalyan/scrappy.git ../scrappy

# Build it
cd ../scrappy
go build -o scrappy ./cmd/scrappy/

# Add to PATH or set binary path
export PATH=$PWD:$PATH
```

**Option B: Install via `go install`**
```bash
go install github.com/arinbalyan/scrappy/cmd/scrappy@latest
```

**Option C: Set config path explicitly**
```bash
SCRAPPY_CONFIG=../scrappy/config.yaml go run ./cmd/scrape/
```

**How scrape finds config (in order):**
1. `$SCRAPPY_CONFIG` env var
2. `/home/nemesis/projects/scrappy/config.yaml`
3. `../scrappy/config.yaml`
4. `config.yaml` (current directory)
5. `~/.scrappy/config.yaml`

> **GitHub Actions:** The scrape workflow checks out both repos and builds scrappy from source automatically.

---

### 2. ".env not found"

**Error:**
```
[1] .env file .............. ✗ missing — create from .env.example
```

**Solution:** Run the doctor:
```bash
go run ./cmd/doctor/
```

The doctor **automatically creates** `.env` from `.env.example` if it doesn't exist. Then edit `.env` with your real API keys.

```bash
# Or do it manually
cp .env.example .env
# Then edit .env
```

---

### 3. Database Connection Failures

**Error:**
```
Failed to load config: DATABASE_URL is required
```
or
```
database connection failed: dial tcp: lookup ...
```

**Causes and fixes:**

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| `DATABASE_URL is required` | Not set in `.env` | Add `DATABASE_URL=postgres://...` to `.env` |
| `dial tcp: lookup ... no such host` | Wrong hostname | Check your NeonDB hostname in the connection string |
| `connection refused` | DB not running (local Docker) | `docker compose up -d postgres` |
| `SSL required` | Missing `sslmode` | Add `?sslmode=require` to NeonDB URLs |
| `password authentication failed` | Wrong password | Check NeonDB console for correct password |
| `timeout` | Network block | Check firewall/proxy settings |

**Test database connectivity:**
```bash
# Via doctor (checks TCP reachability)
go run ./cmd/doctor/

# Direct test
psql "$DATABASE_URL" -c "SELECT 1"
```

**Local Docker Postgres connection string:**
```
DATABASE_URL=postgres://jobhunter:jobhunter_dev@localhost:5432/jobhunter?sslmode=disable
```

**NeonDB connection string format:**
```
DATABASE_URL=postgres://user:password@ep-xxx.us-east-2.aws.neon.tech/neondb?sslmode=require
```

---

### 4. Gmail SMTP Auth Failures

**Error:**
```
sending (1/4): Software Engineer at Acme Corp -> hr@acme.com
failed: 535-5.7.8 Username and Password not accepted
```
or doctor shows:
```
[7] Gmail SMTP ............ ✗ GMAIL_USER or GMAIL_APP_PASS not set
```

**Causes and fixes:**

| Issue | Solution |
|-------|----------|
| Wrong password | Use a Gmail **App Password**, not your regular password |
| 2FA not enabled | You must enable 2FA on your Google account to use App Passwords |
| App Password expired | Generate a new one at [myaccount.google.com/apppasswords](https://myaccount.google.com/apppasswords) |
| "Less secure apps" blocked | Google removed this option -- App Passwords are the only way now |

**Generate a Gmail App Password:**
1. Go to [myaccount.google.com/apppasswords](https://myaccount.google.com/apppasswords)
2. Select "Mail" and your device
3. Click "Generate"
4. Copy the 16-character password (spaces optional)
5. Add to `.env`: `GMAIL_APP_PASS=xxxx xxxx xxxx xxxx`

---

### 5. OpenRouter API Errors

**Error:**
```
LLM generation failed: 401 Unauthorized
```
or doctor shows:
```
[8] OpenRouter API ........ ✗ status 401: {"error":{"message":"Invalid API key"}}
```

**Causes and fixes:**

| Error | Cause | Fix |
|-------|-------|-----|
| 401 Unauthorized | Invalid API key | Check `OPENROUTER_API_KEY` in `.env` |
| 402 Payment Required | Free tier exhausted | Add payment method or wait for reset |
| 429 Too Many Requests | Rate limited | Wait or add more providers for round-robin |
| 503 Service Unavailable | OpenRouter down | Retry later -- router will skip to next provider |

**Get an OpenRouter API key:**
1. Go to [openrouter.ai/keys](https://openrouter.ai/keys)
2. Create a new key
3. Copy and add to `.env`: `OPENROUTER_API_KEY=sk-or-...`

**Verify the key:**
```bash
curl -H "Authorization: Bearer $OPENROUTER_API_KEY" \
  https://openrouter.ai/api/v1/models | jq '.data | length'
```

**Fallback strategy:** If OpenRouter is down, other providers (Groq, Together, etc.) will be tried automatically. Add more API keys for redundancy.

---

### 6. LLM Router Errors

**Error:**
```
LLM generation failed: no healthy providers
```
or
```
could not load llm.yaml: open .agent-data/llm.yaml: no such file or directory
```

**Cause:** All LLM providers are failing, or the llm.yaml file is missing.

**Fixes:**

| Issue | Solution |
|-------|----------|
| No API keys configured | Add at least `OPENROUTER_API_KEY` to `.env` |
| All providers returning errors | Check network connectivity, API key validity, rate limits |
| `llm.yaml` missing | The file is optional -- providers are loaded from env vars directly. Create it if you need provider-specific model mappings |
| All providers exhausted | Add more API keys or wait for rate limits to reset |

---

### 7. Email Sending Issues

**Error:**
```
send complete: 0 sent, 0 failed
```

**Cause:** No pending items in the email queue.

**Fixes:**
```bash
# Check if there are pending items
go run ./cmd/inbox/

# If queue is empty, run scrape first to find jobs
go run ./cmd/scrape/

# Check that jobs passed filters (skipped reasons shown in scrape output)
```

**Error:**
```
send complete: 0 sent, 4 failed
```

**Cause:** Sending failed. Check logs for specific errors:

| Log Message | Cause | Fix |
|-------------|-------|-----|
| `535 Authentication failed` | Bad Gmail credentials | Regenerate App Password |
| `550 5.1.1 Recipient rejected` | Invalid recipient email | Check email addresses in config.yaml |
| `450 4.7.0 Greylisted` | Gmail throttling | Reduce `EMAIL_DELAY_SECONDS` or `MAX_EMAILS_PER_RUN` |
| `connection timed out` | Network issue | Check internet/`TRACKING_SERVER_URL` |

---

### 8. Docker Issues

**Error:**
```
Error response from daemon: Port 5432 is already in use
```

**Fix:** Change the host port in `docker-compose.yml` or stop the existing Postgres:
```bash
# Stop existing service
sudo systemctl stop postgresql

# Or change port mapping in docker-compose.yml
ports:
  - "5433:5432"
```

**Error:**
```
docker compose up -d
ERROR: Couldn't connect to Docker daemon
```

**Fix:** Ensure Docker is running:
```bash
sudo systemctl start docker
```

**Error:**
```
container "jobhunter-tracker" is unhealthy
```

**Fix:** The tracker depends on Postgres and may fail if Postgres isn't ready yet. Check Postgres logs:
```bash
docker compose logs postgres
```

---

### 9. GitHub Actions Issues

**Error in CI:**
```
Error: DATABASE_URL secret is not set
```

**Fix:** Add secrets to GitHub repository:
1. Go to Settings > Secrets and variables > Actions
2. Add `DATABASE_URL`, `GMAIL_USER`, `GMAIL_APP_PASS`, `OPENROUTER_API_KEY`
3. Run again

Or use the syncsecrets tool:
```bash
go run ./cmd/syncsecrets/
```

**Error in CI:**
```
could not find scrappy binary
```

**Fix:** The scrape workflow checks out both repos. Ensure the workflow `.github/workflows/scrape.yml` has:
```yaml
- uses: actions/checkout@v4
  with:
    repository: arinbalyan/scrappy
    path: scrappy
```

---

### 10. Vercel Deployment Issues

**Error:**
```
Error: The Runtime "@vercel/go" is not installed or not available
```

**Fix:** Ensure you're using the Serverless Function plan (not the free Hobby plan, which doesn't support Go on some regions). Try:
```bash
# Deploy with --force flag
vercel deploy --prod --force
```

**Error:**
```
Deployment failed: Build Error
```

**Fix:** The Vercel build expects the Go module to be at the root. Check `vercel.json`:
```json
{
  "builds": [
    {
      "src": "cmd/tracker/main.go",
      "use": "@vercel/go"
    }
  ]
}
```

**Error after deployment:**
```
GET /track?id=... 500 Internal Server Error
```

**Fix:** Set environment variables in Vercel project settings:
```
DATABASE_URL=postgres://...
TRACKING_SERVER_PORT=8080
```

---

## Diagnostic Checklist

If something isn't working, run these in order:

```bash
# 1. Run doctor (comprehensive 15-point check)
go run ./cmd/doctor/

# 2. Check database connectivity
docker compose up -d postgres
# or test with psql

# 3. Run a single scrape to verify scrappy works
go run ./cmd/scrape/

# 4. Check telemetry
go run ./cmd/inbox/

# 5. Try a dry-run send
go run ./cmd/send/ --dry-run
```

## Getting Help

If issues persist:

- Check the doctor output for specific `✗` failures
- Run `go run ./cmd/inbox/` for current system state
- Check Docker logs: `docker compose logs`
- File an issue or PR on the repository
