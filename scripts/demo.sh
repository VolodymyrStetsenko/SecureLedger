#!/usr/bin/env sh
set -eu

BASE_URL="${BASE_URL:-http://localhost:8080}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for the demo assertions" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

create_account() {
  owner="$1"
  balance="$2"
  curl --fail-with-body -sS -X POST "$BASE_URL/v1/accounts" \
    -H 'Content-Type: application/json' \
    -H 'X-Principal-ID: operator-1' \
    -H 'X-Principal-Role: operator' \
    --data "{\"owner_id\":\"$owner\",\"currency\":\"GBP\",\"opening_balance_minor\":$balance}"
}

get_account() {
  owner="$1"
  account_id="$2"
  curl --fail-with-body -sS "$BASE_URL/v1/accounts/$account_id" \
    -H "X-Principal-ID: $owner" \
    -H 'X-Principal-Role: customer'
}

alice_json="$(create_account alice 10000)"
bob_json="$(create_account bob 0)"
alice_id="$(printf '%s' "$alice_json" | jq -er '.id')"
bob_id="$(printf '%s' "$bob_json" | jq -er '.id')"

transfer_body="$(printf '{"from_account_id":"%s","to_account_id":"%s","amount_minor":2500,"description":"Local verification"}' "$alice_id" "$bob_id")"

first_status="$(curl --fail-with-body -sS -o "$tmp_dir/first.json" -w '%{http_code}' -X POST "$BASE_URL/v1/transfers" \
  -H 'Content-Type: application/json' \
  -H 'X-Principal-ID: alice' \
  -H 'X-Principal-Role: customer' \
  -H 'Idempotency-Key: local-demo-transfer-0001' \
  --data "$transfer_body")"
test "$first_status" = "201"
jq -e '.replayed == false' "$tmp_dir/first.json" >/dev/null

replay_status="$(curl --fail-with-body -sS -D "$tmp_dir/replay.headers" -o "$tmp_dir/replay.json" -w '%{http_code}' -X POST "$BASE_URL/v1/transfers" \
  -H 'Content-Type: application/json' \
  -H 'X-Principal-ID: alice' \
  -H 'X-Principal-Role: customer' \
  -H 'Idempotency-Key: local-demo-transfer-0001' \
  --data "$transfer_body")"
test "$replay_status" = "200"
jq -e '.replayed == true' "$tmp_dir/replay.json" >/dev/null
grep -qi '^Idempotent-Replayed: true' "$tmp_dir/replay.headers"

get_account alice "$alice_id" | jq -e '.balance_minor == 7500' >/dev/null
get_account bob "$bob_id" | jq -e '.balance_minor == 2500' >/dev/null

printf 'SecureLedger demo passed\n'
printf '  Alice account: %s (7500 minor units)\n' "$alice_id"
printf '  Bob account:   %s (2500 minor units)\n' "$bob_id"
printf '  Exact replay returned the original transfer without moving funds twice.\n'
