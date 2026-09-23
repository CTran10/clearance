import { describe, expect, test } from "vitest";

import { normalizeApiBaseUrl } from "../../src/lib/storage.ts";

describe("normalizeApiBaseUrl", () => {
  test("falls back to the local Go service default for blank input", () => {
    expect(normalizeApiBaseUrl("")).toBe("http://127.0.0.1:8080");
  });

  test("trims whitespace and trailing slashes", () => {
    expect(normalizeApiBaseUrl(" http://localhost:9000/// ")).toBe("http://localhost:9000");
  });
});
