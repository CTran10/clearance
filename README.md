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

I am currently in Phase 2: moving user storage from in-memory Python data to Postgres.

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

The important shift right now is from memory stored users dicts to postgres stored records

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

## Local Database Setup

```txt
Docker container port: 5432
Mac host port: 5433(local homebrew postgres was using port 5432, so i'm hosting my docker postgres container on 5433)
DATABASE_URL: postgresql+psycopg://clearance:clearance@localhost:5433/clearance
```

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

Start the FastAPI app:

```bash
.venv/bin/uvicorn app.main:app --reload
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

## Project Structure

```txt
app/
  main.py
  auth/
    routes.py
    schemas.py
  core/
    security.py
  db/
    dependencies.py
    models.py
    session.py
  users/
    dependencies.py
    routes.py
docker-compose.yml
```

The idea behind the structure:

- `main.py` wires the app together.
- route files own HTTP endpoints.
- schema files own request and response contracts.
- `core/security.py` owns password hashing and JWT helpers.
- `db/session.py` owns the SQLAlchemy engine/session setup.
- `db/dependencies.py` gives routes request-scoped DB sessions.
- `db/models.py` defines database tables.

## Roadmap

### 1. Finish Auth Foundation

- Clean response casing.
- Add JWT access tokens.
- Add `GET /users/me`.
- Add protected route dependencies.
- Add response models.
- Refactor auth/security code out of `main.py`.

Status: mostly complete.

### 2. Move From Memory To Postgres

- Replace `users = []` with database storage.
- Add a `users` table.
- Add a unique email constraint.
- Store password hashes only.
- Add created/updated timestamps.
- Query users by indexed email instead of scanning a list.

Status: in progress.

### 3. Add Merchant And Transaction Domain

- `POST /merchants`
- `GET /merchants`
- `POST /transactions`
- `GET /transactions`
- `GET /transactions/{id}`

This is where the app starts becoming the actual product instead of only auth.

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

### 6. Add Audit Logs

Record important events:

- `REGISTERED_USER`
- `LOGGED_IN`
- `CREATED_MERCHANT`
- `AUTHORIZED_TRANSACTION`
- `DECLINED_TRANSACTION`
- `REVIEWED_TRANSACTION`

The point is traceability: who did what, when, and why.

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

## Architecture Direction

Clearance starts as a modular monolith. I want the code organized by responsibility without pretending it needs microservices before the domain is mature.

