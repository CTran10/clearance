import { describe, expect, test } from "vitest";

import { centsToDollarString, formatAmountCents, formatRelativeTime } from "../../src/lib/format.ts";

describe("formatAmountCents", () => {
  test("formats integer cents as currency without floating-point math", () => {
    expect(formatAmountCents(12550, "USD")).toBe("$125.50");
  });

  test("falls back to a raw string for non-finite input", () => {
    expect(formatAmountCents(Number.NaN, "USD")).toBe("NaN USD");
  });
});

describe("centsToDollarString", () => {
  test("converts a cents string to a two-decimal dollar string", () => {
    expect(centsToDollarString("12550")).toBe("125.50");
  });

  test("returns an empty string for invalid input", () => {
    expect(centsToDollarString("12.50")).toBe("");
    expect(centsToDollarString("")).toBe("");
  });
});

describe("formatRelativeTime", () => {
  test("reports recent timestamps as 'just now'", () => {
    const now = 1_000_000_000_000;
    expect(formatRelativeTime(new Date(now), now)).toBe("just now");
  });

  test("reports minutes for older timestamps", () => {
    const now = 1_000_000_000_000;
    expect(formatRelativeTime(new Date(now - 120_000), now)).toBe("2m ago");
  });
});
