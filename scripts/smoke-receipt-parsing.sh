#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080/api}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
CATEGORY_ID="${CATEGORY_ID:-}"
CURRENCY="${CURRENCY:-BYN}"

api_curl() {
  if [[ -n "$AUTH_TOKEN" ]]; then
    curl -fsS -H "Authorization: Bearer $AUTH_TOKEN" "$@"
  else
    curl -fsS "$@"
  fi
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

receipt_file="$tmp_dir/receipt.png"
printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR' > "$receipt_file"

if [[ -z "$CATEGORY_ID" ]]; then
  categories_json="$tmp_dir/categories.json"
  api_curl "$API_BASE_URL/categories" > "$categories_json"
  CATEGORY_ID="$(python3 - "$categories_json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)

if not data:
    raise SystemExit("no categories returned; create a category or pass CATEGORY_ID")

print(data[0]["id"])
PY
)"
fi

create_json="$tmp_dir/create.json"
api_curl \
  -X POST "$API_BASE_URL/receipt-parses" \
  -F "receipt=@$receipt_file;type=image/png" \
  -F "category_ids=$CATEGORY_ID" \
  -F "currency=$CURRENCY" \
  > "$create_json"

job_id="$(python3 - "$create_json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)

print(data["id"])
PY
)"

parse_json="$tmp_dir/parse.json"
for _ in $(seq 1 30); do
  api_curl "$API_BASE_URL/receipt-parses/$job_id" > "$parse_json"
  status="$(python3 - "$parse_json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)

print(data["status"])
PY
)"
  if [[ "$status" == "ready" ]]; then
    break
  fi
  if [[ "$status" == "failed" ]]; then
    cat "$parse_json"
    exit 1
  fi
  sleep 1
done

if [[ "$status" != "ready" ]]; then
  echo "receipt parse did not become ready, last status: $status" >&2
  cat "$parse_json" >&2
  exit 1
fi

approve_json="$tmp_dir/approve.json"
python3 - "$parse_json" > "$approve_json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)

expenses = []
for draft in data["draft_expenses"]:
    expenses.append({
        "draft_id": draft["id"],
        "title": draft["title"],
        "amount": draft["amount"],
        "currency": draft["currency"],
        "category_ids": [draft["category_id"]],
        "date": "2026-04-25",
    })

print(json.dumps({"expenses": expenses}))
PY

approved_json="$tmp_dir/approved.json"
api_curl \
  -X POST "$API_BASE_URL/receipt-parses/$job_id/approve" \
  -H "Content-Type: application/json" \
  --data-binary "@$approve_json" \
  > "$approved_json"

approved_status="$(python3 - "$approved_json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)

print(data["status"])
PY
)"

if [[ "$approved_status" != "approved" ]]; then
  echo "approve failed, status: $approved_status" >&2
  cat "$approved_json" >&2
  exit 1
fi

echo "receipt parsing smoke passed: job_id=$job_id"
