import { SAFE_TOKEN_PATTERN } from "./constants.ts";
import type { TransactionInput, TransactionPayload } from "../types.ts";

export function buildTransactionHeaders(
  authValue: string,
  idempotencyKey: string,
  correlationId: string,
): Record<string, string> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    "Idempotency-Key": String(idempotencyKey || "").trim(),
    "X-Correlation-ID": String(correlationId || "").trim(),
  };
  const bearer = String(authValue || "").trim();
  if (bearer) {
    headers.Authorization = `Bearer ${bearer}`;
  }
  return headers;
}

/**
 * Validates operator input and returns the exact wire payload. The Go service
 * uses DisallowUnknownFields, so the shape here must stay limited to these
 * four keys.
 */
// real talk: "DisallowUnknownFields" on the Go side means if i send ONE extra key (even a harmless typo like
// "amount_cent") the whole request gets rejected, not silently ignored. strict, kinda annoying, but it means
// the frontend and backend can never quietly drift apart without something loudly breaking. so the return below
// is deliberately exactly 4 keys, no more. adding a field here without adding it in Go = instant 400
export function buildTransactionPayload(input: TransactionInput): TransactionPayload {
  const accountId = String(input.accountId || "").trim();
  const merchantId = String(input.merchantId || "").trim();
  const amountText = String(input.amountCents || "").trim();
  const currency = String(input.currency || "").trim().toUpperCase();

  if (!accountId) {
    throw new Error("account id is required");
  }
  if (!merchantId) {
    throw new Error("merchant id is required");
  }
  if (!SAFE_TOKEN_PATTERN.test(accountId) || !SAFE_TOKEN_PATTERN.test(merchantId)) {
    throw new Error("ids must use safe characters only");
  }
  if (!/^\d+$/.test(amountText)) {
    throw new Error("amount cents must be a whole number");
  }
  const amountCents = Number(amountText);
  if (!Number.isSafeInteger(amountCents) || amountCents <= 0) {
    throw new Error("amount cents must be greater than zero");
  }
  if (!/^[A-Z]{3}$/.test(currency)) {
    throw new Error("currency must be a three-letter code");
  }

  return {
    account_id: accountId,
    merchant_id: merchantId,
    amount_cents: amountCents,
    currency,
  };
}

export function assertSafeToken(value: string, label: string): string {
  const trimmed = String(value || "").trim();
  if (!SAFE_TOKEN_PATTERN.test(trimmed)) {
    throw new Error(`${label} must use safe characters only`);
  }
  return trimmed;
}
