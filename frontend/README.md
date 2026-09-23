# Clearance Authorization Console

An operator console for the Go-based Clearance authorization platform. It gives
reviewers one browser surface for the backend MVP: authenticated
transaction submission, idempotency and correlation headers, a live risk
preview, and a local receipt trail.

The console is intentionally thin. It does not replace the backend event log or
ledger data — it only makes the distributed flow easier to demo. State that
matters to the API contract (the transaction payload, the four headers, the
$500.00 risk threshold) is covered by unit tests.

## Stack

- **Vite + React 19 + TypeScript** (strict)
- Hand-built design system — no UI framework. Tokens live in
  `src/styles/tokens.css`; primitives in `src/components/ui`.
- Self-hosted **Inter** and **JetBrains Mono** (bundled, offline-safe)
- **Vitest** for the domain-logic layer (`src/lib`)

## Layout

```
src/
├── lib/            # pure domain logic (validation, risk, formatting, api)
├── state/          # useConsole state and requests
├── components/
│   ├── ui/         # Button, Field, Panel, StatusPill (+ ui.css)
│   ├── shell/      # NoticeBar, MetricStrip
│   ├── submit/     # SubmitPanel + live RiskPreview
│   └── receipts/   # receipt stream
└── styles/         # tokens.css, global.css
test/lib/           # Vitest suites mirroring the API contract
```

## What it does

- Submit `POST /transactions` with `Authorization`, `Idempotency-Key`, and
  `X-Correlation-ID` headers.
- Preview the Risk Service decision live as you type the amount, with a
  cents → dollars echo.
- Read the local demo connection from frontend environment variables.
- Show accepted PENDING responses as a local receipt trail (last 12).

## Develop

```bash
cd frontend
npm install
npm run dev          # http://127.0.0.1:5173
```

Other scripts:

```bash
npm test             # Vitest (domain logic)
npm run typecheck    # tsc --noEmit
npm run build        # type-check + production build to dist/
npm run preview      # serve the production build
```

## Connect to the platform

Start the platform from the repository root:

```bash
docker compose up --build
```

The default API base URL is `http://127.0.0.1:8080`. In `frontend/.env.local`,
set `VITE_TRANSACTION_API_AUTH_VALUE` to match the local platform's
`TRANSACTION_API_AUTH_VALUE`. Set `VITE_API_BASE_URL` only if using a different
API address, then restart Vite. The bearer is available only in development and
omitted from production builds. It is visible in the local browser; use a local
demo bearer only, never a production credential.

If the browser blocks API calls, allow the frontend
origin in the root `.env`:

```env
CORS_ORIGINS=http://127.0.0.1:5173
```
