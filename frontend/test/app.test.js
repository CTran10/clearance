import test from "node:test";
import assert from "node:assert/strict";

import {
  buildAuthHeaders,
  decisionClass,
  formatCurrency,
  normalizeApiBaseUrl,
  sortNewestFirst,
  summarizeDecisions,
} from "../app.js";

test("normalizes blank API base URL to local FastAPI default", () => {
  assert.equal(normalizeApiBaseUrl(""), "http://127.0.0.1:8000");
});

test("normalizes API base URL by trimming trailing slashes", () => {
  assert.equal(normalizeApiBaseUrl(" http://localhost:9000/// "), "http://localhost:9000");
});

test("builds auth headers only when a token is available", () => {
  assert.deepEqual(buildAuthHeaders(""), {});
  assert.deepEqual(buildAuthHeaders("abc123"), { Authorization: "Bearer abc123" });
});

test("summarizes transaction decisions for dashboard metrics", () => {
  const summary = summarizeDecisions([
    { status: "approved" },
    { status: "approved" },
    { status: "review" },
    { status: "declined" },
  ]);

  assert.deepEqual(summary, {
    total: 4,
    approved: 2,
    declined: 1,
    review: 1,
  });
});

test("maps decision statuses to display classes", () => {
  assert.equal(decisionClass("approved"), "status status-approved");
  assert.equal(decisionClass("declined"), "status status-declined");
  assert.equal(decisionClass("review"), "status status-review");
  assert.equal(decisionClass("unknown"), "status");
});

test("formats currency amounts for the transaction table", () => {
  assert.equal(formatCurrency("125.5", "USD"), "$125.50");
});

test("sorts API records newest first", () => {
  const sorted = sortNewestFirst([
    { id: 1, created_at: "2026-01-01T10:00:00Z" },
    { id: 2, created_at: "2026-01-02T10:00:00Z" },
  ]);

  assert.deepEqual(sorted.map((item) => item.id), [2, 1]);
});
