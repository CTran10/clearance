import type { ReceiptSummary } from "../../types.ts";

interface MetricStripProps {
  summary: ReceiptSummary;
}

function Stat({ label, value, tone }: { label: string; value: number; tone?: string }) {
  return (
    <div className="metric">
      <span className="metric__value mono" data-tone={tone}>
        {value}
      </span>
      <span className="metric__label">{label}</span>
    </div>
  );
}

export function MetricStrip({ summary }: MetricStripProps) {
  const scored = summary.lowRisk + summary.highRisk;
  const lowPct = scored === 0 ? 0 : Math.round((summary.lowRisk / scored) * 100);

  return (
    <div className="metricstrip" role="group" aria-label="Session totals">
      <Stat label="Receipts" value={summary.total} />
      <span className="metric__rule" aria-hidden="true" />
      <Stat label="Pending" value={summary.pending} tone="pending" />
      <span className="metric__rule" aria-hidden="true" />
      <Stat label="LOW" value={summary.lowRisk} tone="positive" />
      <span className="metric__rule" aria-hidden="true" />
      <Stat label="HIGH" value={summary.highRisk} tone="negative" />
      {scored > 0 && (
        <div className="riskmix" title={`${lowPct}% previewed LOW risk`}>
          <div className="riskmix__bar">
            <span className="riskmix__low" style={{ width: `${lowPct}%` }} />
          </div>
          <span className="riskmix__caption mono">{lowPct}% LOW</span>
        </div>
      )}
    </div>
  );
}
