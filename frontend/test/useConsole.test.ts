import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";

import { depositFunds, getTransaction, submitTransaction } from "../src/lib/api.ts";
import type { DepositResponse } from "../src/types.ts";
import { useConsole } from "../src/state/useConsole.ts";

vi.mock("../src/lib/api.ts", async (importOriginal) => ({
  ...await importOriginal<typeof import("../src/lib/api.ts")>(),
  getTransaction: vi.fn(),
  submitTransaction: vi.fn(),
  depositFunds: vi.fn(),
}));

const fields = {
  accountId: "acct_123",
  merchantId: "merchant_123",
  amountCents: "12550",
  currency: "USD",
  idempotencyKey: "idem-test",
  correlationId: "trace-test",
};

beforeEach(() => {
  vi.stubEnv("VITE_API_BASE_URL", "");
  vi.stubEnv("VITE_TRANSACTION_API_AUTH_VALUE", "");
});

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
  vi.unstubAllEnvs();
  vi.useRealTimers();
});

test.each(["", "   "])("uses the local Go service default for a blank API URL (%j)", (baseUrl) => {
  vi.stubEnv("VITE_API_BASE_URL", baseUrl);
  const { result } = renderHook(() => useConsole());
  expect(result.current.state.apiBaseUrl).toBe("http://127.0.0.1:8080");
});

test("uses the local demo configuration and tracks an accepted transaction to completion", async () => {
  vi.stubEnv("VITE_API_BASE_URL", " http://localhost:9000/// ");
  vi.stubEnv("VITE_TRANSACTION_API_AUTH_VALUE", " local-token ");
  vi.mocked(submitTransaction).mockResolvedValue({
    transaction_id: "txn-test",
    status: "PENDING",
    correlation_id: "trace-server",
  });
  vi.mocked(getTransaction).mockRejectedValueOnce(new Error("temporarily unavailable"));
  const { result } = renderHook(() => useConsole());

  vi.useFakeTimers();
  await act(() => result.current.actions.submit(fields));
  expect(submitTransaction).toHaveBeenCalledWith({
    baseUrl: "http://localhost:9000",
    authValue: "local-token",
    payload: { account_id: "acct_123", merchant_id: "merchant_123", amount_cents: 12550, currency: "USD" },
    idempotencyKey: "idem-test",
    correlationId: "trace-test",
  });
  expect(result.current.state).toMatchObject({
    submitting: false,
    correlationId: "trace-server",
    idempotencyKey: "idem-test",
    receipts: [{ transactionId: "txn-test", status: "PENDING", previewRisk: "LOW" }],
  });
  expect(getTransaction).toHaveBeenCalledOnce();
  expect(result.current.state.receipts[0].status).toBe("PENDING");

  vi.mocked(getTransaction).mockResolvedValue({
    transaction_id: "txn-test",
    kind: "PAYMENT",
    account_id: "acct_123",
    amount_cents: 12550,
    currency: "USD",
    status: "AUTHORIZED",
    correlation_id: "trace-server",
    created_at: "2026-09-30T00:00:00Z",
    updated_at: "2026-09-30T00:00:01Z",
  });
  await act(() => vi.advanceTimersByTimeAsync(1_500));
  expect(result.current.state.receipts[0].status).toBe("AUTHORIZED");
  await act(() => vi.advanceTimersByTimeAsync(1_500));
  expect(getTransaction).toHaveBeenCalledTimes(2);
});

test("reports validation and request failures without leaving submission active", async () => {
  const { result } = renderHook(() => useConsole());
  await act(() => result.current.actions.submit({ ...fields, amountCents: "12.50" }));
  expect(submitTransaction).not.toHaveBeenCalled();
  expect(result.current.state.notice?.text).toBe("amount cents must be a whole number");

  vi.mocked(submitTransaction).mockRejectedValue(new Error("insufficient funds"));
  await act(() => result.current.actions.submit(fields));
  expect(result.current.state).toMatchObject({
    submitting: false,
    receipts: [],
    notice: { tone: "negative", text: "insufficient funds" },
  });

  act(() => result.current.actions.dismissNotice());
  expect(result.current.state.notice).toBeNull();
});

test("does not send the local demo bearer outside development", async () => {
  vi.stubEnv("DEV", false);
  vi.stubEnv("VITE_TRANSACTION_API_AUTH_VALUE", "local-token");
  vi.mocked(submitTransaction).mockRejectedValue(new Error("unauthorized"));
  const { result } = renderHook(() => useConsole());
  await act(() => result.current.actions.submit(fields));
  expect(submitTransaction).toHaveBeenCalledWith(expect.objectContaining({ authValue: "" }));
});

