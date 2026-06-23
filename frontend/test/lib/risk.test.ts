import { describe, expect, test } from "vitest";

import { riskPreview } from "../../src/lib/risk.ts";

describe("riskPreview", () => {
  test("treats the threshold amount as LOW risk", () => {
    expect(riskPreview(50_000)).toEqual({
      level: "LOW",
      outcome: "likely authorized",
      reason: "Amount is at or below 500.00.",
    });
  });

  test("treats amounts above the threshold as HIGH risk", () => {
    expect(riskPreview(50_001)).toEqual({
      level: "HIGH",
      outcome: "likely failed",
      reason: "Amount is greater than 500.00.",
    });
  });
});
