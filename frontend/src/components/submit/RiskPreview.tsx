import { centsToDollarString, formatAmountCents } from "../../lib/format.ts";
import { riskPreview } from "../../lib/risk.ts";
import { riskTone } from "../../lib/receipts.ts";
import { StatusPill } from "../ui/StatusPill.tsx";

interface RiskPreviewProps {
  amountCents: string;
  currency: string;
}

export function RiskPreview({ amountCents, currency }: RiskPreviewProps) {
  const dollars = centsToDollarString(amountCents);
  const valid = dollars !== "" && Number(amountCents) > 0;

  if (!valid) {
    return (
      <div className="riskpreview riskpreview--empty">
        <div className="riskpreview__amount mono">$ —</div>
        <p className="riskpreview__hint">Enter an amount in cents to preview the Risk Service decision.</p>
      </div>
    );
  }

  const cents = Number(amountCents);
  const preview = riskPreview(cents);
  const tone = riskTone(preview.level);

  return (
    <div className="riskpreview" data-tone={tone}>
      <div className="riskpreview__main">
        <span className="riskpreview__eyebrow">Submitting</span>
        <div className="riskpreview__amount mono">{formatAmountCents(cents, currency)}</div>
      </div>
      <div className="riskpreview__verdict">
        <StatusPill tone={tone} label={`${preview.level} risk`} />
        <span className="riskpreview__outcome">{preview.outcome}</span>
        <span className="riskpreview__reason">{preview.reason}</span>
      </div>
    </div>
  );
}
