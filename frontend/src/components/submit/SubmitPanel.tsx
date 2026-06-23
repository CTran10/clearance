import { useState } from "react";
import type { FormEvent } from "react";

import { CURRENCIES, DEMO_ACCOUNTS, DEMO_MERCHANTS } from "../../lib/constants.ts";
import { centsToDollarString } from "../../lib/format.ts";
import type { SubmitFields } from "../../state/useConsole.ts";
import { Button } from "../ui/Button.tsx";
import { Panel } from "../ui/Panel.tsx";
import { SelectField, TextField } from "../ui/Field.tsx";
import { RiskPreview } from "./RiskPreview.tsx";
import "./submit.css";

interface SubmitPanelProps {
  idempotencyKey: string;
  correlationId: string;
  submitting: boolean;
  onSubmit: (fields: SubmitFields) => Promise<boolean>;
  onRegenerateKeys: () => void;
}

const DEFAULT_AMOUNT = "12550";

export function SubmitPanel({
  idempotencyKey,
  correlationId,
  submitting,
  onSubmit,
  onRegenerateKeys,
}: SubmitPanelProps) {
  const [amount, setAmount] = useState(DEFAULT_AMOUNT);
  const [currency, setCurrency] = useState("USD");

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await onSubmit({
      accountId: String(form.get("accountId") ?? ""),
      merchantId: String(form.get("merchantId") ?? ""),
      amountCents: amount,
      currency: String(form.get("currency") ?? ""),
      idempotencyKey: String(form.get("idempotencyKey") ?? ""),
      correlationId: String(form.get("correlationId") ?? ""),
    });
  }

  const dollars = centsToDollarString(amount);

  return (
    <Panel
      title="Submit transaction"
      index="01"
      actions={<span className="panel__meta">POST /transactions</span>}
    >
      <form className="submit" onSubmit={handleSubmit}>
        <RiskPreview amountCents={amount} currency={currency} />

        <fieldset className="submit__group">
          <legend className="submit__legend">Request body</legend>
          <div className="submit__grid">
            <SelectField label="Account" name="accountId" options={DEMO_ACCOUNTS} defaultValue="acct_123" />
            <SelectField
              label="Merchant"
              name="merchantId"
              options={DEMO_MERCHANTS}
              defaultValue="merchant_123"
            />
            <TextField
              label="Amount (cents)"
              name="amountCents"
              type="number"
              min="1"
              step="1"
              inputMode="numeric"
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
              mono
              hint={
                dollars
                  ? `= $${dollars}. Above $500.00 (50001) previews HIGH risk.`
                  : "Whole number of cents. Above 50001 previews HIGH risk."
              }
            />
            <SelectField
              label="Currency"
              name="currency"
              options={CURRENCIES}
              value={currency}
              onChange={(event) => setCurrency(event.target.value)}
            />
          </div>
        </fieldset>

        <fieldset className="submit__group">
          <legend className="submit__legend">
            Request headers
            <button type="button" className="link-button" onClick={onRegenerateKeys}>
              Regenerate
            </button>
          </legend>
          <TextField
            key={idempotencyKey}
            label="Idempotency-Key"
            name="idempotencyKey"
            defaultValue={idempotencyKey}
            mono
            spellCheck={false}
          />
          <TextField
            key={correlationId}
            label="X-Correlation-ID"
            name="correlationId"
            defaultValue={correlationId}
            mono
            spellCheck={false}
          />
        </fieldset>

        <div className="submit__actions">
          <Button type="submit" loading={submitting} block>
            {submitting ? "Submitting…" : "Submit transaction"}
          </Button>
        </div>
      </form>
    </Panel>
  );
}
