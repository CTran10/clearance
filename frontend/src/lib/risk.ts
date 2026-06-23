import { RISK_THRESHOLD_CENTS } from "./constants.ts";
import type { RiskPreview } from "../types.ts";

const thresholdDollars = (RISK_THRESHOLD_CENTS / 100).toFixed(2);

/**
 * Mirrors the Risk Service decision so operators see the likely outcome before
 * the event round-trips. Amounts strictly above the threshold are HIGH risk.
 */
export function riskPreview(amountCents: number): RiskPreview {
  if (Number(amountCents) > RISK_THRESHOLD_CENTS) {
    return {
      level: "HIGH",
      outcome: "likely failed",
      reason: `Amount is greater than ${thresholdDollars}.`,
    };
  }
  return {
    level: "LOW",
    outcome: "likely authorized",
    reason: `Amount is at or below ${thresholdDollars}.`,
  };
}
