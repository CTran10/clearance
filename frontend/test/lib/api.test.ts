import { describe, expect, test } from "vitest";

import { parseApiError } from "../../src/lib/api.ts";
import { normalizeApiBaseUrl } from "../../src/lib/storage.ts";

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
