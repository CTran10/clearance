import type { Tone } from "../../types.ts";

interface StatusPillProps {
  tone: Tone;
  label: string;
}

export function StatusPill({ tone, label }: StatusPillProps) {
  return (
    <span className="pill" data-tone={tone}>
      <span className="pill__dot" aria-hidden="true" />
      {label}
    </span>
  );
}
