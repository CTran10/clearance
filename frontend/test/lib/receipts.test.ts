import { describe, expect, test } from "vitest";

import { riskTone, statusTone, summarizeReceipts } from "../../src/lib/receipts.ts";

describe("summarizeReceipts", () => {
  test("counts totals, pending replies, and risk previews", () => {
    expect(
      summarizeReceipts([
        { status: "PENDING", previewRisk: "LOW" },
        { status: "PENDING", previewRisk: "HIGH" },
        { status: "FAILED", previewRisk: "HIGH" },
      ]),
    ).toEqual({ total: 3, pending: 2, lowRisk: 1, highRisk: 2 });
  });

  test("returns a zeroed summary for an empty list", () => {
    expect(summarizeReceipts([])).toEqual({ total: 0, pending: 0, lowRisk: 0, highRisk: 0 });
  });
});

describe("tone mapping", () => {
  test("maps platform statuses to semantic tones", () => {
    expect(statusTone("AUTHORIZED")).toBe("positive");
    expect(statusTone("PENDING")).toBe("pending");
    expect(statusTone("FAILED")).toBe("negative");
    expect(statusTone("anything-else")).toBe("neutral");
  });

  test("maps risk levels to semantic tones", () => {
    expect(riskTone("LOW")).toBe("positive");
    expect(riskTone("HIGH")).toBe("negative");
  });
});
