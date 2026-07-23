#!/usr/bin/env bash
# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0
#
# Validation pipeline for the Envoy AccessControl feature.
#
#   1. docker compose -f gateway/cmd/xdsdemo/docker-compose.yaml up -d --build
#   2. cp gateway/cmd/xdsdemo/.env.example gateway/cmd/xdsdemo/.env  (fill secret)
#   3. gateway/cmd/xdsdemo/scripts/validate.sh
#
# Asserts:
#   - no token                    -> 401 (jwt_authn rejects)
#   - valid token, wrong Host     -> 404 (host-based routing, RT-02)
#   - valid client_credentials    -> 200 (JWT verified, azp allowed, proxied)
#   - LMS token issued + injected -> upstream sees a new JWT (iss=lms-issuer-poc)
#                                    minted by the ext_authz issuer, not the
#                                    original consumer token
#
# The demo route matches Host "demo-route.local" and path prefix "/", so every
# proxied request must carry that Host header.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
env_file="${here}/.env"

if [[ ! -f "$env_file" ]]; then
  echo "FAIL: $env_file not found. Copy .env.example to .env and set CLIENT_SECRET." >&2
  exit 1
fi
# shellcheck disable=SC1090
set -a; source "$env_file"; set +a

: "${CLIENT_ID:?CLIENT_ID must be set in .env}"
: "${CLIENT_SECRET:?CLIENT_SECRET must be set in .env}"
: "${TOKEN_URL:?TOKEN_URL must be set in .env}"
: "${ENVOY_URL:=http://localhost:8080/foo}"
: "${ROUTE_HOST:=demo-route.local}"

pass=0; fail=0
# Colors, disabled when stdout is not a TTY or NO_COLOR is set.
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  red=$'\e[31m'; green=$'\e[32m'; yellow=$'\e[33m'; dim=$'\e[2m'; bold=$'\e[1m'; reset=$'\e[0m'
else
  red=; green=; yellow=; dim=; bold=; reset=
fi

check() { # name expected actual
  if [[ "$2" == "$3" ]]; then echo "${green}${bold}PASS${reset}: $1 (HTTP $3)"; pass=$((pass+1));
  else echo "${red}${bold}FAIL${reset}: $1 — expected ${yellow}$2${reset}, got ${yellow}$3${reset}"; fail=$((fail+1)); fi
}

# req HOST [BEARER] -> echoes what it sends, prints the response, returns the
# HTTP status code on stdout's last line.
req() {
  local host="$1" bearer="${2:-}"
  local -a args=(-s -o /dev/null -w '%{http_code}' -H "Host: ${host}")
  local auth="(none)"
  if [[ -n "$bearer" ]]; then
    args+=(-H "Authorization: Bearer ${bearer}")
    auth="Bearer ${bearer:0:12}...(${#bearer} chars)"
  fi
  echo "  ${dim}-> GET ${ENVOY_URL}  Host: ${host}  Authorization: ${auth}${reset}" >&2
  local code
  code=$(curl "${args[@]}" "$ENVOY_URL")
  echo "  ${dim}<- HTTP ${code}${reset}" >&2
  echo "$code"
}

echo "== waiting for envoy listener =="
for i in {1..30}; do
  code=$(curl -s -o /dev/null -w '%{http_code}' -H "Host: ${ROUTE_HOST}" "$ENVOY_URL" || true)
  # Any HTTP response (even 401) means the listener is up.
  [[ "$code" != "000" ]] && break
  sleep 1
done
[[ "$code" == "000" ]] && { echo "FAIL: envoy listener never came up" >&2; exit 1; }

echo "== case 1: no token -> 401 =="
code=$(req "$ROUTE_HOST")
check "no token" 401 "$code"

echo "== fetching client_credentials token for $CLIENT_ID =="
echo "  -> POST ${TOKEN_URL}  grant_type=client_credentials client_id=${CLIENT_ID}"
token=$(curl -s -X POST "$TOKEN_URL" \
  -d grant_type=client_credentials \
  -d "client_id=${CLIENT_ID}" \
  -d "client_secret=${CLIENT_SECRET}" | jq -r '.access_token // empty')
if [[ -z "$token" ]]; then
  echo "FAIL: could not obtain token (check CLIENT_SECRET / network to $TOKEN_URL)" >&2
  exit 1
fi
echo "  <- token received (${#token} chars)"

# The HTTP filter chain (jwt_authn -> rbac -> router) runs before route
# matching, so an unauthenticated request 401s regardless of host. To isolate
# host-based routing we send a VALID token to a wrong host: auth passes, then
# the router finds no matching virtual host -> 404.
echo "== case 2: valid token, wrong Host -> 404 (host-based routing) =="
code=$(req "nope.invalid" "$token")
check "wrong host" 404 "$code"

echo "== case 3: valid token -> 200 =="
code=$(req "$ROUTE_HOST" "$token")
check "valid token" 200 "$code"

# decode_jwt_payload BASE64URL_JWT -> pretty JSON payload (adds base64 padding).
decode_jwt_payload() {
  local payload="${1#*.}"; payload="${payload%%.*}"
  payload="${payload//-/+}"; payload="${payload//_/\/}"
  case $(( ${#payload} % 4 )) in 2) payload+="==";; 3) payload+="=";; esac
  printf '%s' "$payload" | base64 -d 2>/dev/null
}

echo "== case 4: LMS token issued and injected upstream =="
# The upstream (/get) echoes the headers it received. ext_authz calls the LMS
# issuer, which mints a new JWT; Envoy replaces Authorization before proxying.
# We assert the upstream saw a DIFFERENT token, issued by the LMS issuer.
upstream_auth=$(curl -s -H "Host: ${ROUTE_HOST}" -H "Authorization: Bearer ${token}" "$ENVOY_URL" \
  | jq -r '.headers.Authorization[0]? // .headers.Authorization // empty')
lms_token="${upstream_auth#Bearer }"

if [[ -z "$lms_token" ]]; then
  check "LMS token present upstream" "present" "absent"
else
  echo "  ${dim}lms-token=${lms_token}${reset}"
  echo "  ${dim}lms-token claims:${reset}"
  decode_jwt_payload "$lms_token" | jq .
  lms_iss=$(decode_jwt_payload "$lms_token" | jq -r '.iss // empty')
  echo "  ${dim}upstream token iss=${lms_iss}${reset}"
  check "LMS token issued by issuer" "lms-issuer-poc" "$lms_iss"
  # The minted token must not be the consumer token that came in.
  if [[ "$lms_token" != "$token" ]]; then
    echo "${green}${bold}PASS${reset}: LMS token replaced consumer token upstream"; pass=$((pass+1))
  else
    echo "${red}${bold}FAIL${reset}: upstream still carries the original consumer token"; fail=$((fail+1))
  fi
fi

if [[ "$fail" -gt 0 ]]; then
  echo "== summary: ${green}$pass passed${reset}, ${red}${bold}$fail failed${reset} =="
else
  echo "== summary: ${green}${bold}$pass passed${reset}, $fail failed =="
fi
exit $(( fail > 0 ? 1 : 0 ))
