import { useId } from "react";
import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from "react";

import type { Option } from "../../lib/constants.ts";

interface FieldShellProps {
  label: string;
  aside?: ReactNode;
  htmlFor: string;
  children: ReactNode;
}

function FieldShell({ label, aside, htmlFor, children }: FieldShellProps) {
  return (
    <div className="field">
      <div className="field__label">
        <label htmlFor={htmlFor}>{label}</label>
        {aside}
      </div>
      {children}
    </div>
  );
}

interface TextFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  aside?: ReactNode;
  mono?: boolean;
}

export function TextField({ label, aside, mono = false, className, id, ...rest }: TextFieldProps) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  const classes = ["input", mono ? "input--mono" : "", className ?? ""].filter(Boolean).join(" ");
  return (
    <FieldShell label={label} aside={aside} htmlFor={fieldId}>
      <input id={fieldId} className={classes} {...rest} />
    </FieldShell>
  );
}

interface SelectFieldProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label: string;
  aside?: ReactNode;
  options: Option[];
}

export function SelectField({ label, aside, options, id, ...rest }: SelectFieldProps) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  return (
    <FieldShell label={label} aside={aside} htmlFor={fieldId}>
      <select id={fieldId} className="select" {...rest}>
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.hint ? `${option.value} — ${option.hint}` : option.value}
          </option>
        ))}
      </select>
    </FieldShell>
  );
}
