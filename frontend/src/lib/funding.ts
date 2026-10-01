import type { DepositInput, DepositPayload } from "../types.ts";
import { assertSafeToken } from "./transaction.ts";

export function buildDepositRequest(input: DepositInput): { accountId: string; payload: DepositPayload } {
  const accountId = assertSafeToken(input.accountId, "account id");
  const fundingSource = assertSafeToken(input.fundingSource, "funding source");
  const externalReference = assertSafeToken(input.externalReference, "external reference");
  const amount = input.amountCents.trim();
  if (!/^\d+$/.test(amount)) throw new Error("amount cents must be a whole number");
  const amountCents = Number(amount);
  if (!Number.isSafeInteger(amountCents) || amountCents <= 0) throw new Error("amount cents must be greater than zero");
  const currency = input.currency.trim().toUpperCase();
  if (!/^[A-Z]{3}$/.test(currency)) throw new Error("currency must be a three-letter code");
  const reason = input.operatorReason.trim();
  if (!reason || new TextEncoder().encode(reason).length > 256 || /[\u0000-\u001f\u007f-\u009f]/.test(reason)) {
    throw new Error("operator reason must be 1–256 bytes without control characters");
  }
  return { accountId, payload: { amount_cents: amountCents, currency, funding_source: fundingSource,
    external_reference: externalReference, operator_reason: reason } };
}
