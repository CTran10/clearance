import { afterEach, describe, expect, test, vi } from "vitest";

import { ApiError, depositFunds, getTransaction, parseApiError, submitTransaction } from "../../src/lib/api.ts";

afterEach(() => {
  vi.unstubAllGlobals();
});

test("preserves the HTTP error status for an expected idempotency conflict", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: "idempotency conflict" }), { status: 409 })));
  const request = submitTransaction({ baseUrl: "http://localhost:9000", authValue: "transaction-test",
    idempotencyKey: "idem-test", correlationId: "trace-test",
    payload: { account_id: "acct-test", merchant_id: "merchant-test", amount_cents: 12551, currency: "USD" },
  });
  await expect(request).rejects.toBeInstanceOf(ApiError);
  await expect(request).rejects.toMatchObject({ status: 409, message: "idempotency conflict" });
});

test("funding forwards the requested amount and audit fields with its scoped bearer", async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response("{}", { status: 201 }));
  vi.stubGlobal("fetch", fetchMock);
  await depositFunds({ baseUrl: "http://localhost:9000", authValue: "funding-test", accountId: "acct-funding",
    idempotencyKey: "fund-key", correlationId: "trace-test", payload: { amount_cents: 23000, currency: "USD",
      funding_source: "manual", external_reference: "fund-key", operator_reason: "Account funding" } });
  expect(fetchMock).toHaveBeenCalledWith("http://localhost:9000/accounts/acct-funding/deposits", expect.objectContaining({
    method: "POST", headers: expect.objectContaining({ Authorization: "Bearer funding-test" }),
    body: JSON.stringify({ amount_cents: 23000, currency: "USD", funding_source: "manual",
      external_reference: "fund-key", operator_reason: "Account funding" }),
  }));
});

describe("parseApiError", () => {
  test("reads the Go API 'error' field", async () => {
    const response = new Response(JSON.stringify({ error: "invalid request" }), {
      status: 400,
      headers: { "Content-Type": "application/json" },
    });
    expect(await parseApiError(response)).toBe("invalid request");
  });

  test("falls back to the status code when the body is not JSON", async () => {
    const response = new Response("not json", { status: 503 });
    expect(await parseApiError(response)).toBe("Request failed with 503");
  });
});

describe("getTransaction", () => {
  test("loads the durable transaction status with bearer authorization", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          transaction_id: "txn_123",
          kind: "PAYMENT",
          account_id: "acct_123",
          merchant_id: "merchant_123",
          amount_cents: 1250,
          currency: "USD",
          status: "AUTHORIZED",
          correlation_id: "trace_123",
          created_at: "2026-07-13T00:00:00Z",
          updated_at: "2026-07-13T00:00:01Z",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const transaction = await getTransaction({
      baseUrl: "http://127.0.0.1:8080",
      authValue: "local-token",
      transactionId: "txn_123",
    });

    expect(transaction.status).toBe("AUTHORIZED");
    expect(fetchMock).toHaveBeenCalledWith("http://127.0.0.1:8080/transactions/txn_123", {
      method: "GET",
      headers: { Authorization: "Bearer local-token" },
    });
  });
});
