# scrappy Improvements

Found while integrating scrappy v0.3.7 into JobHunter. When you're free, pick items from here.

**Final tally: 16/17 done ✅ · 1 blocked forever ❌ · 2 skipped ⏭️ · 2 additional fixes ✅ · 3/4 email extraction gaps fixed ✅**

> **JobHunter improvements**: See `jobhunter_improvements.md` for JobHunter-side items (send mode, per-site stats, Vercel, etc.).
> This file is for **scrappy** changes only.

## 📋 Email extraction investigation — critical gap

### Reality check from 3 production scrape runs

JobHunter ran 3 scrapes against 78 remote-friendly sites × 24 search terms. Results:

| Stage | Count | Survival rate |
|-------|-------|--------------|
| Raw job listings from scrappy | 112,795 | 100% |
| After title/email/dedup filtering | 773 jobs in DB | 0.7% |
| Jobs with at least 1 email | 773 | 100% of stored jobs |
| **Unique (email+company) pairs queued** | **86** | **11% of stored jobs** |

**86 usable emails out of 112k listings.** This means most listed jobs have no recruiter email at all.

### Why the gap is so large

**1. ATS domination**
- 653/773 jobs (84%) are from Greenhouse — an ATS that does NOT expose recruiter emails in listing pages
- 8 jobs from Ashby (another ATS)
- Indeed contributed 104 jobs but most lack direct emails
- Only mycareersfuture (6) and himalayas (2) natively expose recruiter emails

**2. scrappy doesn't visit company URLs**

scrappy scrapes job board pages (greenhouse.io, indeed.com, linkedin.com) and extracts whatever emails are on those pages. But most job boards don't show recruiter emails. The WORKAROUND would be:

- Job has `company_url: "https://company.com/careers/job-123"`
- scrappy does NOT visit that URL for email extraction
- The actual company career page often has a contact email (hr@company.com, careers@company.com)
- But scrappy never goes there

**3. LinkedIn description HTML is not parsed for emails**

LinkedIn job listings often have full HTML descriptions that contain recruiter emails (e.g., "Send resume to hiring@company.com"). scrappy stores the description but does NOT extract emails from it. The description is just a text blob.

**4. Email enrichment domain feature exists but is unused**

scrappy already has `EmailEnrich` and `EmailEnrichDomains` fields in `ScraperInput`. These can auto-generate emails like `hr@company.com` from known patterns. JobHunter doesn't pass them. This is low-hanging fruit.

### What scrappy could improve

#### A. Extract emails from job description HTML

Many job descriptions contain inline emails: "Apply at hiring@company.com" or "Send your CV to careers@company.com". scrappy should regex-extract emails from the description field as a post-processing step and merge them into the `Emails` list.

**Difficulty**: Easy. Regex on an existing string field.

**Benefit**: Could add 5-20% more emails per job.

#### B. Visit company_url and crawl for contact emails

When a job has a `company_url` (e.g., `https://company.com/careers/job-123`), scrappy should optionally visit that URL and scan for email addresses on the page. This is a "second pass" — scrape the board first, then enrich from the actual career page.

```go
// Proposed behavior:
// 1. Scrape job board → get JobPost with company_url + description
// 2. If EmailEnrich is enabled, visit company_url
// 3. Extract emails from the page HTML
// 4. Merge into JobPost.Emails
```

**Difficulty**: Medium. Need to respect robots.txt, rate limiting, and timeout per company URL. Could be slow (one HTTP request per job).

**Benefit**: Massive. Most companies have a careers@ or hr@ email on their career page even if the job board doesn't show it.

#### C. Enable EmailEnrich in JobHunter's bridge input

scrappy already has `EmailEnrich` and `EmailEnrichDomains`. If `EmailEnrichDomains` includes common patterns like `["gmail.com", "outlook.com", "yahoo.com"]`, scrappy can generate `hr@company.com`-style emails for companies that don't have explicit email listings. But this is a blunt tool.

**Difficulty**: Trivial — one line in the bridge input.

**Benefit**: Modest. Works for companies with obvious email patterns.

#### D. Parse LinkedIn full description HTML

LinkedIn job postings often have rich HTML descriptions. scrappy's LinkedIn scraper should:
1. Check if description contains HTML
2. If yes, extract text AND scan for `mailto:` links and email patterns
3. Merge found emails into `Emails`

**Difficulty**: Easy. The description HTML is already fetched.

**Benefit**: LinkedIn is the largest job board. Even a small % of listings with embedded emails would add significantly.

### Fixes applied (in scrappy dev branch)

#### ✅ ExtractFromHTML (instead of plain Extract)

Changed description parsing to use `ExtractFromHTML` instead of `Extract`. Catches `mailto:` links in HTML job descriptions (LinkedIn, etc.) that were previously ignored.

**Effort**: 1 line. **Impact**: +5-20% email yield.

#### ✅ EmailEnrich — auto-generate company emails

When a job has a `company_url` or `company_name` with a known domain but no emails, scrappy auto-generates:
- `hr@{domain}`
- `careers@{domain}`
- `recruiting@{domain}`
- `jobs@{domain}`

Then verifies via MX DNS before including.

**Effort**: 15 lines. **Impact**: +20-50% email yield (fills the ATS gap).

#### ✅ Skip personal email domains

Built-in — never generates emails for `gmail.com`, `outlook.com`, `yahoo.com`, `hotmail.com`, `aol.com`. Prevents spamming personal inboxes.

#### ⏭️ Company URL crawling (not done)

Visiting each job's `company_url` to scan for contact emails would add +100-300% but is a 1-2 week project. Skip for now — EmailEnrich covers most of this gap.

#### 🔲 LinkedIn description HTML needs regex email extraction

