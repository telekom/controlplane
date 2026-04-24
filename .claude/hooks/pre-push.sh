#!/usr/bin/env bash
# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Claude Code pre-push hook: verifies affected modules and commit messages.
# Exit 2 = block the push, exit 0 = allow.

set -euo pipefail

cd "$CLAUDE_PROJECT_DIR"

# --- Determine merge base ---
UPSTREAM=$(git rev-parse --abbrev-ref "@{upstream}" 2>/dev/null || echo "")

if [ -n "$UPSTREAM" ]; then
  MERGE_BASE=$(git merge-base "$UPSTREAM" HEAD 2>/dev/null || echo "")
else
  DEFAULT_BRANCH=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null \
    | sed 's@^refs/remotes/origin/@@' || echo "main")
  MERGE_BASE=$(git merge-base "origin/$DEFAULT_BRANCH" HEAD 2>/dev/null || echo "")
fi

# --- 1. Verify affected Go modules ---
if [ -n "$MERGE_BASE" ]; then
  CHANGED_FILES=$(git diff --name-only "$MERGE_BASE..HEAD" -- '*.go')
else
  CHANGED_FILES=$(git diff --name-only HEAD~1..HEAD -- '*.go' 2>/dev/null || true)
fi

if [ -n "$CHANGED_FILES" ]; then
  AFFECTED=()
  while IFS= read -r mod_dir; do
    if [ -f "$mod_dir/Makefile" ] && echo "$CHANGED_FILES" | grep -q "^${mod_dir}/"; then
      AFFECTED+=("$mod_dir")
    fi
  done < <(find . -name 'go.mod' -not -path './docs/*' -not -path '*/node_modules/*' -not -path './tools/*' -exec dirname {} \; | sed 's|^\./||' | sort)

  if [ ${#AFFECTED[@]} -gt 0 ]; then
    if ! hack/verify.sh "${AFFECTED[@]}"; then
      echo "Verification failed. Push blocked." >&2
      exit 2
    fi
  fi
fi

# --- 2. Validate commit messages (conventional commits) ---
if [ -n "$MERGE_BASE" ]; then
  COMMITS=$(git log "$MERGE_BASE..HEAD" --format="%s" 2>/dev/null || true)
else
  COMMITS=$(git log HEAD~1..HEAD --format="%s" 2>/dev/null || true)
fi

CONVENTIONAL="^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\(.+\))?(!)?: .+"
while IFS= read -r msg; do
  [ -z "$msg" ] && continue
  if ! echo "$msg" | grep -qE "$CONVENTIONAL"; then
    echo "Commit message does not follow Conventional Commits: '$msg'" >&2
    echo "Expected format: <type>(<optional scope>): <description>" >&2
    exit 2
  fi
done <<< "$COMMITS"

exit 0
