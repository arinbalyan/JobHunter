#!/usr/bin/env bash
# promote.sh <from_branch> <to_branch> [--force] [--pr-only]
set -euo pipefail

FROM=$1
TO=$2
FORCE=${3:-}
PR_ONLY=${4:-}

git fetch origin "$FROM" "$TO"

AHEAD=$(git rev-list --count "origin/$TO..origin/$FROM" 2>/dev/null || echo 0)
if [ "$AHEAD" -eq 0 ]; then
  echo "no new commits on $FROM since last $TO promotion — skipping"
  exit 0
fi

if [ "$FORCE" != "--force" ]; then
  LAST_TS=$(git log -1 --format=%ct "origin/$FROM")
  NOW=$(date +%s)
  AGE_DAYS=$(( (NOW - LAST_TS) / 86400 ))
  echo "latest $FROM commit is ${AGE_DAYS}d old"
  if [ "$AGE_DAYS" -lt 7 ]; then
    echo "not ready — only ${AGE_DAYS}d old, need 7d"
    exit 0
  fi
fi

LOG=$(git log --oneline "origin/$TO..origin/$FROM" 2>/dev/null || echo "")
COUNT=$(echo "$LOG" | wc -l)

PR_URL=$(gh pr list --head "$FROM" --base "$TO" --json url --jq '.[0].url' 2>/dev/null || echo "")
if [ -z "$PR_URL" ]; then
  gh pr create \
    --head "$FROM" --base "$TO" \
    --title "promote: $FROM → $TO" \
    --body "## Promote: $FROM → $TO

### Commits ($COUNT)

$LOG

---
Automated promotion." 2>/dev/null || true
  PR_URL=$(gh pr list --head "$FROM" --base "$TO" --json url --jq '.[0].url' 2>/dev/null || echo "")
fi

if [ -n "$PR_URL" ]; then echo "PR: $PR_URL"; fi

if [ "$PR_ONLY" = "--pr-only" ]; then
  echo "PR created — merge manually"
  exit 0
fi

git fetch origin "$TO"
git checkout -B temp-merge "origin/$TO"
git merge --no-ff "origin/$FROM" -m "promote: $FROM → $TO"
git push origin "temp-merge:$TO"
git branch -D temp-merge 2>/dev/null || true
echo "promoted $FROM → $TO"
