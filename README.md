# Clearance

Clearance is a FastAPI backend project I am building to refine / build on the foundation of my backend engineering skills by building the system piece by piece.

The product idea is a transaction authorization platform. Clients submit transaction requests, and Clearance eventually decides whether each transaction should be approved, declined, or sent to review based on users, merchants, amount, request history, and risk rules.

The goal is not just to make a CRUD app. The goal is to build something that shows backend fundamentals clearly:

- authentication
- API contracts
- durable storage
- database constraints
- idempotency
- audit logs
- risk decisions
- observability

## Current Status

I am currently building out the product domain and production-style backend behaviors.

Completed so far:

- FastAPI app setup
- modular route structure
- `POST /auth/register`
- `POST /auth/login`
- `GET /users/me`
- password hashing
- JWT access tokens
- protected route dependency
- Pydantic request models
- Pydantic response models
- Dockerized local Postgres
- SQLAlchemy engine/session setup
- `User` database model
- `users` table
- `Merchant` database model
- `Transaction` database model
- `AuditEvent` database model
- `POST /merchants`
- `GET /merchants`
- `POST /transactions`
- `GET /transactions`
- `GET /transactions/{id}`
- `GET /audit-events`
- idempotent transaction creation with `Idempotency-Key`
- basic risk decisions: `approved`, `declined`, `review`
- merchant trust status
- velocity-based transaction review
- audit event creation
- request IDs
- request logging
- rate limiting
- CORS configuration
- basic security headers
- pytest-based API tests
- dependency manifests for runtime and test setup

The important shift has been moving from memory-stored user dicts to Postgres-backed records and then layering real system behavior on top of that.

Memory is runtime state. A database is durable state.

## Stack

- FastAPI for HTTP routes and request/response handling
- Pydantic for request validation and response schemas
- SQLAlchemy for database models, sessions, and queries
- psycopg for speaking to Postgres
- Postgres for durable storage
- Docker Compose for local database setup
- passlib/bcrypt for password hashing
- JWT for signed access tokens
- in-memory rate limiting for local API protection
- request middleware for IDs, logs, and security headers
- pytest and httpx for API-level verification

## Local Setup

Install runtime dependencies:

```bash
.venv/bin/python -m pip install -r requirements.txt
```

Install runtime plus test dependencies:

```bash
.venv/bin/python -m pip install -r requirements-dev.txt
```

## Local Database Setup

```txt
Docker container port: 5432
Mac host port: 5433 (local homebrew postgres was using port 5432, so i'm hosting my docker postgres container on 5433)
DATABASE_URL: postgresql+psycopg://clearance:YOUR_LOCAL_PASSWORD@localhost:5433/clearance
```

The Compose database is bound to `127.0.0.1`, so it is reachable from this machine without exposing Postgres on every network interface.

Start Postgres:

```bash
docker compose up -d
```

Verify the container database:

```bash
docker exec clearance-postgres psql -U clearance -d clearance -c "select current_user, current_database();"
```

List tables:

```bash
docker exec clearance-postgres psql -U clearance -d clearance -c "\dt"
```

## Running The API

Create a local `.env` from `.env.example`, set a real `SECRET_KEY`, and use the same local Postgres password in both `POSTGRES_PASSWORD` and `DATABASE_URL`. The app intentionally fails startup if `SECRET_KEY` is missing, too short, or still set to a placeholder.

You can generate a local development secret with:

```bash
python -c "import secrets; print(secrets.token_urlsafe(32))"
```

Start the FastAPI app:

```bash
.venv/bin/uvicorn app.main:app --reload
```

For local development, `.env.example` sets:

```env
AUTO_CREATE_TABLES=true
ENABLE_DOCS=true
```

For a production-style deployment, schema changes should be handled by migrations instead of automatic table creation, and API docs can be disabled with:

```env
AUTO_CREATE_TABLES=false
ENABLE_DOCS=false
```

Health check:

```bash
curl -i http://127.0.0.1:8000/health
```

Register:

```bash
curl -i -X POST http://127.0.0.1:8000/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"calvin@example.com","password":"Password1!"}'
```

Login:

```bash
curl -i -X POST http://127.0.0.1:8000/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"calvin@example.com","password":"Password1!"}'
```

Use the returned token:

```bash
curl -i http://127.0.0.1:8000/users/me \
  -H "Authorization: Bearer PASTE_TOKEN_HERE"
```

Create a merchant:

```bash
curl -i -X POST http://127.0.0.1:8000/merchants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer PASTE_TOKEN_HERE" \
  -d '{"name":"Summit Coffee","category":"food","trust_status":"trusted"}'
```

List merchants:

```bash
curl -i http://127.0.0.1:8000/merchants \
  -H "Authorization: Bearer PASTE_TOKEN_HERE"
```

Create a transaction:

```bash
curl -i -X POST http://127.0.0.1:8000/transactions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer PASTE_TOKEN_HERE" \
  -H "Idempotency-Key: demo-key-001" \
  -d '{"merchant_id":1,"amount":"125.50","currency":"USD"}'
```

