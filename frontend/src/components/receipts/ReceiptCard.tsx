import { formatAmountCents, formatDateTime, formatRelativeTime } from "../../lib/format.ts";
import { riskTone, statusTone } from "../../lib/receipts.ts";
import type { Receipt } from "../../types.ts";
import { StatusPill } from "../ui/StatusPill.tsx";

interface ReceiptCardProps {
  receipt: Receipt;
}

function Detail({ term, value, mono = false }: { term: string; value: string; mono?: boolean }) {
  return (
    <div className="receipt__detail">
      <dt>{term}</dt>
      <dd className={mono ? "mono" : undefined}>{value}</dd>
    </div>
  );
}

export function ReceiptCard({ receipt }: ReceiptCardProps) {
  return (
    <article className="receipt">
      <header className="receipt__head">
        <div className="receipt__id">
          <span className="receipt__idlabel">{receipt.kind === "DEPOSIT" ? "Deposit" : "Payment"}</span>
          <span className="receipt__idvalue mono">{receipt.transactionId || "pending"}</span>
        </div>
        <div className="receipt__pills">
          <StatusPill tone={statusTone(receipt.status)} label={receipt.status} />
          {receipt.kind === "PAYMENT" && <StatusPill
            tone={riskTone(receipt.riskLevel ?? receipt.previewRisk)}
            label={`${receipt.riskLevel ?? receipt.previewRisk} ${receipt.riskLevel ? "risk" : "risk preview"}`}
          />}
        </div>
      </header>

      <div className="receipt__amount mono">{formatAmountCents(receipt.amountCents, receipt.currency)}</div>

      <dl className="receipt__grid">
        <Detail term="Account" value={receipt.accountId} mono />
        {receipt.kind === "PAYMENT" ? <Detail term="Merchant" value={receipt.merchantId} mono /> :
          <Detail term="Balance after" value={formatAmountCents(receipt.balanceAfterCents, receipt.currency)} mono />}
        <Detail term="Idempotency" value={receipt.idempotencyKey} mono />
        <Detail term="Correlation" value={receipt.correlationId} mono />
      </dl>

      <footer className="receipt__foot">
        <span className="receipt__reason">
          {receipt.kind === "DEPOSIT" ? `Funding source: ${receipt.fundingSource}` :
            receipt.riskReason ? `Backend: ${receipt.riskReason}` : `Risk preview: ${receipt.previewReason}`}
          {receipt.statusError && ` · Status read: ${receipt.statusError}`}
        </span>
        <time dateTime={receipt.createdAt} title={formatDateTime(receipt.createdAt)}>
          {formatRelativeTime(receipt.createdAt)}
        </time>
      </footer>
    </article>
  );
}
