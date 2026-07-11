#!/usr/bin/env bash
# Push the current branch to the GitHub remote.
#
# Prerequisites:
#   1. Authenticate the GitHub CLI once:  gh auth login
#      (or configure a credential helper / SSH key for git)
#   2. Ensure the 'origin' remote points at your repository:
#        git remote -v
#
# Usage: scripts/push.sh [branch]
set -euo pipefail

BRANCH="${1:-$(git rev-parse --abbrev-ref HEAD)}"
REMOTE="origin"

if ! git remote get-url "$REMOTE" >/dev/null 2>&1; then
  echo "error: remote '$REMOTE' is not configured" >&2
  echo "add it with: git remote add origin https://github.com/Prasanna-Kumar-N-16/llm-gateway-platform.git" >&2
  exit 1
fi

echo "Pushing '$BRANCH' to $REMOTE ($(git remote get-url "$REMOTE"))..."
git push -u "$REMOTE" "$BRANCH"
echo "Done."
