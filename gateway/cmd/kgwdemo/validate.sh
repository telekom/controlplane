#!/usr/bin/env bash
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0
#
# Validation for the kgateway AccessControl feature (jwtAuth + rbac).
#
# It exercises the route rendered from the Route CR by the kgateway
# FeatureBuilder: an HTTPRoute (path prefix /cosmoparrot-advanced-jwt) with an
# ExtensionRef to a TrafficPolicy that requires a JWT and RBAC-allows only a
# specific consumer claim.
#
# Prereqs:
#   - the Route is applied and kgateway has programmed the gateway deployment
#   - port-forward to the gateway data plane:
#       kubectl port-forward deployment/kgateway-poc -n kgateway-poc 8080:8080
#   - a token endpoint for the mock IdP (mock-idp-jwt), reachable from here
#
# Config via env (or a .env file next to this script):
#   TOKEN_URL       OAuth2 token endpoint of the mock IdP           (required)
#   CLIENT_ID       client whose azp/sub is on the allow-list       (required)
#   CLIENT_SECRET   its secret                                      (required)
#   DENIED_CLIENT_ID / DENIED_CLIENT_SECRET   a client NOT allowed  (optional)
#   GATEWAY_URL     defaults to the port-forward target below
#   ROUTE_PATH      defaults to /cosmoparrot-advanced-jwt
#
# Asserts:
#   1. no token                 -> 401 (jwt_authn rejects)
#   2. allowed consumer token   -> 200 (JWT valid, rbac allows, proxied)
#   3. denied consumer token    -> 403 (JWT valid, rbac denies)   [if configured]
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
env_file="${here}/.env"
if [[ -f "$env_file" ]]; then
  # shellcheck disable=SC1090,SC1091
  set -a; source "$env_file"; set +a
fi

: "${TOKEN_URL:?TOKEN_URL must be set (mock IdP token endpoint)}"
: "${CLIENT_ID:?CLIENT_ID must be set (allowed consumer)}"
: "${CLIENT_SECRET:?CLIENT_SECRET must be set}"
: "${GATEWAY_URL:=http://localhost:8080}"
: "${ROUTE_PATH:=/get}"
url="${GATEWAY_URL%/}${ROUTE_PATH}"

pass=0; fail=0
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  red=$'\e[31m'; green=$'\e[32m'; yellow=$'\e[33m'; dim=$'\e[2m'; bold=$'\e[1m'; reset=$'\e[0m'
else
  red=; green=; yellow=; dim=; bold=; reset=
fi

check() { # name expected actual
  if [[ "$2" == "$3" ]]; then echo "${green}${bold}PASS${reset}: $1 (HTTP $3)"; pass=$((pass+1));
  else echo "${red}${bold}FAIL${reset}: $1 — expected ${yellow}$2${reset}, got ${yellow}$3${reset}"; fail=$((fail+1)); fi
}

# req [BEARER] -> prints exchange to stderr, echoes the HTTP status code.
req() {
  local bearer="${1:-}"
  local -a args=(-s -o /dev/null -w '%{http_code}')
  local auth="(none)"
  if [[ -n "$bearer" ]]; then
    args+=(-H "Authorization: Bearer ${bearer}")
    auth="Bearer ${bearer:0:12}...(${#bearer} chars)"
  fi
  echo "  ${dim}-> GET ${url}  Authorization: ${auth}${reset}" >&2
  local code; code=$(curl "${args[@]}" "$url")
  echo "  ${dim}<- HTTP ${code}${reset}" >&2
  echo "$code"
}

# token CLIENT_ID CLIENT_SECRET -> echoes access_token or empty
token() {
  curl -s -X POST "$TOKEN_URL" \
    -d grant_type=client_credentials \
    -d "client_id=${1}" -d "client_secret=${2}" | jq -r '.access_token // empty'
}

echo "== waiting for gateway listener at ${GATEWAY_URL} =="
for _ in {1..30}; do
  code=$(curl -s -o /dev/null -w '%{http_code}' "$url" || true)
  [[ "$code" != "000" ]] && break
  sleep 1
done
[[ "$code" == "000" ]] && {
  echo "FAIL: gateway never responded — is the port-forward running?" >&2
  echo "      kubectl port-forward deployment/kgateway-poc -n kgateway-poc 8080:8080" >&2
  exit 1
}

echo "== case 1: no token -> 401 (jwtAuth rejects) =="
check "no token" 401 "$(req)"

echo "== fetching token for allowed consumer ${CLIENT_ID} =="
allowed=$(token "$CLIENT_ID" "$CLIENT_SECRET")
[[ -z "$allowed" ]] && { echo "FAIL: could not obtain token for ${CLIENT_ID}" >&2; exit 1; }
echo "  <- token received (${#allowed} chars)"

echo "== case 2: allowed consumer token -> 200 (rbac allows) =="
check "allowed consumer" 200 "$(req "$allowed")"

if [[ -n "${DENIED_CLIENT_ID:-}" && -n "${DENIED_CLIENT_SECRET:-}" ]]; then
  echo "== fetching token for denied consumer ${DENIED_CLIENT_ID} =="
  denied=$(token "$DENIED_CLIENT_ID" "$DENIED_CLIENT_SECRET")
  [[ -z "$denied" ]] && { echo "FAIL: could not obtain token for ${DENIED_CLIENT_ID}" >&2; exit 1; }
  echo "== case 3: denied consumer token -> 403 (rbac denies) =="
  check "denied consumer" 403 "$(req "$denied")"
else
  echo "== case 3 skipped: set DENIED_CLIENT_ID / DENIED_CLIENT_SECRET to test rbac deny =="
fi

if [[ "$fail" -gt 0 ]]; then
  echo "== summary: ${green}$pass passed${reset}, ${red}${bold}$fail failed${reset} =="
else
  echo "== summary: ${green}${bold}$pass passed${reset}, $fail failed =="
fi
exit $(( fail > 0 ? 1 : 0 ))
