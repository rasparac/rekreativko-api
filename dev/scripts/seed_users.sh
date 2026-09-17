#!/usr/bin/env bash
#
# Seeds three users through the real identity + account-profile flow:
#   1. register  -> identity.accounts (status=pending) + a verification code
#   2. read the plaintext code straight out of the DB (dev only - stands in
#      for "check your email")
#   3. verify    -> identity.accounts flips to active, which raises
#      identity.account.verified onto the outbox
#   4. outbox-publisher ships that event to NATS; account-profile's
#      subscriber creates an empty account_profile.profiles row + default
#      settings for it
#   5. login, then fill in the (now-existing) profile with some basic info
#      so the seeded users are actually useful in the UI/Postman/etc.
#
# Idempotent: safe to re-run. Existing accounts are detected by email and
# only the missing steps (verify/profile) are (re)run.
#
# Requires the full stack running: gateway, identity, account-profile,
# outbox-publisher, nats, db (e.g. `task docker:up`, or `task docker:run:local`
# for infra plus `task run:gateway|identity|account-profile|outbox` locally).

set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
DB_CONTAINER="${DB_CONTAINER:-rekreativko_db}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-rekreativko}"
SEED_PASSWORD="${SEED_PASSWORD:-Test1234!}"
PROFILE_WAIT_TIMEOUT="${PROFILE_WAIT_TIMEOUT:-30}" # seconds to wait for the async profile

for bin in curl jq docker; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "error: '$bin' is required but not found on PATH" >&2
    exit 1
  fi
done

# email|full_name|nickname|city
USERS=(
  "igor.borovica@gmail.com|Igor Borovica|igorb|Zagreb"
  "tester@gmail.com|Marko Kovač|markok|Split"
  "tester1@gmail.com|Ivana Novak|ivanan|Rijeka"
)

HTTP_STATUS=""
HTTP_BODY=""

# do_http METHOD PATH [JSON_BODY] [AUTH_TOKEN]
do_http() {
  local method="$1" path="$2" data="${3:-}" token="${4:-}"
  local -a args=(-sS -X "$method" -H "Content-Type: application/json")
  if [[ -n "$token" ]]; then
    args+=(-H "Authorization: Bearer $token")
  fi
  if [[ -n "$data" ]]; then
    args+=(-d "$data")
  fi

  local resp
  resp=$(curl "${args[@]}" -w $'\n%{http_code}' "${GATEWAY_URL}${path}")
  HTTP_STATUS="${resp##*$'\n'}"
  HTTP_BODY="${resp%$'\n'*}"
}

db_query() {
  docker exec "$DB_CONTAINER" psql -X -U "$DB_USER" -d "$DB_NAME" -tAc "$1" | tr -d '[:space:]'
}

echo "Seeding 3 users against ${GATEWAY_URL} (db container: ${DB_CONTAINER})"
echo

printf '%-28s %-12s %-38s\n' "EMAIL" "PASSWORD" "ACCOUNT_ID"

for entry in "${USERS[@]}"; do
  IFS='|' read -r email full_name nickname city <<<"$entry"

  existing="$(db_query "SELECT id || '|' || status FROM identity.accounts WHERE email = '${email}' AND deleted_at IS NULL LIMIT 1;")"

  if [[ -n "$existing" ]]; then
    account_id="${existing%%|*}"
    status="${existing##*|}"
    echo "-> ${email} already exists (${account_id}, status=${status})"
  else
    do_http POST "/identity/api/v1/register" "$(jq -n --arg email "$email" --arg password "$SEED_PASSWORD" \
      '{email: $email, password: $password}')"

    if [[ "$HTTP_STATUS" != "200" && "$HTTP_STATUS" != "201" ]]; then
      echo "error: register failed for ${email} (HTTP ${HTTP_STATUS}): ${HTTP_BODY}" >&2
      exit 1
    fi

    account_id="$(jq -r '.data' <<<"$HTTP_BODY")"
    status="pending"
    echo "-> registered ${email} (${account_id})"
  fi

  if [[ "$status" != "active" ]]; then
    code="$(db_query "SELECT code FROM identity.verification_codes WHERE account_id = '${account_id}' AND used_at IS NULL ORDER BY created_at DESC LIMIT 1;")"
    if [[ -z "$code" ]]; then
      echo "error: no unused verification code found for ${email} (${account_id})" >&2
      exit 1
    fi

    do_http POST "/identity/api/v1/verify-account" "$(jq -n --arg code "$code" '{code: $code}')"
    if [[ "$HTTP_STATUS" != "200" ]]; then
      echo "error: verify failed for ${email} (HTTP ${HTTP_STATUS}): ${HTTP_BODY}" >&2
      exit 1
    fi
    echo "   verified with code ${code}"
  fi

  do_http POST "/identity/api/v1/login" "$(jq -n --arg email "$email" --arg password "$SEED_PASSWORD" \
    '{email: $email, password: $password}')"
  if [[ "$HTTP_STATUS" != "200" ]]; then
    echo "error: login failed for ${email} (HTTP ${HTTP_STATUS}): ${HTTP_BODY}" >&2
    exit 1
  fi
  access_token="$(jq -r '.data.access_token' <<<"$HTTP_BODY")"

  # account-profile creates the row asynchronously off identity.account.verified
  # (outbox-publisher -> NATS -> account-profile subscriber), so poll for it.
  waited=0
  profile_exists=""
  while (( waited < PROFILE_WAIT_TIMEOUT )); do
    profile_exists="$(db_query "SELECT 1 FROM account_profile.profiles WHERE account_id = '${account_id}';")"
    [[ -n "$profile_exists" ]] && break
    sleep 2
    waited=$((waited + 2))
  done

  if [[ -z "$profile_exists" ]]; then
    echo "warning: account_profile row for ${email} (${account_id}) did not appear within ${PROFILE_WAIT_TIMEOUT}s." >&2
    echo "         make sure outbox-publisher, nats, and account-profile are running." >&2
  else
    do_http PUT "/account-profile/api/v1/my/profile" "$(jq -n \
      --arg full_name "$full_name" --arg nickname "$nickname" --arg city "$city" \
      '{full_name: $full_name, nickname: $nickname, location_city: $city, location_country: "Croatia"}')" "$access_token"
    if [[ "$HTTP_STATUS" != "200" ]]; then
      echo "warning: profile update failed for ${email} (HTTP ${HTTP_STATUS}): ${HTTP_BODY}" >&2
    fi
  fi

  printf '%-28s %-12s %-38s\n' "$email" "$SEED_PASSWORD" "$account_id"
done

echo
echo "Done."
