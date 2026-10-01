import { useEffect, useRef, useState } from "react";

import { ApiError, depositFunds, getTransaction, submitTransaction } from "../lib/api.ts";
import { buildDepositRequest } from "../lib/funding.ts";
import { DEFAULT_API_BASE_URL } from "../lib/constants.ts";
import { createSafeId } from "../lib/ids.ts";
import { riskPreview } from "../lib/risk.ts";
import { assertSafeToken, buildTransactionPayload } from "../lib/transaction.ts";
import type { DepositInput, Receipt, Tone, TransactionInput, TransactionStatus } from "../types.ts";

const MAX_RECEIPTS = 12;

export interface Notice {
  tone: Tone;
  text: string;
}

interface ConsoleState {
  apiBaseUrl: string;
  authValue: string;
  fundingAuthValue: string;
  submitting: boolean;
  receipts: Receipt[];
  idempotencyKey: string;
  correlationId: string;
  notice: Notice | null;
}

function init(): ConsoleState {
  return {
    apiBaseUrl: (import.meta.env.VITE_API_BASE_URL?.trim() || DEFAULT_API_BASE_URL).replace(/\/+$/, ""),
    authValue: import.meta.env.DEV ? import.meta.env.VITE_TRANSACTION_API_AUTH_VALUE?.trim() || "" : "",
    fundingAuthValue: "",
    submitting: false,
    receipts: [],
    idempotencyKey: createSafeId("idem"),
    correlationId: createSafeId("trace"),
    notice: null,
  };
}

export interface SubmitFields extends TransactionInput {
  idempotencyKey: string;
  correlationId: string;
}

export interface DepositFields extends DepositInput {
  idempotencyKey: string;
  correlationId: string;
}

export type SubmissionResult = { ok: true; receipt: Receipt } | { ok: false; error: Error };

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return `HTTP ${error.status}: ${error.message}`;
  return error instanceof Error ? error.message : "Request failed";
}

function isTransactionStatus(value: unknown): value is TransactionStatus {
  return value === "PENDING" || value === "AUTHORIZED" || value === "FAILED";
}

