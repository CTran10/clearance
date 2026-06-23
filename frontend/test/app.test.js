import test from "node:test";
import assert from "node:assert/strict";

import {
  buildTransactionHeaders,
  buildTransactionPayload,
  formatAmountCents,
  normalizeApiBaseUrl,
  parseApiError,
  riskPreview,
  statusClass,
  summarizeReceipts,
} from "../app.js";

test("normalizes blank API base URL to local Go service default", () => {
  assert.equal(normalizeApiBaseUrl(""), "http://127.0.0.1:8080");
});

test("normalizes API base URL by trimming trailing slashes", () => {
  assert.equal(normalizeApiBaseUrl(" http://localhost:9000/// "), "http://localhost:9000");
});

test("builds transaction headers without leaking bearer values elsewhere", () => {
  assert.deepEqual(buildTransactionHeaders("", "idem-123", "trace-123"), {
    "Content-Type": "application/json",
    "Idempotency-Key": "idem-123",
    "X-Correlation-ID": "trace-123",
  });
  assert.deepEqual(buildTransactionHeaders("local bearer", "idem-123", "trace-123"), {
    Authorization: "Bearer local bearer",
    "Content-Type": "application/json",
    "Idempotency-Key": "idem-123",
    "X-Correlation-ID": "trace-123",
  });
});

test("builds transaction payload for the Go API contract", () => {
  assert.deepEqual(
    buildTransactionPayload({
      accountId: " acct_123 ",
      merchantId: " merchant_123 ",
      amountCents: "12550",
      currency: "usd",
    }),
    {
      account_id: "acct_123",
      merchant_id: "merchant_123",
      amount_cents: 12550,
      currency: "USD",
    },
  );
});

test("rejects invalid transaction payload values", () => {
  assert.throws(() => {
    buildTransactionPayload({
      accountId: "",
      merchantId: "merchant_123",
      amountCents: "12550",
      currency: "USD",
    });
  }, /account id is required/);

  assert.throws(() => {
    buildTransactionPayload({
      accountId: "acct_123",
      merchantId: "merchant_123",
      amountCents: "12.50",
      currency: "USD",
    });
  }, /amount cents must be a whole number/);
});

test("previews risk with the same threshold as the Risk Service", () => {
  assert.deepEqual(riskPreview(50_000), {
    level: "LOW",
    outcome: "likely authorized",
    reason: "Amount is at or below 500.00.",
  });
  assert.deepEqual(riskPreview(50_001), {
    level: "HIGH",
    outcome: "likely failed",
    reason: "Amount is greater than 500.00.",
  });
});

test("summarizes local transaction receipts", () => {
  const summary = summarizeReceipts([
    { status: "PENDING", previewRisk: "LOW" },
    { status: "PENDING", previewRisk: "HIGH" },
    { status: "FAILED", previewRisk: "HIGH" },
  ]);

  assert.deepEqual(summary, {
    total: 3,
    pending: 2,
    lowRisk: 1,
    highRisk: 2,
  });
});

test("maps platform statuses to display classes", () => {
  assert.equal(statusClass("PENDING"), "status status-pending");
  assert.equal(statusClass("AUTHORIZED"), "status status-authorized");
  assert.equal(statusClass("FAILED"), "status status-failed");
  assert.equal(statusClass("LOW"), "status status-low");
  assert.equal(statusClass("HIGH"), "status status-high");
  assert.equal(statusClass("unknown"), "status");
});

test("formats integer cents without floating point money math", () => {
  assert.equal(formatAmountCents(12550, "USD"), "$125.50");
});

test("parses Go API error responses", async () => {
  const response = new Response(JSON.stringify({ error: "invalid request" }), {
    status: 400,
    headers: { "Content-Type": "application/json" },
  });

  assert.equal(await parseApiError(response), "invalid request");
});
