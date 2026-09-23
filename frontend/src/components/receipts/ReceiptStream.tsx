import type { Receipt } from "../../types.ts";
import { Panel } from "../ui/Panel.tsx";
import { ReceiptCard } from "./ReceiptCard.tsx";
import "./receipts.css";

interface ReceiptStreamProps {
  receipts: Receipt[];
}

export function ReceiptStream({ receipts }: ReceiptStreamProps) {
  return (
    <Panel title="Receipts" bodyClassName="receipts__body">
      {receipts.length === 0 ? (
        <div className="receipts__empty">
          <p className="receipts__emptytitle">No receipts yet</p>
        </div>
      ) : (
        <div className="receipts__list">
          {receipts.map((receipt) => (
            <ReceiptCard
              key={`${receipt.transactionId}-${receipt.createdAt}`}
              receipt={receipt}
            />
          ))}
        </div>
      )}
    </Panel>
  );
}
