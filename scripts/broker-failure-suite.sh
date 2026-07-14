#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="${CLEARANCE_FAILURE_RUN_ID:-$$}"
PROJECT_NAME="clearance-failure-${RUN_ID}"

free_port() {
  python3 -c 'import socket; sock = socket.socket(); sock.bind(("127.0.0.1", 0)); print(sock.getsockname()[1]); sock.close()'
}

API_PORT="${CLEARANCE_FAILURE_API_PORT:-$(free_port)}"

export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-failure-suite-postgres}"
export POSTGRES_HOST_PORT="${POSTGRES_HOST_PORT:-$(free_port)}"
export REDIS_HOST_PORT="${REDIS_HOST_PORT:-$(free_port)}"
export REDPANDA_HOST_PORT="${REDPANDA_HOST_PORT:-$(free_port)}"
export REDPANDA_ADMIN_HOST_PORT="${REDPANDA_ADMIN_HOST_PORT:-$(free_port)}"
export TRANSACTION_SERVICE_HOST_PORT="$API_PORT"
export OUTBOX_HEALTH_HOST_PORT="${OUTBOX_HEALTH_HOST_PORT:-$(free_port)}"
export RISK_HEALTH_HOST_PORT="${RISK_HEALTH_HOST_PORT:-$(free_port)}"
export LEDGER_HEALTH_HOST_PORT="${LEDGER_HEALTH_HOST_PORT:-$(free_port)}"
export TRANSACTION_API_AUTH_VALUE="${TRANSACTION_API_AUTH_VALUE:-failure-suite-transaction}"
export FUNDING_API_AUTH_VALUE="${FUNDING_API_AUTH_VALUE:-failure-suite-funding}"
export OPERATOR_API_AUTH_VALUE="${OPERATOR_API_AUTH_VALUE:-failure-suite-operator}"
export OUTBOX_MAX_ATTEMPTS=1
export CONSUMER_MAX_ATTEMPTS=1
export METRICS_ENABLED=true

ACCOUNT_ID="acct_failure_${RUN_ID}"
API_BASE="http://127.0.0.1:${API_PORT}"

compose() {
  docker compose --project-name "$PROJECT_NAME" --file "$ROOT_DIR/docker-compose.yml" "$@"
}

sql() {
  compose exec -T postgres psql -U clearance -d clearance -v ON_ERROR_STOP=1 -Atc "$1"
}

json_field() {
  python3 -c 'import json, sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

fail() {
  echo "failure suite: $*" >&2
  return 1
}

wait_for_http() {
  local url="$1"
  for _ in $(seq 1 90); do
    if curl --fail --silent --output /dev/null "$url"; then
      return 0
    fi
    sleep 1
  done
  fail "timed out waiting for $url"
}

wait_for_sql() {
  local expected="$1"
  local query="$2"
  local actual=""
  for _ in $(seq 1 90); do
    actual="$(sql "$query" 2>/dev/null || true)"
    if [[ "$actual" == "$expected" ]]; then
      return 0
    fi
    sleep 1
  done
  fail "timed out waiting for SQL result '$expected'; last result was '$actual'"
}

wait_for_transaction() {
  local transaction_id="$1"
  local expected="$2"
  local response=""
  local status=""
  for _ in $(seq 1 90); do
    response="$(curl --fail --silent \
      -H "Authorization: Bearer $TRANSACTION_API_AUTH_VALUE" \
      "$API_BASE/transactions/$transaction_id" 2>/dev/null || true)"
    if [[ -n "$response" ]]; then
      status="$(json_field status <<<"$response")"
      if [[ "$status" == "$expected" ]]; then
        return 0
      fi
      if [[ "$status" != "PENDING" ]]; then
        fail "transaction $transaction_id reached unexpected status $status"
      fi
    fi
    sleep 1
  done
  fail "timed out waiting for transaction $transaction_id to become $expected"
}

submit_transaction() {
  local suffix="$1"
  local response
  response="$(curl --fail-with-body --silent --show-error \
    -X POST "$API_BASE/transactions" \
    -H "Authorization: Bearer $TRANSACTION_API_AUTH_VALUE" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: idem_${RUN_ID}_${suffix}" \
    -H "X-Correlation-ID: trace_${RUN_ID}_${suffix}" \
    --data "{\"account_id\":\"$ACCOUNT_ID\",\"merchant_id\":\"merchant_failure\",\"amount_cents\":1250,\"currency\":\"USD\"}")"
  json_field transaction_id <<<"$response"
}

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ $status -ne 0 ]]; then
    compose ps >&2 || true
    compose logs --tail=120 app outbox-publisher risk-service ledger-service redpanda >&2 || true
  fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT INT TERM

echo "failure suite: starting isolated stack $PROJECT_NAME"
compose up --detach --build --wait --wait-timeout 180
wait_for_http "$API_BASE/healthz"

