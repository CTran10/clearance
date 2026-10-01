import { buildTransactionHeaders } from "./transaction.ts";
import type { TransactionDetail, TransactionPayload, TransactionResponse } from "../types.ts";

export async function parseApiError(response: Response): Promise<string> {
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    return `Request failed with ${response.status}`;
  }

  if (body && typeof body === "object") {
    const record = body as Record<string, unknown>;
    if (typeof record.error === "string") {
      return record.error;
    }
    if (typeof record.detail === "string") {
      return record.detail;
    }
  }
  return `Request failed with ${response.status}`;
}

async function request<T>(baseUrl: string, path: string, options: RequestInit = {}): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${baseUrl}${path}`, options);
  } catch (cause) {
    const reason = cause instanceof Error ? cause.message : "network error";
    throw new Error(`Could not reach ${baseUrl}${path} (${reason}). Is the platform running?`);
  }
  if (!response.ok) {
    throw new Error(await parseApiError(response));
  }
  return response.json() as Promise<T>;
}

export interface SubmitArgs {
  baseUrl: string;
  authValue: string;
  payload: TransactionPayload;
  idempotencyKey: string;
  correlationId: string;
}

export async function submitTransaction({
  baseUrl,
  authValue,
  payload,
  idempotencyKey,
  correlationId,
}: SubmitArgs): Promise<TransactionResponse> {
  return request<TransactionResponse>(baseUrl, "/transactions", {
    method: "POST",
    headers: buildTransactionHeaders(authValue, idempotencyKey, correlationId),
    body: JSON.stringify(payload),
  });
}

export interface GetTransactionArgs {
  baseUrl: string;
  authValue: string;
  transactionId: string;
}

export async function getTransaction({
  baseUrl,
  authValue,
  transactionId,
}: GetTransactionArgs): Promise<TransactionDetail> {
  return request<TransactionDetail>(baseUrl, `/transactions/${encodeURIComponent(transactionId)}`, {
    method: "GET",
    headers: { Authorization: `Bearer ${authValue}` },
  });
}
