#!/usr/bin/env sh
set -eu

BASE_URL="${BASE_URL:-http://localhost:8080}"

create_account() {
  owner="$1"
  balance="$2"
  curl --fail-with-body -sS -X POST "$BASE_URL/v1/accounts" \
    -H 'Content-Type: application/json' \
    -H 'X-Principal-ID: operator-1' \
    -H 'X-Principal-Role: operator' \
    --data "{\"owner_id\":\"$owner\",\"currency\":\"GBP\",\"opening_balance_minor\":$balance}"
}

echo "Create Alice:"
create_account alice 10000
echo
echo "Create Bob:"
create_account bob 0
echo
echo "Copy account IDs and use the transfer example in README.md."
