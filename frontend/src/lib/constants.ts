export const DEFAULT_API_BASE_URL = "http://127.0.0.1:8080";

/** Mirrors the Risk Service threshold: amounts strictly above this are HIGH risk. */
export const RISK_THRESHOLD_CENTS = 50_000;

/** Server-side guard (`^[A-Za-z0-9._:-]{1,128}$`) for header tokens and ids. */
export const SAFE_TOKEN_PATTERN = /^[A-Za-z0-9._:-]{1,128}$/;

export interface Option {
  value: string;
  hint?: string;
}

export const DEMO_ACCOUNTS: Option[] = [
  { value: "acct_123", hint: "Default funded account" },
  { value: "acct_empty", hint: "Zero balance" },
  { value: "acct_attacker", hint: "Ownership mismatch" },
];

export const DEMO_MERCHANTS: Option[] = [
  { value: "merchant_123", hint: "Default" },
  { value: "merchant_grocer", hint: "Grocery" },
  { value: "merchant_travel", hint: "Travel" },
];

export const CURRENCIES: Option[] = [
  { value: "USD" },
  { value: "EUR" },
  { value: "GBP" },
];
