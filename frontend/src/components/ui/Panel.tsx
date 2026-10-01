import type { ReactNode } from "react";

interface PanelProps {
  title: string;
  actions?: ReactNode;
  bodyClassName?: string;
  children: ReactNode;
}

export function Panel({ title, actions, bodyClassName, children }: PanelProps) {
  return (
    <section className="panel">
      <div className="panel__head">
        <h2 className="panel__title">{title}</h2>
        {actions}
      </div>
      <div className={bodyClassName ? `panel__body ${bodyClassName}` : "panel__body"}>{children}</div>
    </section>
  );
}
