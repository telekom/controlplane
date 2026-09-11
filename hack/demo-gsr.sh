#!/bin/bash

# Copyright 2026 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Demonstrates graceful client-secret rotation:
# After rotating, BOTH the old and new secrets remain valid.

set -e

SKIP_ROTATE=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-rotate) SKIP_ROTATE=true; shift ;;
    *) APP_NAME="$1"; shift ;;
  esac
done
APP_NAME="${APP_NAME:-api-test-123}"

GREEN='\033[0;32m'
RED='\033[0;31m'
BOLD='\033[1m'
RESET='\033[0m'

pass() { echo -e "  ${GREEN}✓ $1${RESET}"; }
fail() { echo -e "  ${RED}✗ $1${RESET}"; exit 1; }

fetch_token() {
  local client_id="$1" client_secret="$2" token_url="$3"
  curl -s --max-time 10 -X POST "$token_url" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=client_credentials" \
    -d "client_id=$client_id" \
    -d "client_secret=$client_secret"
}

get_info() {
  roverctl get-info --name "$APP_NAME" --format json 2>/dev/null | jq -r '.applications[0]'
}

echo -e "${BOLD}=== Graceful Secret Rotation Demo (app: $APP_NAME) ===${RESET}\n"

# --- Step 1: Verify current secret works ---
echo -e "${BOLD}1. Fetching current credentials...${RESET}"
INFO=$(get_info)
CLIENT_ID=$(echo "$INFO" | jq -r '.irisClientId')
TOKEN_URL=$(echo "$INFO" | jq -r '.irisTokenEndpointUrl')
OLD_SECRET=$(echo "$INFO" | jq -r '.irisClientSecret')

echo "   Client ID:     $CLIENT_ID"
echo "   Token URL:     $TOKEN_URL"
echo "   Secret:        ${OLD_SECRET:0:10}..."

echo -e "\n${BOLD}2. Verifying current secret works...${RESET}"
RESPONSE=$(fetch_token "$CLIENT_ID" "$OLD_SECRET" "$TOKEN_URL")
if echo "$RESPONSE" | jq -e '.access_token' > /dev/null 2>&1; then
  EXPIRES_IN=$(echo "$RESPONSE" | jq -r '.expires_in')
  pass "Token obtained (expires in ${EXPIRES_IN}s)"
else
  fail "Could not obtain token with current secret"
fi

# --- Step 2: Rotate the secret (unless --skip-rotate) ---
if [ "$SKIP_ROTATE" = true ]; then
  echo -e "\n${BOLD}3. Skipping rotation (--skip-rotate)${RESET}"
else
  echo -e "\n${BOLD}3. Rotating secret using roverctl rotate-secret...${RESET}"
  roverctl rotate-secret --name "$APP_NAME" 2>/dev/null
  pass "rotate-secret completed"
fi

# --- Step 3: Fetch updated credentials ---
echo -e "\n${BOLD}4. Fetching updated credentials after rotation...${RESET}"
INFO=$(get_info)
NEW_SECRET=$(echo "$INFO" | jq -r '.secretInfo.clientSecret')
ROTATED_SECRET=$(echo "$INFO" | jq -r '.secretInfo.rotatedClientSecret')
NEW_EXPIRES=$(echo "$INFO" | jq -r '.secretInfo.currentExpiresAt')
ROTATED_EXPIRES=$(echo "$INFO" | jq -r '.secretInfo.rotatedExpiresAt')

echo "   New secret:      ${NEW_SECRET:0:10}...  (expires: $NEW_EXPIRES)"
echo "   Old secret:      ${ROTATED_SECRET:0:10}...  (expires: $ROTATED_EXPIRES)"

# --- Step 4: Prove BOTH secrets work ---
echo -e "\n${BOLD}5. Testing NEW secret...${RESET}"
RESPONSE=$(fetch_token "$CLIENT_ID" "$NEW_SECRET" "$TOKEN_URL")
if echo "$RESPONSE" | jq -e '.access_token' > /dev/null 2>&1; then
  pass "New secret works"
else
  fail "New secret does NOT work"
fi

echo -e "\n${BOLD}6. Testing OLD (rotated) secret...${RESET}"
echo "   (rotated secret expires: $ROTATED_EXPIRES)"
NOW=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
if [[ "$NOW" > "$ROTATED_EXPIRES" ]]; then
  fail "Old secret has already expired ($ROTATED_EXPIRES < now $NOW). Rotate sooner to demo the grace period."
fi
RESPONSE=$(fetch_token "$CLIENT_ID" "$ROTATED_SECRET" "$TOKEN_URL")
if echo "$RESPONSE" | jq -e '.access_token' > /dev/null 2>&1; then
  pass "Old secret still works (graceful rotation confirmed)"
else
  ERROR=$(echo "$RESPONSE" | jq -r '.error_description // .error // "unknown error"')
  fail "Old secret rejected: $ERROR"
fi

echo -e "\n${BOLD}=== Result: Graceful rotation verified ✓ ===${RESET}"
echo "Both secrets are valid during the rotation window."
echo "Old secret expires at: $ROTATED_EXPIRES"
