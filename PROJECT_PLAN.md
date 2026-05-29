# RiskGate Project Plan

## Project

**RiskGate** is a FastAPI transaction authorization backend.

The goal is to grow a small auth API into a portfolio-worthy backend system that demonstrates real software engineering fundamentals: authentication, persistence, API contracts, idempotency, auditability, risk decisions, and observability.

The product idea:

> Clients submit transaction authorization requests, and RiskGate decides whether to approve, decline, or send each transaction to review based on user, merchant, amount, request history, and risk rules.

## Why This Project Is Strong

- It is more distinctive than a generic CRUD app.
- It maps well to fintech, platform, and payment-network backend work.
- It shows production-style backend thinking.
- It creates room for SWE II-level features like idempotency, audit logs, request tracing, and risk decisions.

## Current Phase

### Phase 1: Real Auth API

Build:

- `POST /auth/register`
- `POST /auth/login`
- `GET /users/me`

Learn:

- FastAPI routes
- Pydantic request validation
- Pydantic response models
- password hashing
- JWT access tokens
- `Authorization: Bearer <token>`
- protected route dependencies
- HTTP status codes

Current status:

- Basic auth flow works.
- Passwords are hashed.
- Login issues a JWT.
- `/users/me` is protected by token authentication.
- Response models have been added.

## Build Roadmap

### 1. Finish Auth Foundation

- Clean response casing.
- Add JWT access tokens.
- Add `GET /users/me`.
- Add a protected route dependency.
- Add response models.
- Refactor auth/security code out of `main.py`.

### 2. Move From Memory To Postgres

- Replace `users = []` with database storage.
- Add a `users` table.
- Add a unique email constraint.
- Store password hashes only.
- Add created/updated timestamps.

Key concept:

> Memory is runtime state. A database is durable state.

### 3. Add Merchant And Transaction Domain

- `POST /merchants`
- `GET /merchants`
- `POST /transactions`
- `GET /transactions`
- `GET /transactions/{id}`

Learn:

- resource modeling
- ownership
- foreign keys
- request and response schemas
- service-layer logic

### 4. Add Authorization Decisions

Supported transaction decisions:

- `approved`
- `declined`
- `review`

Initial risk rules:

- high amount transactions go to review
- very high amount transactions are declined
- too many transactions in a short time window go to review
- untrusted merchants go to review

Learn:

- domain rules
- deterministic decisioning
- explainable backend behavior

### 5. Add Idempotency

Support:

```http
POST /transactions
Idempotency-Key: abc-123
```

Behavior:

- First request creates a transaction.
- Retried request with the same key returns the original transaction.
- Duplicate transactions are not created.

Postgres constraint idea:

```txt
unique(user_id, idempotency_key)
```

Learn:

- safe retries
- distributed systems thinking
- database constraints
- production API behavior

### 6. Add Audit Logs

Record important events:

- `REGISTERED_USER`
- `LOGGED_IN`
- `CREATED_MERCHANT`
- `AUTHORIZED_TRANSACTION`
- `DECLINED_TRANSACTION`
- `REVIEWED_TRANSACTION`

Learn:

- traceability
- append-only records
- compliance-style system design
- debugging production behavior

### 7. Add Middleware, Tests, Docker, And README Polish

Add:

- request IDs
- structured request logging
- request timing
- CORS
- tests
- Docker Compose
- polished README
- architecture diagram
- API examples
- tradeoffs and future work

Important tradeoff:

> RiskGate starts as a modular monolith because the system is small, local development is simpler, and service boundaries are still evolving.

## Target Portfolio Description

> Built RiskGate, a FastAPI transaction authorization backend with JWT auth, Postgres persistence, idempotent transaction creation, risk-based approval decisions, audit logging, and request tracing.

## Future Project Idea

After RiskGate, build a used motorcycle marketplace with automatic deal and pricing ratings.

Potential features:

- motorcycle listings
- search and filters
- saved listings
- price history
- deal score
- market comparison
- seller reputation
- pricing model or rules engine

