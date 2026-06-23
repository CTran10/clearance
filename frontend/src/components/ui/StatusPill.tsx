import type { Tone } from "../../types.ts";

interface StatusPillProps {
  tone: Tone;
  label: string;
  /** Pulsing dot for in-flight / live states. */
  live?: boolean;
}

export function StatusPill({ tone, label, live = false }: StatusPillProps) {
  return (
    <span className="pill" data-tone={tone}>
      <span className={live ? "pill__dot pill__dot--live" : "pill__dot"} aria-hidden="true" />
      {label}
    </span>
  );
}
