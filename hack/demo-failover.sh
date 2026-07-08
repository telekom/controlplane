#!/bin/bash

# Copyright 2026 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# ─────────────────────────────────────────────────────────────────────────────
# Failover demo — provider failover (PF) + consumer failover (CF) in one run,
# via roverctl (deployed environment). Both use the same shared API.
#
# PROVIDER FAILOVER (PF)  — on the *exposure*: exposures[].failover.zones: [<zone>]
#   The api-controller provisions a SECONDARY gateway Route in each failover zone
#   carrying the provider's real upstream and points the primary route's
#   traffic.failover.targets at the failover-zone gateway
#   (api/internal/handler/apiexposure/handler.go:311). If the primary provider
#   zone is unavailable, traffic is served from the backup zone.
#
# CONSUMER FAILOVER (CF)  — on the *Rover spec*: failoverEnabled: true
#   (rover-server Rover.EnableFailoverOnAllSubscriptions). The api-controller
#   enriches ALL routes of the exposure with the DTC hostnames/paths and trusted
#   IDP issuers of every zone that has the "ConsumerFailover" gateway preset
#   (handler.go:370; admin/api/v1/zone_types.go:499). External DNS/DTC can then
#   transparently switch a consumer between zone gateways.
#   PRECONDITION: at least one zone must expose a "ConsumerFailover" gateway preset.
#
# HOW THIS DEMO PROVES IT (roverctl-only, like demo-gsr.sh)
#   roverctl cannot read gateway Route / ApiExposure CRs
#   (rover-ctl/pkg/handlers/registry.go registers only Rover/ApiSpecification/…),
#   so the observable proof is: both Rovers are ACCEPTED, reconcile to status
#   "complete", their failover config round-trips into the persisted spec, and a
#   real token-authenticated request through the gateway succeeds.
# ─────────────────────────────────────────────────────────────────────────────

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ── Config / flags ───────────────────────────────────────────────────────────
PROVIDER_NAME="rover-failover-provider"
CONSUMER_NAME="rover-failover-consumer"
PROVIDER_ZONE="dataplane1"
PROVIDER_FAILOVER_ZONE="dataplane2"
CONSUMER_ZONE="dataplane2"
BASE_PATH="/eni/failover-demo/v1"
UPSTREAM="https://httpbin.org/anything"
SPEC_FILE="${SCRIPT_DIR}/demo-failover-spec.yaml"
ROVER_FILE=""
SKIP_SPEC=false
TIMEOUT=120

while [[ $# -gt 0 ]]; do
  case "$1" in
    -f|--file)                ROVER_FILE="$2"; shift 2 ;;
    --spec)                   SPEC_FILE="$2"; shift 2 ;;
    --skip-spec)              SKIP_SPEC=true; shift ;;
    --provider-name)          PROVIDER_NAME="$2"; shift 2 ;;
    --consumer-name)          CONSUMER_NAME="$2"; shift 2 ;;
    --provider-zone)          PROVIDER_ZONE="$2"; shift 2 ;;
    --provider-failover-zone) PROVIDER_FAILOVER_ZONE="$2"; shift 2 ;;
    --consumer-zone)          CONSUMER_ZONE="$2"; shift 2 ;;
    --base-path)              BASE_PATH="$2"; shift 2 ;;
    --timeout)                TIMEOUT="$2"; shift 2 ;;
    -h|--help)
      echo "Usage: $0 [options]"
      echo "  -f, --file <path>              Use an existing Rover file instead of the generated one"
      echo "      --spec <path>              ApiSpecification file (default: $SPEC_FILE)"
      echo "      --skip-spec                Do not apply the ApiSpecification (assume already registered)"
      echo "      --provider-name <n>        Provider Rover name (default: $PROVIDER_NAME)"
      echo "      --consumer-name <n>        Consumer Rover name (default: $CONSUMER_NAME)"
      echo "      --provider-zone <z>        Provider zone (default: $PROVIDER_ZONE)"
      echo "      --provider-failover-zone <z> Provider backup zone (default: $PROVIDER_FAILOVER_ZONE)"
      echo "      --consumer-zone <z>        Consumer zone (default: $CONSUMER_ZONE)"
      echo "      --base-path <p>            API base path (default: $BASE_PATH)"
      echo "      --timeout <s>              Seconds to wait for 'complete' (default: $TIMEOUT)"
      exit 0 ;;
    *) echo "Unknown option: $1 (use -h)"; exit 1 ;;
  esac
done

GREEN='\033[0;32m'; RED='\033[0;31m'; BOLD='\033[1m'; RESET='\033[0m'
pass() { echo -e "  ${GREEN}✓ $1${RESET}"; }
fail() { echo -e "  ${RED}✗ $1${RESET}"; exit 1; }

app_info() { roverctl get-info --name "$1" --format json 2>/dev/null | jq -r '.applications[0]'; }

wait_complete() {
  local name="$1" elapsed=0 info status
  while [ "$elapsed" -lt "$TIMEOUT" ]; do
    info="$(app_info "$name")"
    status="$(echo "$info" | jq -r '.status // empty')"
    case "$status" in
      complete) pass "$name: complete"; return 0 ;;
      blocked|failed)
        echo "$info" | jq -r '.errors[]?.message' 2>/dev/null | sed 's/^/     /'
        fail "$name reconciliation $status" ;;
      *) sleep 3; elapsed=$((elapsed + 3)) ;;
    esac
  done
  fail "$name timed out after ${TIMEOUT}s (last status: ${status:-unknown})"
}

echo -e "${BOLD}=== Failover Demo (provider + consumer) ===${RESET}"
echo "   Provider:          $PROVIDER_NAME (zone $PROVIDER_ZONE, failover $PROVIDER_FAILOVER_ZONE)"
echo "   Consumer:          $CONSUMER_NAME (zone $CONSUMER_ZONE, failoverEnabled)"
echo "   Base path:         $BASE_PATH"
echo ""

