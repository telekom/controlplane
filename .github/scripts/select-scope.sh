#!/usr/bin/env bash
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

# Computes which CI modules need to be built/tested and which need their
# container images (re)packaged, for a given execution mode and set of
# changed files.
#
# Usage:
#   .github/scripts/select-scope.sh <maximum|minimum|optimized|none> <changed-file>...
#   git diff --name-only origin/main...HEAD | .github/scripts/select-scope.sh optimized
#
# "none" always yields empty build/package lists, regardless of changed
# files - used for triggers (e.g. tag pushes) that intentionally shouldn't
# run any module CI here, such as when a separate workflow already covers
# build/test/publish for that event.
#
# Changed files can be passed as positional args and/or piped via stdin
# (one path per line); both sources are combined.
#
# Output (stdout): a single JSON object:
#   {
#     "mode": "optimized",        # the mode actually used (may be forced
#                                  # to "maximum" by a global-impact change)
#     "run_full": false,
#     "build_modules": [ ... module config objects ... ],
#     "package_modules": [ ... subset of build_modules, packageable only ... ]
#   }
#
# Each module config object carries all fields from .github/ci/modules.yaml
# (with defaults applied) so it can be fed directly into a GitHub Actions
# matrix `include:` list via fromJSON().

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

MODULES_FILE=".github/ci/modules.yaml"
GRAPH_FILE=".github/ci/module-graph.json"

# Paths that can affect CI behavior/build output in ways a simple
# per-module diff can't safely reason about (shared tooling, lint config,
# workflow definitions, the CI scoping machinery itself, ...). Any change
# under these paths forces mode=maximum regardless of the requested mode.
GLOBAL_IMPACT_PATTERNS=(
  '^Makefile$'
  '^\.golangci\.yml$'
  '^\.ko\.yaml$'
  '^hack/'
  '^\.github/workflows/'
  '^\.github/scripts/'
  '^\.github/ci/'
)

usage() {
  echo "Usage: $0 <maximum|minimum|optimized|none> [changed-file...]" >&2
  exit 1
}

[ $# -ge 1 ] || usage
mode="$1"
shift
case "$mode" in
  maximum | minimum | optimized | none) ;;
  *)
    echo "error: unknown mode '$mode' (expected maximum|minimum|optimized|none)" >&2
    exit 1
    ;;
esac

if [ "$mode" = "none" ]; then
  jq -n '{mode: "none", run_full: false, build_modules: [], package_modules: []}'
  exit 0
fi

for tool in yq jq; do
  command -v "$tool" &>/dev/null || {
    echo "error: $tool is required" >&2
    exit 1
  }
done

# --- Collect changed files (args + stdin) ---
changed_files=()
for f in "$@"; do
  [ -n "$f" ] && changed_files+=("$f")
done
if [ ! -t 0 ]; then
  while IFS= read -r line; do
    [ -n "$line" ] && changed_files+=("$line")
  done
fi

# --- Load module config (with defaults applied) ---
modules_json="$(yq -o=json '
  .modules[] |= (
    {
      "packageable": true,
      "ko_build_path": "cmd/main.go",
      "run_check_generated_files": false,
      "run_lint": true,
      "lint_fail_on_issues": false,
      "coverage_threshold": 70
    } * .
  ) | .modules
' "$MODULES_FILE")"

graph_json="$(cat "$GRAPH_FILE")"

# --- Determine global-impact / forced maximum ---
run_full=false
if [ "$mode" = "maximum" ]; then
  run_full=true
else
  for f in "${changed_files[@]}"; do
    for pat in "${GLOBAL_IMPACT_PATTERNS[@]}"; do
      if [[ "$f" =~ $pat ]]; then
        run_full=true
        break 2
      fi
    done
  done
fi

effective_mode="$mode"
if $run_full; then
  effective_mode="maximum"
fi

# --- Determine directly-changed modules (by path prefix match) ---
changed_files_json="$(printf '%s\n' "${changed_files[@]}" | jq -R . | jq -sc '[.[] | select(length > 0)]')"

changed_modules_json="$(jq -c --argjson modules "$modules_json" --argjson files "$changed_files_json" '
  [ $modules[] | select(
      . as $m | $files | any(startswith($m.path + "/") or . == $m.path)
    ) | .name
  ] | unique
' <<<'null')"

if [ "$effective_mode" = "maximum" ]; then
  build_modules_json="$(jq -c '[.[].name]' <<<"$modules_json")"
else
  build_modules_json="$changed_modules_json"

  if [ "$effective_mode" = "optimized" ]; then
    # Transitive closure over dependents: repeatedly add any module whose
    # depends_on list intersects the current build set, until no more are
    # added (fixed point). The graph is small (tens of nodes), so a
    # bounded loop is simpler and safer than a recursive jq filter.
    for _ in $(seq 1 "$(jq 'length' <<<"$modules_json")"); do
      next_json="$(jq -c --argjson graph "$graph_json" --argjson cur "$build_modules_json" '
        ($cur + [
          $graph | to_entries[] | select(
            (.value.depends_on // []) | any(. as $d | $cur | index($d) != null)
          ) | .key
        ]) | unique
      ' <<<'null')"
      if [ "$next_json" = "$build_modules_json" ]; then
        break
      fi
      build_modules_json="$next_json"
    done
  fi
fi

# package_modules: only modules that were themselves directly changed (no
# point re-publishing an image whose contents didn't change), except in
# maximum mode where every packageable module is (re)published, matching
# today's behavior on main/tags/schedule.
if [ "$effective_mode" = "maximum" ]; then
  package_source_json="$build_modules_json"
else
  package_source_json="$changed_modules_json"
fi

build_module_objs_json="$(jq -c --argjson modules "$modules_json" --argjson names "$build_modules_json" '
  [ $modules[] | select(.name as $n | $names | index($n) != null) ]
' <<<'null')"

package_module_objs_json="$(jq -c --argjson modules "$modules_json" --argjson names "$package_source_json" '
  [ $modules[] | select(.packageable and (.name as $n | $names | index($n) != null)) ]
' <<<'null')"

jq -n \
  --arg mode "$effective_mode" \
  --argjson run_full "$run_full" \
  --argjson build_modules "$build_module_objs_json" \
  --argjson package_modules "$package_module_objs_json" \
  '{mode: $mode, run_full: $run_full, build_modules: $build_modules, package_modules: $package_modules}'
