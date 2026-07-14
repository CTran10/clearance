import { afterEach, describe, expect, test, vi } from "vitest";

import { getTransaction, parseApiError } from "../../src/lib/api.ts";
import { normalizeApiBaseUrl } from "../../src/lib/storage.ts";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("normalizeApiBaseUrl", () => {
  test("falls back to the local Go service default for blank input", () => {
    expect(normalizeApiBaseUrl("")).toBe("http://127.0.0.1:8080");
  });

  test("trims whitespace and trailing slashes", () => {
    expect(normalizeApiBaseUrl(" http://localhost:9000/// ")).toBe("http://localhost:9000");
  });
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
