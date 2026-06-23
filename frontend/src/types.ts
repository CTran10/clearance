export type RiskLevel = "LOW" | "HIGH";

export type TransactionStatus = "PENDING" | "AUTHORIZED" | "FAILED";

/** Visual tone shared by status pills, risk badges, and notices. */
export type Tone = "positive" | "pending" | "negative" | "neutral";

export interface TransactionInput {
  accountId: string;
  merchantId: string;
  amountCents: string;
  currency: string;
}

/** Exact wire shape accepted by the Go Transaction Service (DisallowUnknownFields). */
export interface TransactionPayload {
  account_id: string;
  merchant_id: string;
  amount_cents: number;
  currency: string;
}

export interface TransactionResponse {
  transaction_id: string;
  status: string;
  correlation_id?: string;
}

export interface RiskPreview {
  level: RiskLevel;
  outcome: string;
  reason: string;
}

export interface Receipt {
  transactionId: string;
  status: TransactionStatus;
  correlationId: string;
  idempotencyKey: string;
  accountId: string;
  merchantId: string;
  amountCents: number;
  currency: string;
  previewRisk: RiskLevel;
  previewOutcome: string;
  previewReason: string;
  createdAt: string;
}

export interface ReceiptSummary {
  total: number;
  pending: number;
  lowRisk: number;
  highRisk: number;
}

export type HealthState = "unknown" | "checking" | "ok" | "down";
