import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { App } from "../src/App.tsx";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.unstubAllEnvs(); });

function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function detail(id: string, status: string, reason: string, risk: string) {
  return { transaction_id: id, kind: "PAYMENT", status, risk_reason: reason, risk_level: risk };
}

function configure(transactionBearer = "transaction-test") {
  fireEvent.click(screen.getByText("Connection", { selector: "summary" }));
  fireEvent.change(screen.getByLabelText("API URL"), { target: { value: "http://localhost:9000" } });
  fireEvent.change(screen.getByLabelText("Transaction bearer"), { target: { value: transactionBearer } });
  fireEvent.change(screen.getByLabelText("Funding bearer"), { target: { value: "funding-test" } });
  fireEvent.click(screen.getByRole("button", { name: "Save connection" }));
}

function start(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubEnv("VITE_TRANSACTION_API_AUTH_VALUE", "");
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
}

function amount(value: string) {
  fireEvent.change(screen.getByLabelText("Amount (cents)"), { target: { value } });
}

function depositResponse(url: string, options: RequestInit, accountOverride?: string) {
  const body = JSON.parse(String(options.body));
  return response({ transaction_id: "deposit-1", deposit_id: "deposit-1", status: "AUTHORIZED",
    account_id: accountOverride ?? decodeURIComponent(url.split("/").at(-2) ?? ""), amount_cents: body.amount_cents,
    currency: body.currency, balance_after_cents: body.amount_cents, correlation_id: new Headers(options.headers).get("X-Correlation-ID") }, 201);
}

test("ordinary forms support funding, a payment, replay, conflict, and both business rejections", async () => {
  const fetchMock = vi.fn()
    .mockImplementationOnce(async (url: string, options: RequestInit) => depositResponse(url, options))
    .mockResolvedValueOnce(response({ transaction_id: "txn-first", status: "PENDING" }, 202))
    .mockResolvedValueOnce(response(detail("txn-first", "AUTHORIZED", "amount is at or below 500.00", "LOW")))
    .mockResolvedValueOnce(response({ transaction_id: "txn-first", status: "PENDING" }, 202))
    .mockResolvedValueOnce(response({ error: "idempotency conflict" }, 409))
    .mockResolvedValueOnce(response({ transaction_id: "txn-risk", status: "PENDING" }, 202))
    .mockResolvedValueOnce(response(detail("txn-risk", "FAILED", "amount is greater than 500.00", "HIGH")))
    .mockResolvedValueOnce(response({ transaction_id: "txn-funds", status: "PENDING" }, 202))
    .mockResolvedValueOnce(response(detail("txn-funds", "FAILED", "insufficient funds", "LOW")));
  start(fetchMock);
  expect(screen.getByRole("heading", { name: "Transaction" })).toBeInTheDocument();
  expect(screen.queryByText("Guided demo")).not.toBeInTheDocument();
  expect(fetchMock).not.toHaveBeenCalled();
  configure();
  fireEvent.change(screen.getByLabelText("Account"), { target: { value: "acct-present" } });
  fireEvent.click(screen.getByRole("button", { name: "Deposit" }));
  amount("50000");
  fireEvent.click(screen.getByRole("button", { name: "Deposit funds" }));
  await screen.findByText("deposit-1", { selector: ".receipt__idvalue" });
  expect(screen.getByText("$500.00", { selector: ".receipt__detail dd" })).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "Payment" }));
  fireEvent.change(screen.getByLabelText("Merchant"), { target: { value: "merchant_custom" } });
  amount("12550");
  fireEvent.click(screen.getByRole("button", { name: "Submit payment" }));
  await screen.findByText(/Backend: amount is at or below/);
  expect(screen.getByText("txn-first", { selector: ".receipt__idvalue" }).closest("article")).toHaveTextContent("AUTHORIZED");
  fireEvent.click(screen.getByRole("button", { name: "Submit payment" }));
  await screen.findByText(/Existing payment/);
  expect(screen.getAllByText("txn-first", { selector: ".receipt__idvalue" })).toHaveLength(1);
  amount("12551");
  fireEvent.click(screen.getByRole("button", { name: "Submit payment" }));
  await screen.findByText("HTTP 409: idempotency conflict");

  fireEvent.click(screen.getByRole("button", { name: "New request" }));
  amount("50001");
  fireEvent.click(screen.getByRole("button", { name: "Submit payment" }));
  await screen.findByText(/Backend: amount is greater than/);
  fireEvent.click(screen.getByRole("button", { name: "New request" }));
  amount("40000");
  fireEvent.click(screen.getByRole("button", { name: "Submit payment" }));
  await screen.findByText("Backend: insufficient funds");
  expect(screen.getAllByRole("article")).toHaveLength(4);

  const calls = fetchMock.mock.calls;
  expect(calls).toHaveLength(9);
  expect(calls[0][1].headers).toMatchObject({ Authorization: "Bearer funding-test" });
  expect(JSON.parse(String(calls[0][1].body))).toMatchObject({ funding_source: "manual", amount_cents: 50000 });
  expect(calls[1][1].headers).toMatchObject({ Authorization: "Bearer transaction-test" });
  expect(calls[3][1].body).toBe(calls[1][1].body);
  expect(calls[3][1].headers).toEqual(calls[1][1].headers);
  const originalKey = new Headers(calls[1][1].headers).get("Idempotency-Key");
  expect(new Headers(calls[4][1].headers).get("Idempotency-Key")).toBe(originalKey);
  expect(new Headers(calls[5][1].headers).get("Idempotency-Key")).not.toBe(originalKey);
  expect(JSON.parse(String(calls[1][1].body))).toMatchObject({ account_id: "acct-present", merchant_id: "merchant_custom" });
});

