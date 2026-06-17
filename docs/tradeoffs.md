# Tradeoffs And Future Work

This project is intentionally built as a learning-focused backend that still uses production-style patterns. Some choices are deliberately simple for now, with clear upgrade paths.

## Modular Monolith First

Clearance is one FastAPI app organized by module. That keeps local development fast and keeps the domain easy to reason about.

Tradeoff: a monolith can become crowded as the product grows.

Future path: split only when there is pressure from scale, ownership, deployment cadence, or failure isolation.

## Settings-Backed Risk Rules

Risk rules currently use environment-backed settings for amount thresholds,
high-risk categories, and velocity review limits. That keeps deployment simple
while the product model is still evolving.

Tradeoff: changing rules still requires a config change and app restart.

Future path: move rule configuration into a database-backed policy table or a
small rules engine once non-engineers need live rule management.

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

The app stores a canonical request hash when an idempotency key is first used,
then compares that hash on retries.

Tradeoff: rows created before request hashes existed still need a compatibility
fallback.

Future path: remove the fallback after old rows have been backfilled or aged out.

## Current Next Steps

- Add CI status badge once the GitHub repo is connected.
- Add a small admin or rule-management surface after core backend guarantees are stable.
