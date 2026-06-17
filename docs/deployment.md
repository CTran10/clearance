# Deployment Notes

Clearance is not deployed yet, but the app is structured so the production path is clear.

## Required Environment Variables

The app fails startup if required config is missing or unsafe.

```env
DATABASE_URL=postgresql+psycopg://USER:PASSWORD@HOST:5432/DATABASE
SECRET_KEY=replace-with-a-real-random-secret
ACCESS_TOKEN_EXPIRE_SECONDS=1800
CORS_ORIGINS=https://your-frontend.example.com
RATE_LIMIT_MAX_REQUESTS=120
RATE_LIMIT_WINDOW_SECONDS=60
TRUST_PROXY_HEADERS=false
TRUSTED_PROXY_CIDRS=
ENABLE_DOCS=false
MAX_REQUEST_BODY_BYTES=1048576
AUTO_CREATE_TABLES=false
```

## Production Defaults

Use these production-leaning defaults:

- `AUTO_CREATE_TABLES=false`
- `ENABLE_DOCS=false`
- a long random `SECRET_KEY` stored in a secret manager
- explicit `CORS_ORIGINS`
- TLS terminated by a trusted proxy/load balancer
- `TRUST_PROXY_HEADERS=true` only when `TRUSTED_PROXY_CIDRS` is configured

## Database Migrations

Schema changes should be applied with Alembic:

```bash
alembic upgrade head
```

For local development with a fresh database, `AUTO_CREATE_TABLES=true` is convenient.
For production-like environments, migrations are the safer source of truth.

Use this workflow for schema changes:

1. Update the SQLAlchemy model.
2. Generate a revision with a clear message:

   ```bash
   alembic revision --autogenerate -m "describe schema change"
   ```

3. Review the generated migration before applying it. Confirm indexes,
   constraints, nullable changes, server defaults, and downgrade behavior are
   intentional.
4. Apply the migration locally:

   ```bash
   alembic upgrade head
   ```

5. Run the relevant pytest target against the migrated schema. For
   database-critical behavior, include the Postgres integration tests.
6. Before production rollout, apply the same migration command in a staging or
   production-like environment and confirm the app starts with
   `AUTO_CREATE_TABLES=false`.

For rollbacks during development or rehearsed release recovery, use Alembic's
relative downgrade form:

```bash
alembic downgrade -1
```

Destructive migrations should be split into safe rollout steps when practical:
add nullable structures first, backfill idempotently, switch application reads or
writes, then tighten constraints in a later migration.

## Running The App

```bash
uvicorn app.main:app --host 0.0.0.0 --port 8000
```

In production, run Uvicorn behind a process manager or platform runtime rather than relying on `--reload`.

## Health Check

```http
GET /health
```

The current health check confirms the app process is serving requests. A future production version should add a database-readiness check if the deployment platform needs it.

## CI

GitHub Actions runs:

```bash
python -m alembic upgrade head
python -m coverage run -m pytest
python -m coverage report
```

The workflow also starts a Postgres service so integration tests can verify database-specific behavior.
