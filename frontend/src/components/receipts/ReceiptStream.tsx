import type { Receipt } from "../../types.ts";
import { Panel } from "../ui/Panel.tsx";
import { ReceiptCard } from "./ReceiptCard.tsx";
import "./receipts.css";

interface ReceiptStreamProps {
  receipts: Receipt[];
}

export function ReceiptStream({ receipts }: ReceiptStreamProps) {
  return (
    <Panel
      title="Local receipts"
      index="03"
      bodyClassName="receipts__body"
      actions={<span className="panel__meta mono">{receipts.length}/12 kept</span>}
    >
      {receipts.length === 0 ? (
        <div className="receipts__empty">
          <span className="receipts__emptymark" aria-hidden="true">
            ⌁
          </span>
          <p className="receipts__emptytitle">No receipts yet</p>
          <p className="receipts__emptyhint">
            Accepted PENDING responses appear here as a local trail. Nothing is sent anywhere but the
            Transaction Service.
          </p>
        </div>
      ) : (
        <div className="receipts__list">
          {receipts.map((receipt, index) => (
            <ReceiptCard
              key={`${receipt.transactionId}-${receipt.createdAt}`}
              receipt={receipt}
              highlight={index === 0}
            />
          ))}
        </div>
      )}
    </Panel>
  );
}