Retry the same transaction request with the same `Idempotency-Key`. The API should return the original transaction instead of creating a duplicate.

If the same `Idempotency-Key` is reused with a different payload, the API returns `409 Conflict`.

List audit events:

```bash
curl -i http://127.0.0.1:8000/audit-events \
  -H "Authorization: Bearer PASTE_TOKEN_HERE"
```

## Running Tests

Run the backend test suite:

```bash
.venv/bin/python -m pytest
```

Run tests with coverage:

```bash
.venv/bin/python -m coverage run -m pytest
.venv/bin/python -m coverage report
```

The tests use a throwaway SQLite database in a temporary directory. That keeps the feedback loop fast and means the tests do not need Docker or local Postgres to be running.

Optional Postgres integration tests run when `POSTGRES_INTEGRATION_DATABASE_URL` is set:

```bash
POSTGRES_INTEGRATION_DATABASE_URL=postgresql+psycopg://clearance:YOUR_LOCAL_PASSWORD@localhost:5433/clearance \
  .venv/bin/python -m pytest tests/integration
```

The integration tests create a temporary Postgres schema and drop it after the run.

## Project Structure

```txt
app/
  main.py
  auth/
    routes.py
    schemas.py
    service.py
  core/
    config.py
    request_context.py
    security.py
  db/
    dependencies.py
    models.py
    session.py
  audit/
    routes.py
    schemas.py
    service.py
  merchants/
    routes.py
    schemas.py
    service.py
  transactions/
    routes.py
    schemas.py
    risk.py
    service.py
  middleware/
    rate_limit.py
    request_logging.py
    security_headers.py
  users/
    dependencies.py
    routes.py
    schemas.py
docker-compose.yml
requirements.txt
requirements-dev.txt
alembic.ini
migrations/
docs/
.github/workflows/ci.yml
tests/
```

The idea behind the structure:

- `main.py` wires the app together.
- route files own HTTP endpoints.
- schema files own request and response contracts.
- service files own reusable domain/application logic.
- `core/security.py` owns password hashing and JWT helpers.
- `core/config.py` owns environment-backed runtime configuration.
- `core/request_context.py` owns shared request metadata helpers.
- `db/session.py` owns the SQLAlchemy engine/session setup.
- `db/dependencies.py` gives routes request-scoped DB sessions.
- `db/models.py` defines database tables.
- `audit/service.py` writes audit events.
- `transactions/risk.py` owns the first version of the decision logic.
- middleware owns request-level cross-cutting behavior.
- tests exercise API behavior and security boundaries from the outside.

## Roadmap

Current milestone: Clearance has a working authenticated transaction domain with persistent storage, idempotent transaction creation, audit events, risk decisions, security middleware, Alembic migrations, CI, and a pytest/coverage verification loop. The next major step is making risk rules configurable and adding deeper Postgres/concurrency coverage.

### 1. Finish Auth Foundation

- Clean response casing.
- Add JWT access tokens.
- Add `GET /users/me`.
- Add protected route dependencies.
- Add response models.
- Refactor auth/security code out of `main.py`.

Status: complete

What is in place:

- registration and login routes
- normalized email lookup
- password hashing with `bcrypt_sha256`
- signed JWT access tokens
- protected `GET /users/me`
- current-user dependency
- request/response schemas
- basic login timing hardening for missing users

### 2. Move From Memory To Postgres

- Replace `users = []` with database storage.
- Add a `users` table.
- Add a unique email constraint.
- Store password hashes only.
- Add created/updated timestamps.
- Query users by indexed email instead of scanning a list.

Status: complete

What is in place:

- SQLAlchemy engine/session setup
- Postgres-backed `User` model
- unique/indexed email
- created/updated timestamps
- Docker Compose local Postgres
- environment-backed `DATABASE_URL`
- Alembic migration setup for repeatable schema changes

### 3. Add Merchant And Transaction Domain

- `POST /merchants`
- `GET /merchants`
- `POST /transactions`
- `GET /transactions`
- `GET /transactions/{id}`

This is where the app starts becoming the actual product instead of only auth.

Status: complete

What is in place:

- user-owned merchants
- merchant categories
- merchant trust status
- transaction creation
- transaction listing/detail lookup
- ownership scoping so users cannot read or transact against another user's resources
- bounded list endpoints

### 4. Add Authorization Decisions

Supported decisions:

- `approved`
- `declined`
- `review`

Initial risk rules:

- high amount transactions go to review
- very high amount transactions are declined
- too many transactions in a short time window go to review
- untrusted merchants go to review

Status: current product slice complete

What is in place:

- amount-based review threshold
- amount-based decline threshold
- high-risk merchant category review
- untrusted merchant review
- non-USD currency review
- recent-transaction velocity review
- risk thresholds, high-risk categories, and velocity windows are configurable
- risk score and decision reason stored on each transaction

Next improvements:

- add per-merchant and per-user velocity windows
- add tests for concurrent transaction creation/race conditions

### 5. Add Idempotency

Support:

```http
POST /transactions
Idempotency-Key: abc-123
```

If the client retries the same request with the same idempotency key, Clearance should return the original result instead of creating a duplicate transaction.

