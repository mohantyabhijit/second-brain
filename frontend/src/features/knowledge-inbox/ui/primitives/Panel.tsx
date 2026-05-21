import type { ReactNode } from "react";
import type { IconName } from "../../presentation/viewModel";
import { Icon } from "./Icon";

type PanelProps = {
  id?: string;
  title: string;
  description: string;
  icon: IconName;
  className?: string;
  children: ReactNode;
};

export function Panel({ id, title, description, icon, className, children }: PanelProps) {
  return (
    <section id={id} className={`panel${className ? ` ${className}` : ""}`}>
      <div className="panel-header">
        <div>
          <h2>{title}</h2>
          <p>{description}</p>
        </div>
        <span className="panel-symbol">
          <Icon name={icon} />
        </span>
      </div>
      {children}
    </section>
  );
}
