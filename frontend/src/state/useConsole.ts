import { useEffect, useState } from "react";

import { getTransaction, submitTransaction } from "../lib/api.ts";
import { DEFAULT_API_BASE_URL } from "../lib/constants.ts";
import { createSafeId } from "../lib/ids.ts";
import { riskPreview } from "../lib/risk.ts";
import { assertSafeToken, buildTransactionPayload } from "../lib/transaction.ts";
import type { Receipt, Tone, TransactionInput, TransactionStatus } from "../types.ts";

const MAX_RECEIPTS = 12;

export interface Notice {
  tone: Tone;
  text: string;
}

interface ConsoleState {
  apiBaseUrl: string;
  authValue: string;
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

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Request failed";
}

export function useConsole() {
  const [state, setState] = useState(init);

  useEffect(() => {
    const pendingTransactionIds = state.receipts
      .filter((receipt) => receipt.status === "PENDING")
      .map((receipt) => receipt.transactionId);
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
              if (!stopped && detail.status !== "PENDING") {
                setState((current) => ({
                  ...current,
                  receipts: current.receipts.map((receipt) =>
                    receipt.transactionId === transactionId ? { ...receipt, status: detail.status } : receipt,
                  ),
                }));
              }
            } catch {
              // A temporary read failure must not erase the accepted receipt. The next interval retries it.
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
  }, [state.apiBaseUrl, state.authValue, state.receipts]);

  async function submit(fields: SubmitFields) {
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

      const preview = riskPreview(payload.amount_cents);
      const resolvedCorrelation = response.correlation_id || correlationId;
      const receipt: Receipt = {
        transactionId: response.transaction_id,
        status: (response.status as TransactionStatus) || "PENDING",
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
      setState((current) => ({
        ...current,
        receipts: [receipt, ...current.receipts].slice(0, MAX_RECEIPTS),
        correlationId: resolvedCorrelation,
        idempotencyKey,
        notice: { tone: "positive", text: `Accepted as ${receipt.status}. Event handed to the outbox.` },
      }));
    } catch (error) {
      setState((current) => ({
        ...current,
        notice: { tone: "negative", text: errorMessage(error) },
      }));
    } finally {
      setState((current) => ({ ...current, submitting: false }));
    }
  }

  function regenerateKeys() {
    setState((current) => ({
      ...current,
      idempotencyKey: createSafeId("idem"),
      correlationId: createSafeId("trace"),
      notice: { tone: "neutral", text: "Generated fresh idempotency and correlation ids." },
    }));
  }

  function dismissNotice() {
    setState((current) => ({ ...current, notice: null }));
  }

  return { state, actions: { submit, regenerateKeys, dismissNotice } };
}
