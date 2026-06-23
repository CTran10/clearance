import type { Receipt, ReceiptSummary, Tone, TransactionStatus, RiskLevel } from "../types.ts";

export function summarizeReceipts(receipts: Pick<Receipt, "status" | "previewRisk">[]): ReceiptSummary {
  return receipts.reduce<ReceiptSummary>(
    (summary, receipt) => {
      summary.total += 1;
      if (receipt.status === "PENDING") {
        summary.pending += 1;
      }
      if (receipt.previewRisk === "LOW") {
        summary.lowRisk += 1;
      }
      if (receipt.previewRisk === "HIGH") {
        summary.highRisk += 1;
      }
      return summary;
    },
    { total: 0, pending: 0, lowRisk: 0, highRisk: 0 },
  );
}

/** Maps a platform status to a semantic visual tone. */
export function statusTone(status: TransactionStatus | string): Tone {
  switch (status) {
    case "AUTHORIZED":
      return "positive";
    case "PENDING":
      return "pending";
    case "FAILED":
      return "negative";
    default:
      return "neutral";
  }
}

export function riskTone(level: RiskLevel | string): Tone {
  switch (level) {
    case "LOW":
      return "positive";
    case "HIGH":
      return "negative";
    default:
      return "neutral";
  }
}
