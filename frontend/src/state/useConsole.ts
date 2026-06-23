import { useCallback, useMemo, useReducer } from "react";

import { checkHealth, submitTransaction } from "../lib/api.ts";
import { createSafeId } from "../lib/ids.ts";
import { riskPreview } from "../lib/risk.ts";
import { loadApiBaseUrl, saveApiBaseUrl } from "../lib/storage.ts";
import { assertSafeToken, buildTransactionPayload } from "../lib/transaction.ts";
import type { HealthState, Receipt, Tone, TransactionInput, TransactionStatus } from "../types.ts";

const MAX_RECEIPTS = 12;

export interface Notice {
  tone: Tone;
  text: string;
}

export interface ConsoleState {
  apiBaseUrl: string;
  authValue: string;
  health: HealthState;
  submitting: boolean;
  receipts: Receipt[];
  idempotencyKey: string;
  correlationId: string;
  notice: Notice | null;
}

type Action =
  | { type: "set-health"; health: HealthState }
  | { type: "set-submitting"; submitting: boolean }
  | { type: "add-receipt"; receipt: Receipt; correlationId: string }
  | { type: "save-settings"; apiBaseUrl: string; authValue: string }
  | { type: "regenerate-keys" }
  | { type: "notice"; notice: Notice }
  | { type: "dismiss-notice" };

function init(): ConsoleState {
  return {
    apiBaseUrl: loadApiBaseUrl(),
    authValue: "",
    health: "unknown",
    submitting: false,
    receipts: [],
    idempotencyKey: createSafeId("idem"),
    correlationId: createSafeId("trace"),
    notice: null,
  };
}

// every case here returns a BRAND NEW {...state} object instead of poking state.whatever = x. that's not me being
// fancy — react decides whether to re-render by checking if the state object is a *different reference*. mutate in
// place and the reference is the same, so react goes "nothing changed" and your UI just... doesn't update. baffling
// for an afternoon until it clicked. reducer + dispatch keeps all this state logic in one predictable place too
function reducer(state: ConsoleState, action: Action): ConsoleState {
  switch (action.type) {
    case "set-health":
      return { ...state, health: action.health };
    case "set-submitting":
      return { ...state, submitting: action.submitting };
    case "add-receipt":
      // newest on top ([new, ...old]) and .slice(0, 12) so this list can't grow forever and eat memory in a long session.
      // it's a demo console, nobody needs to scroll 4000 receipts — keep the last dozen and let the rest fall off
      return {
        ...state,
        receipts: [action.receipt, ...state.receipts].slice(0, MAX_RECEIPTS),
        correlationId: action.correlationId,
        idempotencyKey: action.receipt.idempotencyKey,
      };
    case "save-settings":
      return { ...state, apiBaseUrl: action.apiBaseUrl, authValue: action.authValue };
    case "regenerate-keys":
      return {
        ...state,
        idempotencyKey: createSafeId("idem"),
        correlationId: createSafeId("trace"),
        notice: { tone: "neutral", text: "Generated fresh idempotency and correlation ids." },
      };
    case "notice":
      return { ...state, notice: action.notice };
    case "dismiss-notice":
      return { ...state, notice: null };
    default:
      return state;
  }
}

export interface SubmitFields extends TransactionInput {
  idempotencyKey: string;
  correlationId: string;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Request failed";
}

export function useConsole() {
  const [state, dispatch] = useReducer(reducer, undefined, init);

  const saveSettings = useCallback((apiBaseUrl: string, authValue: string) => {
    const normalized = saveApiBaseUrl(apiBaseUrl);
    dispatch({ type: "save-settings", apiBaseUrl: normalized, authValue });
    dispatch({
      type: "notice",
      notice: { tone: "positive", text: "Connection saved. The bearer value stays in this tab only." },
    });
  }, []);

  const runHealthCheck = useCallback(async () => {
    dispatch({ type: "set-health", health: "checking" });
    dispatch({ type: "dismiss-notice" });
    try {
      await checkHealth(state.apiBaseUrl);
      dispatch({ type: "set-health", health: "ok" });
      dispatch({ type: "notice", notice: { tone: "positive", text: "Health check passed." } });
    } catch (error) {
      dispatch({ type: "set-health", health: "down" });
      dispatch({ type: "notice", notice: { tone: "negative", text: errorMessage(error) } });
    }
  }, [state.apiBaseUrl]);

  const submit = useCallback(
    async (fields: SubmitFields) => {
      dispatch({ type: "set-submitting", submitting: true });
      dispatch({ type: "dismiss-notice" });
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
        dispatch({ type: "add-receipt", receipt, correlationId: resolvedCorrelation });
        dispatch({
          type: "notice",
          notice: { tone: "positive", text: `Accepted as ${receipt.status}. Event handed to the outbox.` },
        });
        return true;
      } catch (error) {
        dispatch({ type: "notice", notice: { tone: "negative", text: errorMessage(error) } });
        return false;
      } finally {
        dispatch({ type: "set-submitting", submitting: false });
      }
    },
    [state.apiBaseUrl, state.authValue],
  );

  const regenerateKeys = useCallback(() => dispatch({ type: "regenerate-keys" }), []);
  const dismissNotice = useCallback(() => dispatch({ type: "dismiss-notice" }), []);

  const actions = useMemo(
    () => ({ saveSettings, runHealthCheck, submit, regenerateKeys, dismissNotice }),
    [saveSettings, runHealthCheck, submit, regenerateKeys, dismissNotice],
  );

  return { state, actions };
}
