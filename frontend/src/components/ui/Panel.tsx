import type { ReactNode } from "react";

interface PanelProps {
  title: string;
  /** Small monospace index shown before the title, e.g. "02". */
  index?: string;
  actions?: ReactNode;
  bodyClassName?: string;
  children: ReactNode;
  id?: string;
}

export function Panel({ title, index, actions, bodyClassName, children, id }: PanelProps) {
  return (
    <section className="panel" id={id}>
      <div className="panel__head">
        <h2 className="panel__title">
          {index && <span className="panel__index">{index}</span>}
          {title}
        </h2>
        {actions}
      </div>
      <div className={bodyClassName ? `panel__body ${bodyClassName}` : "panel__body"}>{children}</div>
    </section>
  );
}
