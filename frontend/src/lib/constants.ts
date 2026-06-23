export const DEFAULT_API_BASE_URL = "http://127.0.0.1:8080";

/** Mirrors the Risk Service threshold: amounts strictly above this are HIGH risk. */
export const RISK_THRESHOLD_CENTS = 50_000;

/** Server-side guard (`^[A-Za-z0-9._:-]{1,128}$`) for header tokens and ids. */
export const SAFE_TOKEN_PATTERN = /^[A-Za-z0-9._:-]{1,128}$/;

export interface Option {
  value: string;
  label: string;
  hint?: string;
}

export const DEMO_ACCOUNTS: Option[] = [
  { value: "acct_123", label: "acct_123", hint: "Default funded account" },
  { value: "acct_empty", label: "acct_empty", hint: "Zero balance" },
  { value: "acct_attacker", label: "acct_attacker", hint: "Ownership mismatch" },
];

export const DEMO_MERCHANTS: Option[] = [
  { value: "merchant_123", label: "merchant_123", hint: "Default" },
  { value: "merchant_grocer", label: "merchant_grocer", hint: "Grocery" },
  { value: "merchant_travel", label: "merchant_travel", hint: "Travel" },
];

export const CURRENCIES: Option[] = [
  { value: "USD", label: "USD" },
  { value: "EUR", label: "EUR" },
  { value: "GBP", label: "GBP" },
];

export interface PipelineStage {
  id: string;
  service: string;
  detail: string;
}

export const PIPELINE_STAGES: PipelineStage[] = [
  {
    id: "transaction",
    service: "Transaction Service",
    detail: "Validates input, checks the Redis rate limit, records PENDING, and writes TransactionCreated to the outbox.",
  },
  {
    id: "outbox",
    service: "Outbox Publisher",
    detail: "Publishes TransactionCreated to Redpanda and marks the outbox row as published.",
  },
  {
    id: "risk",
    service: "Risk Service",
    detail: "Consumes the event. Amounts above $500.00 are scored HIGH risk.",
  },
  {
    id: "ledger",
    service: "Ledger Service",
    detail: "Checks the available balance, writes entries for funded LOW-risk transactions, and emits the final event.",
  },
];
