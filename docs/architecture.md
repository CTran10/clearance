# Architecture

Clearance is currently a modular monolith. The code is split by responsibility inside one deployable FastAPI service instead of being split into services before the domain needs that complexity.

## System Diagram

```mermaid
flowchart TD
    Client["API Client"] --> FastAPI["FastAPI App"]
    FastAPI --> Middleware["Middleware\nrequest id, logging, body limit,\nrate limit, security headers, CORS"]
    FastAPI --> Auth["Auth Routes\nregister, login, current user"]
    FastAPI --> Merchants["Merchant Routes"]
    FastAPI --> Transactions["Transaction Routes"]
    FastAPI --> Audit["Audit Routes"]

    Auth --> UserService["Auth/User Logic"]
    Merchants --> MerchantService["Merchant Service"]
    Transactions --> TransactionService["Transaction Service"]
    TransactionService --> Risk["Risk Decision Logic"]
    TransactionService --> AuditService["Audit Service"]
    Auth --> AuditService
    Merchants --> AuditService

    UserService --> DB["Postgres"]
    MerchantService --> DB
    TransactionService --> DB
    AuditService --> DB

    Alembic["Alembic Migrations"] --> DB
    Tests["Pytest + Coverage"] --> FastAPI
    Integration["Optional Postgres Integration Tests"] --> DB
```

## Request Flow

Most protected requests follow the same path:

1. Middleware assigns or validates a request ID and applies cross-cutting safety checks.
2. FastAPI routes parse the HTTP request.
3. Pydantic validates request shape and rejects unknown fields.
4. The auth dependency verifies the JWT and loads the current user.
5. Route handlers call service functions for domain behavior.
6. SQLAlchemy reads/writes durable state in Postgres.
7. Audit events are recorded for important actions.
8. Response models shape what the client receives.

## Transaction Decision Flow

```mermaid
sequenceDiagram
    participant Client
    participant API as FastAPI
    participant Auth as Auth Dependency
    participant Tx as Transaction Service
    participant Risk as Risk Logic
    participant DB as Postgres
    participant Audit as Audit Service

    Client->>API: POST /transactions + Idempotency-Key
    API->>Auth: Validate bearer token
    Auth->>DB: Load current user
    API->>Tx: Create transaction
    Tx->>DB: Check prior idempotency key
    Tx->>DB: Load owned merchant
    Tx->>DB: Count recent user transactions
    Tx->>Risk: Evaluate amount, merchant, currency, velocity
    Risk-->>Tx: approved / declined / review
    Tx->>DB: Insert transaction
    Tx->>Audit: Record decision context
    Audit->>DB: Insert audit event
    Tx-->>API: Transaction result
    API-->>Client: Response
```

## Main Guarantees

- A user can only access resources scoped to their own user id.
- Duplicate transaction retries with the same idempotency key and same payload return the original transaction.
- Reusing an idempotency key with a different payload returns `409 Conflict`.
- Risk decisions are persisted with a score and reason.
- Important actions are recorded in audit events.
- Runtime configuration fails fast when required secrets or invalid security values are missing.
