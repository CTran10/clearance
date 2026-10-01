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

export interface DepositInput {
  accountId: string;
  amountCents: string;
  currency: string;
  fundingSource: string;
  externalReference: string;
  operatorReason: string;
}

export interface DepositPayload {
  amount_cents: number;
  currency: string;
  funding_source: string;
  external_reference: string;
  operator_reason: string;
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

export interface TransactionDetail {
  transaction_id: string;
  kind: "PAYMENT" | "DEPOSIT";
  account_id: string;
  merchant_id?: string;
  funding_source?: string;
  external_reference?: string;
  amount_cents: number;
  currency: string;
  status: TransactionStatus;
  risk_level?: RiskLevel;
  risk_reason?: string;
  correlation_id: string;
  created_at: string;
  updated_at: string;
}

export interface RiskPreview {
  level: RiskLevel;
  outcome: string;
  reason: string;
}

interface ReceiptBase {
  transactionId: string;
  status: TransactionStatus;
  correlationId: string;
  idempotencyKey: string;
  accountId: string;
  amountCents: number;
  currency: string;
  createdAt: string;
  statusError?: string;
}

export type Receipt = ReceiptBase & (
  | { kind: "PAYMENT"; merchantId: string; previewRisk: RiskLevel; previewOutcome: string;
      previewReason: string; riskLevel?: RiskLevel; riskReason?: string }
  | { kind: "DEPOSIT"; fundingSource: string; balanceAfterCents: number; previewRisk?: never }
);

export interface DepositResponse {
  deposit_id: string;
  transaction_id: string;
  status: TransactionStatus;
  account_id: string;
  amount_cents: number;
  currency: string;
  balance_after_cents: number;
  correlation_id: string;
}

export interface ReceiptSummary {
  total: number;
  pending: number;
  lowRisk: number;
  highRisk: number;
}
