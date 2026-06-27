#!/usr/bin/env bash
# sync-all.sh — promote dev→beta→main via auto PRs.
# Run from dev branch after pushing changes.
set -euo pipefail

cd "$(dirname "$0")"

promote() {
  local FROM=$1 TO=$2
  local NO_MERGE=${3:-}

  echo ""
  echo "=== $FROM → $TO ==="
  git fetch origin "$FROM" "$TO" 2>/dev/null || true

  if git diff --quiet "origin/$TO" "origin/$FROM"; then
    echo "no content differences — skipping"
    return
  fi

  local LOG=$(git log --oneline "origin/$TO..origin/$FROM" 2>/dev/null || echo "")
  local COUNT=$(echo "$LOG" | wc -l)
  local BODY="## Promote: $FROM → $TO

### Commits ($COUNT)

$LOG

---
Automated promotion."

  local PR_URL=$(gh pr list --head "$FROM" --base "$TO" --json url --jq '.[0].url' 2>/dev/null || echo "")
  if [ -z "$PR_URL" ]; then
    gh pr create --head "$FROM" --base "$TO" --title "promote: $FROM → $TO" --body "$BODY" 2>/dev/null || true
    PR_URL=$(gh pr list --head "$FROM" --base "$TO" --json url --jq '.[0].url' 2>/dev/null || echo "")
  fi
  [ -z "$PR_URL" ] && echo "failed to create PR — skipping" && return
  echo "PR: $PR_URL"

  if [ "$NO_MERGE" = "--no-merge" ]; then
    echo "PR created — merge manually"; return
  fi

  gh pr review "$PR_URL" --approve 2>/dev/null || true
  sleep 1
  gh pr merge "$PR_URL" --merge 2>/dev/null || true

  echo "waiting for merge..."
  for i in $(seq 1 60); do
    sleep 2
    STATE=$(gh pr view "$PR_URL" --json state --jq '.state' 2>/dev/null || echo "")
    [ "$STATE" = "MERGED" ] && echo "merged" && git fetch origin "$TO" 2>/dev/null || true && break
  done
  echo "done: $FROM → $TO"
}

git fetch origin dev beta main 2>/dev/null || git fetch --all
promote dev beta
promote beta main --no-merge
echo "" && echo "Chain complete. Beta→main PR is open — merge manually."
