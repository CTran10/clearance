import { DEFAULT_API_BASE_URL } from "./constants.ts";

const STORAGE_KEYS = {
  apiBaseUrl: "clearance.apiBaseUrl",
} as const;

export function normalizeApiBaseUrl(value: string | null | undefined): string {
  const trimmed = String(value || "").trim();
  if (!trimmed) {
    return DEFAULT_API_BASE_URL;
  }
  return trimmed.replace(/\/+$/, "");
}

/** Only the API base URL is persisted; the bearer value stays in memory. */
export function loadApiBaseUrl(): string {
  try {
    return normalizeApiBaseUrl(localStorage.getItem(STORAGE_KEYS.apiBaseUrl));
  } catch {
    return DEFAULT_API_BASE_URL;
  }
}

export function saveApiBaseUrl(value: string): string {
  const normalized = normalizeApiBaseUrl(value);
  try {
    localStorage.setItem(STORAGE_KEYS.apiBaseUrl, normalized);
  } catch {
    // Ignore storage failures (private mode, quota) — in-memory state still works.
  }
  return normalized;
}
