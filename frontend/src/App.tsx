import { summarizeReceipts } from "./lib/receipts.ts";
import { useConsole } from "./state/useConsole.ts";
import { MetricStrip } from "./components/shell/MetricStrip.tsx";
import { NoticeBar } from "./components/shell/NoticeBar.tsx";
import { SubmitPanel } from "./components/submit/SubmitPanel.tsx";
import { ReceiptStream } from "./components/receipts/ReceiptStream.tsx";
import { ConnectionSettings } from "./components/shell/ConnectionSettings.tsx";
import "./components/shell/shell.css";

export function App() {
  const console = useConsole();
  const { state, actions } = console;

  const summary = summarizeReceipts(state.receipts);

  return (
    <>
      <a className="skip-link" href="#console">
        Skip to console
      </a>
      <div className="app">
        <NoticeBar notice={state.notice} onDismiss={actions.dismissNotice} />
        <ConnectionSettings console={console} />

        <main id="console" className="grid" tabIndex={-1}>
          <div className="grid__col grid__col--primary">
            <SubmitPanel
              idempotencyKey={state.idempotencyKey}
              correlationId={state.correlationId}
              submitting={state.submitting}
              onSubmit={actions.submit}
              onDeposit={actions.deposit}
              onRegenerateKeys={actions.regenerateKeys}
            />
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