export function useConsole() {
  const [state, setState] = useState(init);
  const requestInFlight = useRef(false);
  const pendingTransactionKey = state.receipts
    .filter((receipt) => receipt.kind === "PAYMENT" && receipt.status === "PENDING")
    .map((receipt) => receipt.transactionId)
    .join("|");

  useEffect(() => {
    const pendingTransactionIds = pendingTransactionKey ? pendingTransactionKey.split("|") : [];
    if (pendingTransactionIds.length === 0 || state.authValue === "") {
      return;
    }

    let stopped = false;
    let polling = false;
    const refresh = async () => {
      if (polling) {
        return;
      }
      polling = true;
      try {
        await Promise.all(
          pendingTransactionIds.map(async (transactionId) => {
            try {
              const detail = await getTransaction({
                baseUrl: state.apiBaseUrl,
                authValue: state.authValue,
                transactionId,
              });
              if (detail.transaction_id !== transactionId || detail.kind !== "PAYMENT" || !isTransactionStatus(detail.status) ||
                (detail.risk_level !== undefined && detail.risk_level !== "LOW" && detail.risk_level !== "HIGH") ||
                (detail.risk_reason !== undefined && typeof detail.risk_reason !== "string")) {
                throw new Error("The API returned an invalid transaction status response.");
              }
              if (!stopped) {
                setState((current) => ({
                  ...current,
                  receipts: current.receipts.map((receipt) =>
                    receipt.kind === "PAYMENT" && receipt.transactionId === transactionId ? {
                      ...receipt,
                      status: detail.status,
                      riskLevel: detail.risk_level,
                      riskReason: detail.risk_reason,
                      statusError: undefined,
                    } : receipt,
                  ),
                }));
              }
            } catch (error) {
              if (!stopped) {
                setState((current) => ({
                  ...current,
                  receipts: current.receipts.map((receipt) => receipt.transactionId === transactionId
                    ? { ...receipt, statusError: errorMessage(error) }
                    : receipt),
                }));
              }
            }
          }),
        );
      } finally {
        polling = false;
      }
    };

    void refresh();
    const interval = window.setInterval(refresh, 1_500);
    return () => {
      stopped = true;
      window.clearInterval(interval);
    };
  }, [state.apiBaseUrl, state.authValue, pendingTransactionKey]);

  async function submit(fields: SubmitFields): Promise<SubmissionResult> {
    if (requestInFlight.current) return { ok: false, error: new Error("A request is already in progress.") };
    requestInFlight.current = true;
    setState((current) => ({ ...current, submitting: true, notice: null }));
    try {
      const payload = buildTransactionPayload(fields);
      const idempotencyKey = assertSafeToken(fields.idempotencyKey, "idempotency key");
      const correlationId = assertSafeToken(fields.correlationId, "correlation id");

      const response = await submitTransaction({
        baseUrl: state.apiBaseUrl,
        authValue: state.authValue,
        payload,
        idempotencyKey,
        correlationId,
      });

      if (typeof response.transaction_id !== "string" || !isTransactionStatus(response.status) ||
        (response.correlation_id !== undefined && typeof response.correlation_id !== "string")) {
        throw new Error("The API returned an invalid transaction response.");
      }

      const preview = riskPreview(payload.amount_cents);
      const transactionId = assertSafeToken(response.transaction_id, "transaction id");
      const resolvedCorrelation = assertSafeToken(response.correlation_id || correlationId, "correlation id");
      const receipt: Receipt = {
        kind: "PAYMENT",
        transactionId,
        status: response.status,
        correlationId: resolvedCorrelation,
        idempotencyKey,
        accountId: payload.account_id,
        merchantId: payload.merchant_id,
        amountCents: payload.amount_cents,
        currency: payload.currency,
        previewRisk: preview.level,
        previewOutcome: preview.outcome,
        previewReason: preview.reason,
        createdAt: new Date().toISOString(),
      };
      recordReceipt(receipt);
      return { ok: true, receipt };
    } catch (error) {
      setState((current) => ({
        ...current,
        notice: { tone: "negative", text: errorMessage(error) },
      }));
      return { ok: false, error: error instanceof Error ? error : new Error("Request failed") };
    } finally {
      requestInFlight.current = false;
      setState((current) => ({ ...current, submitting: false }));
    }
  }

  async function deposit(fields: DepositFields): Promise<SubmissionResult> {
    if (requestInFlight.current) return { ok: false, error: new Error("A request is already in progress.") };
    requestInFlight.current = true;
    setState((current) => ({ ...current, submitting: true, notice: null }));
    try {
      if (!state.fundingAuthValue) throw new Error("Set the funding bearer in Connection first.");
      const { accountId, payload } = buildDepositRequest(fields);
      const idempotencyKey = assertSafeToken(fields.idempotencyKey, "idempotency key");
      const correlationId = assertSafeToken(fields.correlationId, "correlation id");
      const response = await depositFunds({ baseUrl: state.apiBaseUrl, authValue: state.fundingAuthValue,
        accountId, payload, idempotencyKey, correlationId });
      if (typeof response.transaction_id !== "string" || response.status !== "AUTHORIZED" ||
        response.account_id !== accountId || response.amount_cents !== payload.amount_cents || response.currency !== payload.currency ||
        !Number.isSafeInteger(response.balance_after_cents) || response.balance_after_cents < 0 ||
        typeof response.correlation_id !== "string") {
        throw new Error("The API returned an invalid deposit response.");
      }
      const receipt: Receipt = {
        kind: "DEPOSIT", transactionId: assertSafeToken(response.transaction_id, "transaction id"), status: response.status,
        accountId, amountCents: payload.amount_cents, currency: payload.currency, idempotencyKey,
        correlationId: assertSafeToken(response.correlation_id, "correlation id"), fundingSource: payload.funding_source,
        balanceAfterCents: response.balance_after_cents, createdAt: new Date().toISOString(),
      };
      recordReceipt(receipt);
      return { ok: true, receipt };
    } catch (error) {
      setState((current) => ({ ...current, notice: { tone: "negative", text: errorMessage(error) } }));
      return { ok: false, error: error instanceof Error ? error : new Error("Request failed") };
    } finally {
      requestInFlight.current = false;
      setState((current) => ({ ...current, submitting: false }));
    }
  }

  function recordReceipt(receipt: Receipt) {
    setState((current) => {
      const previous = current.receipts.find((item) => item.transactionId === receipt.transactionId);
      const label = receipt.kind === "DEPOSIT" ? "deposit" : "payment";
      return { ...current,
        receipts: [previous ?? receipt, ...current.receipts.filter((item) => item.transactionId !== receipt.transactionId)].slice(0, MAX_RECEIPTS),
        correlationId: receipt.correlationId, idempotencyKey: receipt.idempotencyKey,
        notice: { tone: previous ? "neutral" : "positive", text: previous
          ? `Existing ${label} ${receipt.transactionId} returned.`
          : receipt.kind === "DEPOSIT" ? "Funds deposited." : `Payment accepted as ${receipt.status}.` },
      };
    });
  }

  function regenerateKeys() {
    setState((current) => ({
      ...current,
      idempotencyKey: createSafeId("idem"),
      correlationId: createSafeId("trace"),
      notice: null,
    }));
  }

  function dismissNotice() {
    setState((current) => ({ ...current, notice: null }));
  }

  function configureConnection(values: { apiBaseUrl: string; authValue: string; fundingAuthValue: string }) {
    const url = new URL(values.apiBaseUrl.trim());
    if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
      throw new Error("Use an HTTP or HTTPS API URL without credentials, query parameters, or a fragment.");
    }
    setState((current) => ({
      ...current,
      apiBaseUrl: url.toString().replace(/\/+$/, ""),
      authValue: values.authValue.trim(),
      fundingAuthValue: values.fundingAuthValue.trim(),
      notice: { tone: "neutral", text: "Connection updated. Bearer values stay in memory." },
    }));
  }

  return { state, actions: { submit, deposit, regenerateKeys, dismissNotice, configureConnection } };
}
