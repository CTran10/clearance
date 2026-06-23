import { PIPELINE_STAGES } from "../../lib/constants.ts";
import { riskTone, statusTone } from "../../lib/receipts.ts";
import type { Receipt } from "../../types.ts";
import { Panel } from "../ui/Panel.tsx";
import { StatusPill } from "../ui/StatusPill.tsx";
import "./flow.css";

interface PipelineFlowProps {
  latest: Receipt | null;
}

function stageAnnotation(stageId: string, latest: Receipt) {
  switch (stageId) {
    case "transaction":
      return <StatusPill tone={statusTone(latest.status)} label={latest.status} />;
    case "risk":
      return <StatusPill tone={riskTone(latest.previewRisk)} label={`${latest.previewRisk} predicted`} />;
    case "ledger":
      return (
        <span className="flow__hint">
          {latest.previewRisk === "LOW" ? "Would post ledger entries" : "Would halt — likely failed"}
        </span>
      );
    default:
      return null;
  }
}

export function PipelineFlow({ latest }: PipelineFlowProps) {
  return (
    <Panel
      title="Event pipeline"
      index="02"
      actions={
        latest ? (
          <span className="panel__meta mono">trace {latest.correlationId.slice(0, 12)}…</span>
        ) : (
          <span className="panel__meta">async via Redpanda</span>
        )
      }
    >
      <ol className={latest ? "flow flow--active" : "flow"}>
        {PIPELINE_STAGES.map((stage, index) => (
          <li className="flow__step" key={stage.id}>
            <div className="flow__marker" aria-hidden="true">
              <span className="flow__node">{index + 1}</span>
            </div>
            <div className="flow__content">
              <div className="flow__heading">
                <h3 className="flow__service">{stage.service}</h3>
                {latest && stageAnnotation(stage.id, latest)}
              </div>
              <p className="flow__detail">{stage.detail}</p>
            </div>
          </li>
        ))}
      </ol>
    </Panel>
  );
}
