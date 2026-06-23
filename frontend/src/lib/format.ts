/**
 * Money is handled as integer cents end to end to avoid floating-point drift.
 * These helpers only format for display or translate the operator's dollar
 * input into the integer cents the API expects.
 */

export function formatAmountCents(amountCents: number, currency: string): string {
  const cents = Number(amountCents);
  if (!Number.isFinite(cents)) {
    return `${amountCents} ${currency}`;
  }
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: currency || "USD",
  }).format(cents / 100);
}

export function formatDateTime(value: string | number | Date | null | undefined): string {
  if (!value) {
    return "Not available";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

export function formatRelativeTime(value: string | number | Date, now: number = Date.now()): string {
  const then = new Date(value).getTime();
  if (Number.isNaN(then)) {
    return "";
  }
  const seconds = Math.round((now - then) / 1000);
  if (seconds < 5) return "just now";
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  return `${hours}h ago`;
}

/** "12550" cents -> "125.50" for a read-only echo under the amount field. */
export function centsToDollarString(amountCents: string): string {
  const trimmed = String(amountCents ?? "").trim();
  if (!/^\d+$/.test(trimmed)) {
    return "";
  }
  const cents = Number(trimmed);
  if (!Number.isSafeInteger(cents)) {
    return "";
  }
  return (cents / 100).toFixed(2);
}
