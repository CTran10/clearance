import type { Notice } from "../../state/useConsole.ts";

interface NoticeBarProps {
  notice: Notice | null;
  onDismiss: () => void;
}

export function NoticeBar({ notice, onDismiss }: NoticeBarProps) {
  if (!notice) {
    return (
      <div className="notice notice--idle" role="status">
        <span className="notice__dot" aria-hidden="true" />
        Set the connection, then submit a LOW or HIGH risk transaction to trace the event flow.
      </div>
    );
  }

  return (
    <div className="notice" data-tone={notice.tone} role={notice.tone === "negative" ? "alert" : "status"}>
      <span className="notice__dot" aria-hidden="true" />
      <span className="notice__text">{notice.text}</span>
      <button type="button" className="notice__close" onClick={onDismiss} aria-label="Dismiss message">
        ×
      </button>
    </div>
  );
}
