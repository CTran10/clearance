import { Button } from "../ui/Button.tsx";
import { StatusPill } from "../ui/StatusPill.tsx";
import type { HealthState, Tone } from "../../types.ts";

interface TopBarProps {
  health: HealthState;
  onCheckHealth: () => void;
  onToggleSettings: () => void;
  settingsOpen: boolean;
}

const HEALTH_LABEL: Record<HealthState, { tone: Tone; label: string; live?: boolean }> = {
  unknown: { tone: "neutral", label: "Not checked" },
  checking: { tone: "pending", label: "Checking", live: true },
  ok: { tone: "positive", label: "Healthy" },
  down: { tone: "negative", label: "Unreachable" },
};

export function TopBar({ health, onCheckHealth, onToggleSettings, settingsOpen }: TopBarProps) {
  const status = HEALTH_LABEL[health];

  return (
    <header className="topbar">
      <div className="topbar__brand">
        <span className="brandmark" aria-hidden="true">
          <svg viewBox="0 0 32 32" width="22" height="22">
            <rect width="32" height="32" rx="8" fill="currentColor" />
            <path
              d="M8 16.5l5 5 11-11"
              stroke="var(--accent-ink)"
              strokeWidth="3.4"
              strokeLinecap="round"
              strokeLinejoin="round"
              fill="none"
            />
          </svg>
        </span>
        <div className="topbar__titles">
          <p className="eyebrow">Clearance Platform</p>
          <h1 className="topbar__title">Authorization Console</h1>
        </div>
      </div>

      <div className="topbar__status">
        <span className="topbar__healthlabel">Transaction API</span>
        <StatusPill tone={status.tone} label={status.label} live={status.live} />
        <Button variant="secondary" onClick={onCheckHealth} loading={health === "checking"}>
          Check health
        </Button>
        <Button
          variant="ghost"
          onClick={onToggleSettings}
          aria-expanded={settingsOpen}
          aria-controls="connection-bar"
        >
          {settingsOpen ? "Hide connection" : "Connection"}
        </Button>
      </div>
    </header>
  );
}
