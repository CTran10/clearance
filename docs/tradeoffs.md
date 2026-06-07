# Tradeoffs And Future Work

This project is intentionally built as a learning-focused backend that still uses production-style patterns. Some choices are deliberately simple for now, with clear upgrade paths.

## Modular Monolith First

Clearance is one FastAPI app organized by module. That keeps local development fast and keeps the domain easy to reason about.

Tradeoff: a monolith can become crowded as the product grows.

Future path: split only when there is pressure from scale, ownership, deployment cadence, or failure isolation.

## Hardcoded Risk Rules

Risk rules currently live in Python code. That makes them easy to read, test, and change while the product model is still evolving.

Tradeoff: changing rules requires a code deploy.

Future path: move thresholds and rule configuration into a database-backed policy table or a small rules engine once the rule set becomes dynamic.

## In-Memory Rate Limiting

The current rate limiter is useful for local abuse resistance and for demonstrating request-boundary thinking.

Tradeoff: in-memory limits do not work across multiple app instances and reset on restart.

Future path: use Redis or another shared store, and add tighter login-specific throttling.

## SQLite Fast Tests Plus Optional Postgres Integration

Most tests use a temporary SQLite database because the feedback loop is fast and does not require Docker.

Tradeoff: SQLite does not behave exactly like Postgres for constraints, transaction isolation, timestamps, and numeric types.

Future path: keep fast SQLite tests for everyday work and run Postgres integration tests in CI for database-critical behavior.

## JWT Access Tokens

JWTs keep protected-route authentication stateless and straightforward.

Tradeoff: basic JWTs are harder to revoke before expiration unless the system stores token versions or denylist state.

Future path: add issuer/audience claims, token versioning, refresh tokens, and tests for expired/malformed/deleted-user token cases.

## Idempotency Payload Comparison

The app currently compares important transaction fields when an idempotency key is reused.

Tradeoff: as request bodies become richer, field-by-field comparisons become easier to miss.

Future path: store a canonical request hash with the idempotency key so comparison stays complete and explicit.

## Current Next Steps

- Add richer Alembic migration workflow docs as the schema evolves.
- Add Postgres-backed concurrency tests around idempotency.
- Add CI status badge once the GitHub repo is connected.
- Add a small admin or rule-management surface after core backend guarantees are stable.
