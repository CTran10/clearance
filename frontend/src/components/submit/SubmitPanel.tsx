import { useState } from "react";
import type { FormEvent } from "react";

import { CURRENCIES } from "../../lib/constants.ts";
import { createSafeId } from "../../lib/ids.ts";
import { formatAmountCents } from "../../lib/format.ts";
import type { DepositFields, SubmissionResult, SubmitFields } from "../../state/useConsole.ts";
import { Button } from "../ui/Button.tsx";
import { Panel } from "../ui/Panel.tsx";
import { SelectField, TextField } from "../ui/Field.tsx";
import { RiskPreview } from "./RiskPreview.tsx";
import "./submit.css";

interface SubmitPanelProps {
  idempotencyKey: string;
  correlationId: string;
  submitting: boolean;
  onSubmit: (fields: SubmitFields) => Promise<SubmissionResult>;
  onDeposit: (fields: DepositFields) => Promise<SubmissionResult>;
  onRegenerateKeys: () => void;
}

const DEFAULT_AMOUNT = "12550";

export function SubmitPanel({
  idempotencyKey,
  correlationId,
  submitting,
  onSubmit,
  onDeposit,
  onRegenerateKeys,
}: SubmitPanelProps) {
  const [amount, setAmount] = useState(DEFAULT_AMOUNT);
  const [currency, setCurrency] = useState("USD");
  const [kind, setKind] = useState<"PAYMENT" | "DEPOSIT">("PAYMENT");
  const [accountId, setAccountId] = useState(() => createSafeId("acct"));
  const [externalReference, setExternalReference] = useState(() => createSafeId("ref"));
  const [merchantId, setMerchantId] = useState("merchant_123");
  const [fundingSource, setFundingSource] = useState("manual");
  const [operatorReason, setOperatorReason] = useState("Account funding");

  function newRequest() {
    onRegenerateKeys();
    setExternalReference(createSafeId("ref"));
  }

  function chooseKind(next: typeof kind) {
    if (next !== kind) {
      setKind(next);
      newRequest();
    }
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const common = { accountId, amountCents: amount, currency,
      idempotencyKey: String(form.get("idempotencyKey") ?? ""),
      correlationId: String(form.get("correlationId") ?? ""),
    };
    if (kind === "PAYMENT") {
      await onSubmit({ ...common, merchantId });
    } else {
      await onDeposit({ ...common, fundingSource,
        externalReference: String(form.get("externalReference") ?? ""),
        operatorReason,
      });
    }
  }

  return (
    <Panel
      title="Transaction"
      actions={<button type="button" className="link-button" disabled={submitting} onClick={newRequest}>New request</button>}
    >
      <form className="submit" onSubmit={handleSubmit}>
        <div className="submit__kind" role="group" aria-label="Request type">
          <button type="button" aria-pressed={kind === "PAYMENT"} disabled={submitting} onClick={() => chooseKind("PAYMENT")}>Payment</button>
          <button type="button" aria-pressed={kind === "DEPOSIT"} disabled={submitting} onClick={() => chooseKind("DEPOSIT")}>Deposit</button>
        </div>
        {kind === "PAYMENT" ? <RiskPreview amountCents={amount} currency={currency} /> :
          <div className="riskpreview">
            <div className="riskpreview__main">
              <span className="riskpreview__eyebrow">Deposit</span>
              <div className="riskpreview__amount mono">{formatAmountCents(Number(amount) || 0, currency)}</div>
            </div>
          </div>}

        <fieldset className="submit__group" disabled={submitting}>
          <div className="submit__grid">
            <TextField label="Account" name="accountId" value={accountId} required maxLength={128} mono
              onChange={(event) => setAccountId(event.target.value)}
              aside={<button type="button" className="link-button" onClick={() => { setAccountId(createSafeId("acct")); newRequest(); }}>Generate ID</button>} />
            {kind === "PAYMENT" ?
              <TextField label="Merchant" name="merchantId" value={merchantId} onChange={(event) => setMerchantId(event.target.value)} required maxLength={128} mono /> :
              <TextField label="Funding source" name="fundingSource" value={fundingSource} onChange={(event) => setFundingSource(event.target.value)} required maxLength={128} mono />}
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
              required
            />
            <SelectField
              label="Currency"
              name="currency"
              options={CURRENCIES}
              value={currency}
              onChange={(event) => setCurrency(event.target.value)}
            />
          </div>
          {kind === "DEPOSIT" && <TextField label="Operator reason" name="operatorReason" value={operatorReason} onChange={(event) => setOperatorReason(event.target.value)} required maxLength={256} />}
        </fieldset>

        <details className="submit__details">
          <summary>Request details</summary>
          <fieldset className="submit__group" disabled={submitting}>
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
            {kind === "DEPOSIT" && <TextField key={externalReference} label="External reference" name="externalReference" defaultValue={externalReference} mono />}
          </fieldset>
        </details>

        <div className="submit__actions">
          <Button type="submit" loading={submitting} block>
            {submitting ? "Submitting…" : kind === "PAYMENT" ? "Submit payment" : "Deposit funds"}
          </Button>
        </div>
      </form>
    </Panel>
  );
}
