# Clearance Operations

Clearance keeps destructive recovery actions out of the public HTTP service.
Operators use the `clearance-admin` binary with trusted PostgreSQL and Redpanda
credentials. Every replay, requeue, funding action, and prune records an operator
reason in PostgreSQL.

## Transaction reads

Read one transaction with the transaction or operator bearer value:

```sh
curl http://127.0.0.1:8080/transactions/txn_123 \
  -H "Authorization: Bearer $TRANSACTION_API_AUTH_VALUE"
```

List transactions with the operator bearer value:

```sh
curl 'http://127.0.0.1:8080/transactions?account_id=acct_123&status=AUTHORIZED&kind=PAYMENT&limit=25' \
  -H "Authorization: Bearer $OPERATOR_API_AUTH_VALUE"
```

Use the response `next_cursor` unchanged for the next keyset-paginated page.
The maximum page size is 100.

## Funding

`POST /accounts/{account_id}/deposits` creates an already-authorized `DEPOSIT`
transaction, balanced immutable ledger entries, an audit record, a response
idempotency record, and a `FundsDeposited` outbox event in one PostgreSQL
transaction.

Required controls:

- Separate `FUNDING_API_AUTH_VALUE` bearer credential.
- A unique `Idempotency-Key` for safe request retries.
- A stable external reference that is unique per funding source and currency.
- A human-readable operator reason.
- `FUNDING_MAX_AMOUNT_CENTS`, defaulting to `100000000`.

This endpoint is intended for demonstrations and trusted operator funding. It
does not claim that external money settled.

## Dead letters

List unresolved consumer dead letters:

```sh
docker compose run --rm --no-deps app \
  /usr/local/bin/clearance-admin dlq list OPEN
```

Inspect one record, including exact payload bytes, ordered headers, source
coordinates, failure class, and replay count:

```sh
docker compose run --rm --no-deps app \
  /usr/local/bin/clearance-admin dlq show dlq_123
```

Replay the original bytes to the original topic:

```sh
docker compose run --rm --no-deps app \
  /usr/local/bin/clearance-admin dlq replay dlq_123 "dependency recovered after incident INC-123"
```

Replay is refused when the record is not `OPEN`, its stable event ID is already
processed, or `REPLAY_WINDOW_SECONDS` has elapsed. Payloads and headers cannot be
edited during replay. A failed publish records a failed replay attempt without
marking the DLQ record republished.

## Dead-lettered outbox events

Inspect and recover database outbox rows independently of the Kafka DLQ:

```sh
docker compose run --rm --no-deps app \
  /usr/local/bin/clearance-admin outbox list-dead

docker compose run --rm --no-deps app \
  /usr/local/bin/clearance-admin outbox show evt_123

docker compose run --rm --no-deps app \
  /usr/local/bin/clearance-admin outbox requeue evt_123 "broker recovered after INC-123"
```

Only `DEAD_LETTERED` rows can be requeued. Requeue resets the attempt counter and
records an `OUTBOX_REQUEUE` operator action in the same transaction.

## Processed-event retention

Defaults:

- Replay eligibility: 14 days (`REPLAY_WINDOW_SECONDS=1209600`).
- Processed-event retention: 30 days (`PROCESSED_EVENT_RETENTION_SECONDS=2592000`).
- Maximum rows per prune: 1000 (`PROCESSED_EVENT_PRUNE_BATCH_SIZE=1000`).

Retention must be at least as long as the replay window or the admin command
fails closed at startup. Preview before pruning:

```sh
docker compose run --rm --no-deps app \
  /usr/local/bin/clearance-admin processed-events stats

docker compose run --rm --no-deps app \
  /usr/local/bin/clearance-admin processed-events preview
```

Execute one bounded batch only after reviewing the preview:

```sh
docker compose run --rm --no-deps \
  -e CLEARANCE_ADMIN_CONFIRM=yes \
  app /usr/local/bin/clearance-admin processed-events prune \
  "scheduled retention batch 2026-07-14"
```

Pruning uses a PostgreSQL advisory lock to prevent concurrent runs. It excludes
recent events and events referenced by open/recent dead letters, then records the
prune reason and result. Schedule this command externally; Clearance does not
run an internal retention scheduler.

## Metrics and alerts

With `METRICS_ENABLED=true`, each service exposes `/metrics`. Start the local
monitoring profile with:

```sh
docker compose --profile observability up --build
```

The provisioned dashboard is **Clearance Operations**. Alert rules cover target
availability, open DLQs, old outbox backlog, consumer commit failures, HTTP 5xx
rate, and PostgreSQL pool saturation. Compose binds Prometheus and Grafana to
localhost; production deployments still need authentication, durable storage,
TLS, and an alert receiver.

## Real-broker failure verification

Run:

```sh
./scripts/broker-failure-suite.sh
```

The script is destructive only inside its uniquely named temporary Compose
project. It does not accept a database or broker URL, allocates ephemeral host
ports, and removes its volumes at exit. It verifies:

1. Funding followed by a successful asynchronous authorization.
2. Duplicate `TransactionCreated` delivery without duplicate outbox or ledger effects.
3. Redpanda outage causing a durable outbox dead letter.
4. Broker restart, audited requeue, and transaction recovery.
5. Malformed Kafka bytes persisted and inspectable with exact payload fidelity.
