/** Generates a URL/header-safe id that satisfies SAFE_TOKEN_PATTERN. */
// same client-side-key lesson from way back in the vanilla-js days, just grown up into TS now: the id is born HERE
// on the client so a retry can reuse it. prefixing with "idem"/"trace" is purely so i can eyeball logs and instantly
// know what each id is. the crypto.randomUUID-with-fallback dance is the same as always — real randomness if available
export function createSafeId(prefix: string): string {
  const cryptoApi = globalThis.crypto;
  if (cryptoApi && typeof cryptoApi.randomUUID === "function") {
    return `${prefix}-${cryptoApi.randomUUID()}`;
  }
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
