# Clearance Authorization Console

An operator console for the Go-based Clearance authorization platform. It gives
reviewers one browser surface for the backend MVP: health checks, authenticated
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
├── state/          # useConsole reducer store
├── components/
│   ├── ui/         # Button, Field, Panel, StatusPill (+ ui.css)
│   ├── shell/      # TopBar, ConnectionBar, NoticeBar, MetricStrip
│   ├── submit/     # SubmitPanel + live RiskPreview
│   ├── flow/       # event pipeline stepper
│   └── receipts/   # receipt stream
└── styles/         # tokens.css, global.css
test/lib/           # Vitest suites mirroring the API contract
```

## What it does

- Submit `POST /transactions` with `Authorization`, `Idempotency-Key`, and
  `X-Correlation-ID` headers.
- Preview the Risk Service decision live as you type the amount, with a
  cents → dollars echo.
- Keep the bearer value in memory only; persist just the API base URL.
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

The default API base URL is `http://127.0.0.1:8080` (editable via the
**Connection** panel). If the browser blocks API calls, allow the frontend
origin in the root `.env`:

```env
CORS_ORIGINS=http://127.0.0.1:5173
```

Set the bearer value in the **Connection** panel to match
`TRANSACTION_API_AUTH_VALUE`.