test("keeps only the latest twelve receipts and regenerates request keys on demand", async () => {
  const { result } = renderHook(() => useConsole());
  for (let index = 0; index < 13; index++) {
    vi.mocked(submitTransaction).mockResolvedValue({ transaction_id: `txn-${index}`, status: "PENDING" });
    await act(() => result.current.actions.submit(fields));
  }
  expect(result.current.state.receipts.map((receipt) => receipt.transactionId)).toEqual(
    Array.from({ length: 12 }, (_, index) => `txn-${12 - index}`),
  );
  expect(result.current.state.correlationId).toBe(fields.correlationId);
  expect(getTransaction).not.toHaveBeenCalled();
  act(() => result.current.actions.regenerateKeys());
  expect(result.current.state.idempotencyKey).not.toBe(fields.idempotencyKey);
  expect(result.current.state.correlationId).not.toBe(fields.correlationId);
});

test("replaying creation keeps one receipt and does not regress its known final state", async () => {
  vi.stubEnv("VITE_TRANSACTION_API_AUTH_VALUE", "local-token");
  vi.mocked(submitTransaction).mockResolvedValue({ transaction_id: "txn-replay", status: "PENDING" });
  vi.mocked(getTransaction).mockResolvedValue({
    transaction_id: "txn-replay", kind: "PAYMENT", account_id: "acct_123", amount_cents: 12550,
    currency: "USD", status: "AUTHORIZED", risk_level: "LOW", risk_reason: "amount is at or below 500.00",
    correlation_id: "trace-test", created_at: "2026-09-30T00:00:00Z", updated_at: "2026-09-30T00:00:01Z",
  });
  const { result } = renderHook(() => useConsole());
  await act(async () => { await result.current.actions.submit(fields); });
  expect(result.current.state.receipts[0].status).toBe("AUTHORIZED");
  await act(async () => { await result.current.actions.submit(fields); });
  expect(result.current.state.receipts).toHaveLength(1);
  expect(result.current.state.receipts[0]).toMatchObject({ status: "AUTHORIZED", riskLevel: "LOW" });
  expect(getTransaction).toHaveBeenCalledOnce();
});

test.each([
  { transaction_id: "different", kind: "PAYMENT" as const },
  { transaction_id: "txn-status", kind: "DEPOSIT" as const },
])("an invalid status response preserves pending state and retries on the interval (%j)", async (invalid) => {
  vi.stubEnv("VITE_TRANSACTION_API_AUTH_VALUE", "local-token");
  vi.mocked(submitTransaction).mockResolvedValue({ transaction_id: "txn-status", status: "PENDING" });
  vi.mocked(getTransaction).mockResolvedValue({
    ...invalid, account_id: "acct_123", amount_cents: 12550,
    currency: "USD", status: "AUTHORIZED", correlation_id: "trace-test",
    created_at: "2026-09-30T00:00:00Z", updated_at: "2026-09-30T00:00:01Z",
  });
  const { result } = renderHook(() => useConsole());
  vi.useFakeTimers();
  await act(async () => { await result.current.actions.submit(fields); });
  expect(result.current.state.receipts[0]).toMatchObject({ status: "PENDING", statusError: "The API returned an invalid transaction status response." });
  expect(getTransaction).toHaveBeenCalledOnce();
  await act(() => vi.advanceTimersByTimeAsync(1_500));
  expect(getTransaction).toHaveBeenCalledTimes(2);
});

test("an in-flight deposit blocks a concurrent payment and releases the console after completion", async () => {
  let finish: ((value: DepositResponse) => void) | undefined;
  vi.mocked(depositFunds).mockReturnValue(new Promise<DepositResponse>((resolve) => { finish = resolve; }));
  const { result } = renderHook(() => useConsole());
  act(() => result.current.actions.configureConnection({ apiBaseUrl: "http://localhost:9000", authValue: "transaction-test", fundingAuthValue: "funding-test" }));
  let first: ReturnType<typeof result.current.actions.deposit> | undefined;
  act(() => { first = result.current.actions.deposit({ ...fields, fundingSource: "manual", externalReference: "ref-funding", operatorReason: "Account funding" }); });
  expect(result.current.state.submitting).toBe(true);
  await act(async () => {
    expect(await result.current.actions.submit(fields)).toMatchObject({ ok: false, error: { message: "A request is already in progress." } });
  });
  expect(submitTransaction).not.toHaveBeenCalled();
  if (!finish || !first) throw new Error("The deferred deposit was not started.");
  finish({ transaction_id: "dep-concurrent", deposit_id: "dep-concurrent", status: "AUTHORIZED", account_id: fields.accountId,
    amount_cents: 12550, currency: "USD", balance_after_cents: 12550, correlation_id: fields.correlationId });
  await act(async () => { await first; });
  expect(result.current.state.submitting).toBe(false);
  expect(result.current.state.receipts[0]).toMatchObject({ kind: "DEPOSIT", status: "AUTHORIZED", balanceAfterCents: 12550 });
  vi.mocked(submitTransaction).mockResolvedValue({ transaction_id: "txn-after-deposit", status: "PENDING" });
  await act(async () => { await result.current.actions.submit(fields); });
  expect(submitTransaction).toHaveBeenCalledOnce();
});