test("funding supports arbitrary amounts with only its scoped bearer and no risk score", async () => {
  const fetchMock = vi.fn().mockImplementation(async (url: string, options: RequestInit) => depositResponse(url, options));
  start(fetchMock); configure("");
  fireEvent.click(screen.getByRole("button", { name: "Deposit" }));
  amount("23000");
  fireEvent.click(screen.getByRole("button", { name: "Deposit funds" }));
  await screen.findByText("deposit-1", { selector: ".receipt__idvalue" });
  expect(screen.getByText("$230.00", { selector: ".receipt__detail dd" })).toBeInTheDocument();
  expect(screen.queryByText(/LOW risk/)).not.toBeInTheDocument();
  expect(fetchMock).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("button", { name: "Deposit funds" }));
  await screen.findByText(/Existing deposit/);
  expect(screen.getAllByRole("article")).toHaveLength(1);
  expect(fetchMock.mock.calls[1][1]).toEqual(fetchMock.mock.calls[0][1]);
});

test("a mismatched funding response cannot add an authorized receipt", async () => {
  const fetchMock = vi.fn().mockImplementation(async (url: string, options: RequestInit) => depositResponse(url, options, "other-account"));
  start(fetchMock); configure();
  fireEvent.click(screen.getByRole("button", { name: "Deposit" }));
  amount("50000");
  fireEvent.click(screen.getByRole("button", { name: "Deposit funds" }));
  await screen.findByText("The API returned an invalid deposit response.");
  expect(screen.queryByRole("article")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Deposit funds" })).toBeEnabled();
});

test("generating an account ID starts a fresh request without writing to the backend", () => {
  const fetchMock = vi.fn(); start(fetchMock);
  const account = (screen.getByLabelText("Account") as HTMLInputElement).value;
  const key = (screen.getByLabelText("Idempotency-Key") as HTMLInputElement).value;
  fireEvent.click(screen.getByRole("button", { name: "Generate ID" }));
  expect(screen.getByLabelText("Account")).not.toHaveValue(account);
  expect(screen.getByLabelText("Idempotency-Key")).not.toHaveValue(key);
  expect(fetchMock).not.toHaveBeenCalled();
});

test("payment and deposit fields remain distinct when switching operations", () => {
  const fetchMock = vi.fn(); start(fetchMock);
  fireEvent.change(screen.getByLabelText("Merchant"), { target: { value: "merchant-custom" } });
  fireEvent.click(screen.getByRole("button", { name: "Deposit" }));
  expect(screen.getByLabelText("Funding source")).toHaveValue("manual");
  fireEvent.change(screen.getByLabelText("Funding source"), { target: { value: "bank-transfer" } });
  fireEvent.click(screen.getByRole("button", { name: "Payment" }));
  expect(screen.getByLabelText("Merchant")).toHaveValue("merchant-custom");
  fireEvent.click(screen.getByRole("button", { name: "Deposit" }));
  expect(screen.getByLabelText("Funding source")).toHaveValue("bank-transfer");
  expect(fetchMock).not.toHaveBeenCalled();
});
