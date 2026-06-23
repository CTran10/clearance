import { useMemo, useState } from "react";

import { summarizeReceipts } from "./lib/receipts.ts";
import { useConsole } from "./state/useConsole.ts";
import { ConnectionBar } from "./components/shell/ConnectionBar.tsx";
import { MetricStrip } from "./components/shell/MetricStrip.tsx";
import { NoticeBar } from "./components/shell/NoticeBar.tsx";
import { TopBar } from "./components/shell/TopBar.tsx";
import { SubmitPanel } from "./components/submit/SubmitPanel.tsx";
import { PipelineFlow } from "./components/flow/PipelineFlow.tsx";
import { ReceiptStream } from "./components/receipts/ReceiptStream.tsx";
import "./components/shell/shell.css";

export function App() {
  const { state, actions } = useConsole();
  const [settingsOpen, setSettingsOpen] = useState(false);

  const summary = useMemo(() => summarizeReceipts(state.receipts), [state.receipts]);
  const latest = state.receipts[0] ?? null;

  return (
    <>
      <a className="skip-link" href="#console">
        Skip to console
      </a>
      <div className="app">
        <TopBar
          health={state.health}
          onCheckHealth={actions.runHealthCheck}
          onToggleSettings={() => setSettingsOpen((open) => !open)}
          settingsOpen={settingsOpen}
        />

        {settingsOpen && (
          <ConnectionBar
            apiBaseUrl={state.apiBaseUrl}
            authValue={state.authValue}
            onSave={(url, auth) => {
              actions.saveSettings(url, auth);
              setSettingsOpen(false);
            }}
          />
        )}

        <NoticeBar notice={state.notice} onDismiss={actions.dismissNotice} />

        <main id="console" className="grid">
          <div className="grid__col grid__col--primary">
            <SubmitPanel
              idempotencyKey={state.idempotencyKey}
              correlationId={state.correlationId}
              submitting={state.submitting}
              onSubmit={actions.submit}
              onRegenerateKeys={actions.regenerateKeys}
            />
            <PipelineFlow latest={latest} />
          </div>

          <div className="grid__col grid__col--secondary">
            <MetricStrip summary={summary} />
            <ReceiptStream receipts={state.receipts} />
          </div>
        </main>
      </div>
    </>
  );
}