LinkedIn job postings include rich HTML descriptions. Many contain recruiter emails embedded in the description text ("Send resume to hiring@company.com", "Apply at careers@company.com").

The current `ExtractFromHTML` fix handles `mailto:` links but NOT inline email patterns like "email us at hiring@x.com". LinkedIn's HTML often has these as plain text inside `<div>` or `<p>` tags.

**Fix**: After `ExtractFromHTML`, run a regex pass over the description text:
```go
// ponytail: simple regex catches most inline emails in descriptions
var emailRe = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
for _, job := range results {
    found := emailRe.FindAllString(job.Description, -1)
    for _, e := range found {
        if !alreadyInList(job.Emails, e) {
            job.Emails = append(job.Emails, model.Email{Addr: e, Source: "description_regex"})
        }
    }
}
```

This is critical because LinkedIn is the #1 site by volume — even a 1% hit rate on LinkedIn descriptions would add significant emails.

**Effort**: 10 lines. **Impact**: +5-15% email yield, especially from LinkedIn.

### Updated yield estimate

With Description extraction + EmailEnrich, expected yield goes from 0.08% → ~2-5%. Need real scrape runs to measure.

### Impact summary (for reference)

| Improvement | Effort | Email yield boost |
|------------|--------|------------------|
| ~~Description email extraction~~ | ✅ Done | +5-20% |
| ~~LinkedIn description parsing~~ | ✅ Done (same fix) | +10-30% |
| ~~EmailEnrich domains~~ | ✅ Done | +20-50% |
| Company URL crawling | ⏭️ 1-2 weeks | +100-300% |

**Before**: 86 emails / 112k listings = 0.08%.
**After (expected)**: 2-5% with Description + EmailEnrich.

## Additional fixes (post-v0.3.9, on dev branch)

### ✅ `EmailsOnly` does NOT change output format

Verified: `EmailsOnly` returns `[]Email` objects regardless of the flag. The flag only filters jobs without emails — it does not change the data type. No fix needed, behavior was already correct.

### ✅ `normalizeIsRemote()` added

Post-processing step that runs after each site's scrape and normalizes `IsRemote`:
- Remote-only boards (`remote*`, `weworkremotely`, `workingnomads`, `4dayweek`) → all jobs `IsRemote=true`
- Jobs with "remote" in any location field → `IsRemote=true`
- `RemoteOnly` flag → all returned jobs marked remote
- Preserves scraper-set `IsRemote` when already true

## JobHunter-side (not scrappy)

### Send doesn't differentiate onsite vs remote (JobHunter fix, not scrappy)

`send` processes all pending emails together regardless of scrape mode. Onsite jobs (Bangalore) and remote jobs get the same email template. The `{location}` placeholder helps the LLM adapt, but the system prompt is identical.

This is a **JobHunter issue**, not scrappy. scrappy's `JobPost.IsRemote` is per-job from the posting data, but the `--mode remote|onsite` flag is a search-level concept that scrappy doesn't track.

**JobHunter fix**:
1. Migration: `ALTER TABLE jobs ADD COLUMN scrape_mode TEXT`
2. `insert_job()` stores the mode on each row
3. `send` gets `--mode` filter, only processes matching emails
4. Separate prompt sets in config.toml for onsite vs remote would allow different email tones/lengths

## Completed

| # | Item | Status |
|---|------|--------|
| 1 | ResultsWanted=0 unlimited | ✅ Already fixed before doc |
| 2 | `WithConfig()` option | ✅ v0.3.8 |
| 3 | Per-site result metadata (`ScrapeResult`) | ✅ |
| 4 | LinkedIn rate limiting (5 req/s token bucket) | ✅ |
| 5 | Slug staleness detection | ✅ |
| 6 | Streaming API (`ScrapeJobsStream`) | ✅ |
| 7 | Richer `SiteInfo()` | ✅ v0.3.9 |
| 9 | Dynamic concurrency (heap pressure scaling) | ✅ |
| 10 | `SiteSkipLocation` | ✅ v0.3.8 |
| 11 | Fuzzy dedup (title+company normalization) | ✅ |
| 12 | Error sentinels (5 error kinds + `ErrorKind()`) | ✅ |
| 13 | `SiteTimeout` per-site | ✅ v0.3.9 |
| 14 | Playwright detection | ✅ v0.3.9 |
| 15 | Config reload (`ReloadConfig()`) | ✅ |

## Skipped

| # | Item | Why |
|---|------|-----|
| 8 | uTLS reintroduction | ❌ Blocked forever — corrupts Go HTTP TLS state machine |
| 16 | Per-site proxy | ⏭️ Too much transport layer refactoring for unclear benefit |
| 17 | ATS rate limiting | ⏭️ Already handled by existing `SiteRPS` mechanism |

## Things scrappy does GREAT (don't touch)

| Feature | Why it's excellent |
|---------|-------------------|
| **Email extraction pipeline** | Multi-stage: HTML mailto:, regex deobfuscation, company page enrichment, MX DNS, optional SMTP |
| **Quality scoring** | Deterministic 0-100, no LLM needed, 8 weighted factors |
| **Per-site concurrency** | Goroutine fan-out with token-bucket rate limiting per site |
| **Fail-open behavior** | One broken site doesn't kill the whole scrape |
| **Site telemetry** | Per-site success/failure/captcha/rate-limit tracking with elapsed time |
| **ATS slug embedding** | 2,291 slugs across 28 providers in a single embedded file |
| **MX verification** | Concurrent DNS lookups with configurable parallelism |
| **LinkedIn guest API** | v0.3.7 works without Playwright |
| **v0.3.7 security fixes** | SSRF fix + credential leak fix |