# ── Step 1: Register the shared API specification ────────────────────────────
echo -e "${BOLD}1. Registering API specification...${RESET}"
if [ "$SKIP_SPEC" = true ]; then
  pass "skipped (--skip-spec)"
else
  [ -f "$SPEC_FILE" ] || fail "spec file not found: $SPEC_FILE"
  roverctl apply -f "$SPEC_FILE" && pass "ApiSpecification applied ($SPEC_FILE)" \
    || fail "failed to apply ApiSpecification"
fi

# ── Step 2: Build the Rover manifest (roverctl / tcp.ei.telekom.de format) ────
echo -e "\n${BOLD}2. Preparing provider (PF) + consumer (CF) manifest...${RESET}"
if [ -z "$ROVER_FILE" ]; then
  ROVER_FILE="$(mktemp --suffix=.yaml)"
  trap 'rm -f "$ROVER_FILE"' EXIT
  cat > "$ROVER_FILE" <<EOF
apiVersion: tcp.ei.telekom.de/v1
kind: Rover
metadata:
  name: $PROVIDER_NAME
spec:
  zone: $PROVIDER_ZONE
  exposures:
    - type: api
      basePath: $BASE_PATH
      upstream: $UPSTREAM
      visibility: WORLD
      approval: AUTO
      failover:
        zones:
          - $PROVIDER_FAILOVER_ZONE
---
apiVersion: tcp.ei.telekom.de/v1
kind: Rover
metadata:
  name: $CONSUMER_NAME
spec:
  zone: $CONSUMER_ZONE
  failoverEnabled: true
  subscriptions:
    - type: api
      basePath: $BASE_PATH
EOF
  pass "Generated $ROVER_FILE"
else
  pass "Using provided file $ROVER_FILE"
fi

# ── Step 3: Apply ────────────────────────────────────────────────────────────
echo -e "\n${BOLD}3. Applying Rovers via roverctl...${RESET}"
roverctl apply -f "$ROVER_FILE" && pass "apply accepted" \
  || fail "roverctl apply was rejected"

# ── Step 4: Wait for both to reconcile ───────────────────────────────────────
echo -e "\n${BOLD}4. Waiting for provider exposure (incl. failover routes)...${RESET}"
wait_complete "$PROVIDER_NAME"

echo -e "\n${BOLD}5. Waiting for consumer subscription (with failover enrichment)...${RESET}"
wait_complete "$CONSUMER_NAME"

# ── Step 6: Prove provider failover.zones round-tripped into the stored spec ──
echo -e "\n${BOLD}6. Verifying provider failover config persisted...${RESET}"
ZONES="$(roverctl resource get --kind Rover --api-version tcp.ei.telekom.de/v1 \
  --name "$PROVIDER_NAME" --format json 2>/dev/null \
  | jq -r '.exposures[]?.failover.zones[]?' 2>/dev/null | paste -sd, -)"
if echo "$ZONES" | grep -qw "$PROVIDER_FAILOVER_ZONE"; then
  pass "exposure failover.zones persisted: [$ZONES]"
else
  fail "provider failover zone '$PROVIDER_FAILOVER_ZONE' not in persisted spec (got: [${ZONES:-none}])"
fi

# ── Step 7: Prove consumer failoverEnabled round-tripped into the stored spec ─
echo -e "\n${BOLD}7. Verifying consumer failover flag persisted...${RESET}"
ENABLED="$(roverctl resource get --kind Rover --api-version tcp.ei.telekom.de/v1 \
  --name "$CONSUMER_NAME" --format json 2>/dev/null | jq -r '.failoverEnabled' 2>/dev/null)"
if [ "$ENABLED" = "true" ]; then
  pass "consumer failoverEnabled = true (persisted)"
else
  fail "consumer failover flag not set in persisted spec (got: ${ENABLED:-none})"
fi

# ── Step 8: Send a real request through the gateway with a fetched token ──────
echo -e "\n${BOLD}8. Calling the subscribed API through the gateway...${RESET}"
INFO="$(app_info "$CONSUMER_NAME")"
CLIENT_ID="$(echo "$INFO" | jq -r '.irisClientId')"
CLIENT_SECRET="$(echo "$INFO" | jq -r '.secretInfo.clientSecret // .irisClientSecret')"
TOKEN_URL="$(echo "$INFO" | jq -r '.irisTokenEndpointUrl')"
GATEWAY_URL="$(echo "$INFO" | jq -r '.stargateUrl')"
CALL_URL="${GATEWAY_URL%/}${BASE_PATH}/foo"

TOKEN="$(curl -s --max-time 10 -X POST "$TOKEN_URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=$CLIENT_ID" \
  -d "client_secret=$CLIENT_SECRET" | jq -r '.access_token // empty')"
[ -n "$TOKEN" ] || fail "could not obtain access token from $TOKEN_URL"
pass "access token obtained"

echo "   GATEWAY_URL: $GATEWAY_URL"
echo "   TOKEN (first 20): ${TOKEN:0:20}..."
echo "   GET $CALL_URL"
curl -sS -i --max-time 15 \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json" \
  -w '\n--- HTTP %{http_code}, %{size_download} bytes, %{time_total}s ---\n' \
  "$CALL_URL"

echo -e "\n${BOLD}=== Result: Provider + consumer failover verified ✓ ===${RESET}"
echo "Exposure '$BASE_PATH' served from '$PROVIDER_ZONE' with a secondary route in '$PROVIDER_FAILOVER_ZONE';"
echo "consumer '$CONSUMER_NAME' subscribed with failover enrichment enabled."
