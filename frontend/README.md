# Clearance Event Console

This is a dependency-free frontend proof of concept for the Go-based Clearance
authorization platform. It gives reviewers one small browser surface for the
backend MVP: health checks, authenticated transaction submission, idempotency
keys, correlation IDs, and a local receipt trail.

The console is intentionally thin. It does not replace the backend event log or
ledger data. It only makes the distributed flow easier to demo.

## Scope

- Submit `POST /transactions` requests to the Transaction Service.
- Send `Authorization`, `Idempotency-Key`, and `X-Correlation-ID` headers.
- Preview the same risk threshold used by the Risk Service.
- Keep the bearer value in memory only, not `localStorage`.
- Store only the API base URL locally.
- Show local receipts for accepted PENDING responses.

## Run Locally

Start the platform from the repository root:

```bash
docker compose up --build
```

Serve this directory:

```bash
python3 -m http.server 5173
```

Open:

```txt
http://127.0.0.1:5173
```

The default API base URL is `http://127.0.0.1:8080`. If the browser blocks API
calls, allow the frontend origin in the root `.env` file:

```env
CORS_ORIGINS=http://127.0.0.1:5173
```

Set the local bearer value in the console to match
`TRANSACTION_API_AUTH_VALUE`.

## Test

```bash
npm test
```
