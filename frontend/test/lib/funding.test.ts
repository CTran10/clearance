import { expect, test } from "vitest";
import { buildDepositRequest } from "../../src/lib/funding.ts";

const input = { accountId: " acct-funding ", amountCents: "23000", currency: "usd",
  fundingSource: " manual ", externalReference: " ref-funding ", operatorReason: " Initial funding " };

test("funding validates editable input and sends only the deposit contract fields", () => {
  expect(buildDepositRequest(input)).toEqual({ accountId: "acct-funding", payload: {
    amount_cents: 23000, currency: "USD", funding_source: "manual",
    external_reference: "ref-funding", operator_reason: "Initial funding",
  } });
});

test.each(["0", "-1", "12.50", "1e3", "9007199254740992"])("rejects an invalid deposit amount: %s", (amountCents) => {
  expect(() => buildDepositRequest({ ...input, amountCents })).toThrow();
});

test.each(["", "a".repeat(257), "é".repeat(129), "Initial\tfunding"])("rejects an invalid audit reason", (operatorReason) => {
  expect(() => buildDepositRequest({ ...input, operatorReason })).toThrow(/operator reason/);
});

test("rejects unsafe funding identifiers and malformed currencies", () => {
  expect(() => buildDepositRequest({ ...input, accountId: "account/path" })).toThrow(/safe characters/);
  expect(() => buildDepositRequest({ ...input, fundingSource: "source path" })).toThrow(/safe characters/);
  expect(() => buildDepositRequest({ ...input, externalReference: "ref?query" })).toThrow(/safe characters/);
  expect(() => buildDepositRequest({ ...input, currency: "US" })).toThrow(/three-letter/);
});