echo "failure suite: funding an isolated test account"
curl --fail-with-body --silent --show-error \
  -X POST "$API_BASE/accounts/$ACCOUNT_ID/deposits" \
  -H "Authorization: Bearer $FUNDING_API_AUTH_VALUE" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: funding_${RUN_ID}" \
  -H "X-Correlation-ID: trace_funding_${RUN_ID}" \
  --data "{\"amount_cents\":200000,\"currency\":\"USD\",\"funding_source\":\"failure-suite\",\"external_reference\":\"funding-ref-${RUN_ID}\",\"operator_reason\":\"real broker failure suite setup\"}" \
  >/dev/null

echo "failure suite: verifying happy path and duplicate-delivery idempotency"
HAPPY_TRANSACTION_ID="$(submit_transaction happy)"
wait_for_transaction "$HAPPY_TRANSACTION_ID" AUTHORIZED

OUTBOX_ROW="$(sql "select id || '|' || partition_key || '|' || correlation_id || '|' || encode(convert_to(payload::text, 'UTF8'), 'hex') from outbox_events where aggregate_id = '$HAPPY_TRANSACTION_ID' and event_type = 'TransactionCreated'")"
IFS='|' read -r EVENT_ID PARTITION_KEY CORRELATION_ID PAYLOAD_HEX <<<"$OUTBOX_ROW"
[[ -n "$EVENT_ID" && -n "$PAYLOAD_HEX" ]] || fail "could not load the original TransactionCreated event"
BEFORE_RISK_OUTBOX="$(sql "select count(*) from outbox_events where aggregate_id = '$HAPPY_TRANSACTION_ID' and event_type = 'RiskEvaluated'")"
BEFORE_LEDGER="$(sql "select count(*) from ledger_entries where transaction_id = '$HAPPY_TRANSACTION_ID'")"
BEFORE_LAST_SEEN="$(sql "select extract(epoch from last_seen_at) from processed_events where consumer_name = 'risk-service' and event_id = '$EVENT_ID'")"

python3 -c 'import binascii, sys; sys.stdout.buffer.write(binascii.unhexlify(sys.argv[1]) + b"\n")' "$PAYLOAD_HEX" | \
  compose exec -T redpanda rpk topic produce transactions.created \
    -X brokers=redpanda:9092 \
    -k "$PARTITION_KEY" \
    -H "event_id:$EVENT_ID" \
    -H "correlation_id:$CORRELATION_ID" \
    >/dev/null
wait_for_sql 1 "select count(*) from processed_events where consumer_name = 'risk-service' and event_id = '$EVENT_ID' and last_seen_at > to_timestamp($BEFORE_LAST_SEEN)"
AFTER_RISK_OUTBOX="$(sql "select count(*) from outbox_events where aggregate_id = '$HAPPY_TRANSACTION_ID' and event_type = 'RiskEvaluated'")"
AFTER_LEDGER="$(sql "select count(*) from ledger_entries where transaction_id = '$HAPPY_TRANSACTION_ID'")"
[[ "$AFTER_RISK_OUTBOX" == "$BEFORE_RISK_OUTBOX" ]] || fail "duplicate delivery created another risk outbox event"
[[ "$AFTER_LEDGER" == "$BEFORE_LEDGER" ]] || fail "duplicate delivery changed ledger entries"

echo "failure suite: forcing an outbox dead letter during broker outage"
compose stop redpanda >/dev/null
OUTAGE_TRANSACTION_ID="$(submit_transaction outage)"
wait_for_sql 1 "select count(*) from outbox_events where aggregate_id = '$OUTAGE_TRANSACTION_ID' and status = 'DEAD_LETTERED'"
OUTBOX_ID="$(sql "select id from outbox_events where aggregate_id = '$OUTAGE_TRANSACTION_ID' and status = 'DEAD_LETTERED'")"

echo "failure suite: restarting broker and requeueing the durable outbox event"
compose up --detach --wait --wait-timeout 90 redpanda >/dev/null
compose run --rm --no-deps app /usr/local/bin/clearance-admin outbox requeue "$OUTBOX_ID" "failure suite broker recovery" >/dev/null
wait_for_transaction "$OUTAGE_TRANSACTION_ID" AUTHORIZED
wait_for_sql 1 "select count(*) from operator_actions where action_type = 'OUTBOX_REQUEUE' and target_id = '$OUTBOX_ID'"

echo "failure suite: producing malformed bytes and inspecting the durable DLQ record"
POISON_EVENT_ID="evt_poison_${RUN_ID}"
printf '{malformed-json\n' | compose exec -T redpanda rpk topic produce transactions.created \
  -X brokers=redpanda:9092 \
  -k "$ACCOUNT_ID" \
  -H "event_id:$POISON_EVENT_ID" \
  -H "correlation_id:trace_poison_${RUN_ID}" \
  >/dev/null
wait_for_sql 1 "select count(*) from dead_letter_messages where event_id = '$POISON_EVENT_ID' and state = 'OPEN' and kafka_published_at is not null"
DLQ_ID="$(sql "select id from dead_letter_messages where event_id = '$POISON_EVENT_ID' and state = 'OPEN'")"
DLQ_JSON="$(compose run --rm --no-deps app /usr/local/bin/clearance-admin dlq show "$DLQ_ID")"
[[ "$(json_field event_id <<<"$DLQ_JSON")" == "$POISON_EVENT_ID" ]] || fail "DLQ inspection returned the wrong event"
[[ "$(sql "select encode(payload, 'hex') from dead_letter_messages where id = '$DLQ_ID'")" == "7b6d616c666f726d65642d6a736f6e" ]] || fail "DLQ payload bytes were not preserved exactly"

echo "failure suite: PASS"