Postgres constraint idea:

```txt
unique(user_id, idempotency_key)
```

This is one of the main SWE II-level features of the project.

Status: complete

What is in place:

- required `Idempotency-Key` header
- safe key format validation
- `unique(user_id, idempotency_key)` database constraint
- same-key/same-payload retry returns the original transaction
- same-key/different-payload retry returns `409 Conflict`
- canonical request hashes are stored for stronger payload comparison
- idempotency keys are stored but not exposed in API responses

Next improvements:

- add broader Postgres-backed idempotency tests as new retry cases appear

### 6. Add Audit Logs

Record important events:

- `REGISTERED_USER`
- `LOGGED_IN`
- `CREATED_MERCHANT`
- `TRANSACTION_APPROVED`
- `TRANSACTION_DECLINED`
- `TRANSACTION_REVIEW`

The point is traceability: who did what, when, and why.

Status: complete

What is in place:

- `AuditEvent` model
- audit writes for registration, login, merchant creation, and transaction decisions
- request ID attached to audit events when available
- metadata JSON for transaction decision context
- protected `GET /audit-events`

Next improvements:

- add richer filtering by action/entity/date
- split audit metadata into typed columns if query patterns justify it

### 7. Add Production Polish

- request IDs
- structured request logging
- request timing
- CORS
- tests
- Docker Compose polish
- architecture diagram
- API examples
- tradeoffs and future work

Status: in progress

What is in place:

- request IDs
- request logging with timing
- security headers
- request body size limits enforced on the request stream
- in-memory rate limiting
- trusted-proxy guard for `X-Forwarded-For`
- CORS config through environment variables
- strict env parsing for security-sensitive config
- dependency manifests
- Docker Compose local Postgres polish
- pytest API suite
- optional Postgres integration tests
- GitHub Actions CI
- coverage gate at 80%
- current coverage around 91%
- architecture, tradeoffs, and deployment docs

Next improvements:

- Redis/shared-store rate limiting for multi-instance deployments
- login-specific throttling
- CI badge once the GitHub repo is connected
- richer Postgres concurrency tests

## Migrations

This project uses Alembic for repeatable schema changes:

```bash
.venv/bin/alembic upgrade head
```

For a new schema change, update the model, generate a reviewed revision, and
apply it locally:

```bash
.venv/bin/alembic revision --autogenerate -m "describe schema change"
.venv/bin/alembic upgrade head
```

`AUTO_CREATE_TABLES=true` is still convenient for the local learning loop with a
fresh database. For production-like environments, use migrations instead. See
[Deployment notes](docs/deployment.md) for the full migration and rollback
workflow.

## More Documentation

- [Architecture](docs/architecture.md)
- [Tradeoffs and future work](docs/tradeoffs.md)
- [Deployment notes](docs/deployment.md)

## Security And Reliability Notes

- Passwords are stored as hashes, not raw passwords.
- New password hashes use `bcrypt_sha256` to avoid raw bcrypt's 72-byte password limit.
- JWTs are signed and verified server-side.
- `SECRET_KEY` is required at startup and must not be a placeholder.
- Runtime configuration is centralized in `app/core/config.py`.
- `DATABASE_URL` and `SECRET_KEY` are required environment values.
- Protected routes use the current-user dependency.
- User-owned resources are filtered by `current_user.id`.
- SQLAlchemy ORM queries are used instead of string-built SQL, so user input is bound as parameters rather than interpolated into SQL strings.
- Emails are normalized before lookup/storage.
- Merchant fields reject blank/control-character input, and categories are constrained to a safe character set.
- Merchant trust status is constrained to `trusted` or `untrusted`.
- Transaction currencies must be three letters and are normalized to uppercase.
- Bearer tokens and idempotency keys have explicit length/format checks.
- Request bodies over the configured `MAX_REQUEST_BODY_BYTES` are rejected.
- Request body size is enforced while the app reads the request stream, not only from `Content-Length`.
- Duplicate email is protected by both app logic and a database unique constraint.
- Transaction creation requires an `Idempotency-Key`.
- Duplicate transaction retries return the original result.
- Reusing an idempotency key with a different payload returns `409 Conflict`.
- Idempotency keys are stored for request safety but are not returned in transaction responses.
- Audit events record important auth, merchant, and transaction actions.
- Risk decisions now consider configurable amount thresholds, merchant category, merchant trust status, currency, and recent transaction velocity.
- Rate limiting is currently in-memory and intended for local/single-process development.
- Rate limiting does not trust proxy headers unless `TRUST_PROXY_HEADERS=true` and the direct client IP is inside `TRUSTED_PROXY_CIDRS`.
- Incoming `X-Request-ID` values are validated before they are logged or echoed.
- Unknown request fields are rejected instead of silently ignored.
- CORS origins are configured through environment variables.
- Basic security headers and `Cache-Control: no-store` are added by middleware.
- API tests cover auth, resource ownership, idempotency, validation, request IDs, body size limits, and rate limiting behavior.

## Architecture Direction

Clearance starts as a modular monolith. I want the code organized by responsibility without pretending it needs microservices before the domain is mature.
