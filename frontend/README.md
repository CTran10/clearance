# Clearance Frontend POC

This is a small dependency-free operator console for the Clearance FastAPI backend.
It is intentionally thin: the goal is to make the backend behavior visible for a
portfolio reviewer, not to become a second full application.

## Scope

- Register and log in.
- Store a local demo bearer token in `localStorage`.
- Create and list merchants.
- Create transactions with an explicit `Idempotency-Key`.
- Show transaction decision status, risk score, and decision reason.
- List audit events for the logged-in user.

## Run Locally

Start the API from the repo root:

```bash
.venv/bin/uvicorn app.main:app --reload
```

Serve the frontend from this directory:

```bash
python3 -m http.server 5173
```

Open:

```txt
http://127.0.0.1:5173
```

The default API base URL is `http://127.0.0.1:8000`. If the browser blocks API
calls because of CORS, add the frontend origin to `CORS_ORIGINS` in your local
environment:

```env
CORS_ORIGINS=http://127.0.0.1:5173
```

## Test

```bash
npm test
```
