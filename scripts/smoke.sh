#!/usr/bin/env sh
set -eu

API_URL="${PLANTBRAIN_API_URL:-http://localhost:8080}"
AI_URL="${PLANTBRAIN_AI_URL:-http://localhost:8000}"
API_TOKEN="${PLANTBRAIN_API_TOKEN:-dev-token}"
SKIP_AUTH="${PLANTBRAIN_SKIP_AUTH_SMOKE:-0}"
RUN_LIVE_AI="${PLANTBRAIN_SMOKE_LIVE_AI:-0}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 127
  fi
}

fetch() {
  curl --fail --silent --show-error --max-time 15 "$@"
}

expect_healthy() {
  name="$1"
  url="$2"
  body="$(fetch "$url/health")"
  case "$body" in
    *healthy*)
      echo "OK: $name health"
      ;;
    *)
      echo "Unexpected $name health response: $body" >&2
      exit 1
      ;;
  esac
}

expect_healthy "API" "$API_URL"
expect_healthy "AI" "$AI_URL"

if [ "$SKIP_AUTH" != "1" ]; then
  me_body="$(fetch -H "Authorization: Bearer $API_TOKEN" "$API_URL/api/me")"
  printf '%s' "$me_body" | grep -q '"userId"'
  printf '%s' "$me_body" | grep -q '"orgId"'
  printf '%s' "$me_body" | grep -q '"role"'
  echo "OK: API authenticated context"
fi

if [ "$RUN_LIVE_AI" = "1" ]; then
  PLANTBRAIN_TEST_URL="$AI_URL" python3 tests/integration_test.py
fi

echo "Smoke checks passed"
