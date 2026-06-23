import { describe, expect, test } from "vitest";

import { buildTransactionHeaders, buildTransactionPayload } from "../../src/lib/transaction.ts";

describe("buildTransactionHeaders", () => {
  test("omits Authorization when no bearer value is set", () => {
    expect(buildTransactionHeaders("", "idem-123", "trace-123")).toEqual({
      "Content-Type": "application/json",
      "Idempotency-Key": "idem-123",
      "X-Correlation-ID": "trace-123",
    });
  });

  test("adds a Bearer Authorization header when a bearer value is present", () => {
    expect(buildTransactionHeaders("local bearer", "idem-123", "trace-123")).toEqual({
      Authorization: "Bearer local bearer",
      "Content-Type": "application/json",
      "Idempotency-Key": "idem-123",
      "X-Correlation-ID": "trace-123",
    });
  });
});

describe("buildTransactionPayload", () => {
  test("trims input and produces the exact Go API contract", () => {
    expect(
      buildTransactionPayload({
        accountId: " acct_123 ",
        merchantId: " merchant_123 ",
        amountCents: "12550",
        currency: "usd",
      }),
    ).toEqual({
      account_id: "acct_123",
      merchant_id: "merchant_123",
      amount_cents: 12550,
      currency: "USD",
    });
  });

  test("requires an account id", () => {
    expect(() =>
      buildTransactionPayload({
        accountId: "",
        merchantId: "merchant_123",
        amountCents: "12550",
        currency: "USD",
      }),
    ).toThrow(/account id is required/);
  });

  test("rejects a non-integer amount", () => {
    expect(() =>
      buildTransactionPayload({
        accountId: "acct_123",
        merchantId: "merchant_123",
        amountCents: "12.50",
        currency: "USD",
      }),
    ).toThrow(/amount cents must be a whole number/);
  });

  test("rejects ids with unsafe characters", () => {
    expect(() =>
      buildTransactionPayload({
        accountId: "acct 123",
        merchantId: "merchant_123",
        amountCents: "100",
        currency: "USD",
      }),
    ).toThrow(/safe characters only/);
  });
});
