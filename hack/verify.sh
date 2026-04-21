#!/usr/bin/env bash
# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Repo-level verification: runs all checks that must pass before pushing.
# Usage: hack/verify.sh [module...]
# Without arguments, checks all Go modules (excluding docs/ and tools/).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

ERRORS=0
FAILED=()

step() { printf "\n=== %s ===\n" "$1"; }
fail() { ERRORS=1; FAILED+=("$1"); echo "FAIL: $1" >&2; }

# --- Discover modules (only those with a Makefile) ---
if [ $# -gt 0 ]; then
  MODULES=("$@")
else
  MODULES=()
  while IFS= read -r dir; do
    [ -f "$dir/Makefile" ] && MODULES+=("$dir")
  done < <(find . -name 'go.mod' -not -path './docs/*' -not -path '*/node_modules/*' -not -path './tools/*' -exec dirname {} \; | sed 's|^\./||' | sort)
fi

# --- 1. Pre-commit checks (REUSE, gitleaks) ---
step "pre-commit"
if ! command -v pre-commit &>/dev/null; then
  fail "pre-commit not installed (see CONTRIBUTING.md)"
else
  pre-commit run --all-files || fail "pre-commit"
fi

# --- 2. Per-module checks ---
for mod in "${MODULES[@]}"; do
  pushd "$mod" > /dev/null

  # go mod tidy check
  step "$mod: go mod tidy"
  HAD_GOSUM=false
  [ -f go.sum ] && HAD_GOSUM=true
  cp go.mod go.mod.bak
  $HAD_GOSUM && cp go.sum go.sum.bak
  if go mod tidy 2>&1; then
    DIRTY=false
    diff -q go.mod go.mod.bak &>/dev/null || DIRTY=true
    if $HAD_GOSUM; then
      diff -q go.sum go.sum.bak &>/dev/null || DIRTY=true
    elif [ -f go.sum ]; then
      DIRTY=true
    fi
    $DIRTY && fail "$mod: go.mod/go.sum not tidy"
  else
    fail "$mod: go mod tidy"
  fi
  mv go.mod.bak go.mod
  $HAD_GOSUM && mv go.sum.bak go.sum || rm -f go.sum 2>/dev/null
  rm -f go.sum.bak 2>/dev/null

  # build
  step "$mod: make build"
  make build 2>&1 || fail "$mod: make build"

  # lint
  step "$mod: make lint"
  if grep -q '^lint:' Makefile 2>/dev/null; then
    make lint 2>&1 || fail "$mod: make lint"
  elif command -v golangci-lint &>/dev/null; then
    golangci-lint run --timeout 5m 2>&1 || fail "$mod: golangci-lint"
  else
    echo "SKIP: no lint target and golangci-lint not on PATH"
  fi

  popd > /dev/null
done

# --- Summary ---
echo ""
echo "==============================="
if [ $ERRORS -eq 0 ]; then
  echo "All checks passed."
else
  echo "FAILED:"
  printf "  - %s\n" "${FAILED[@]}"
  exit 1
fi
