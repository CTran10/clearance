import { useId } from "react";
import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from "react";

import type { Option } from "../../lib/constants.ts";

interface FieldShellProps {
  label: string;
  hint?: ReactNode;
  /** Rendered at the right edge of the label row, e.g. a live badge. */
  aside?: ReactNode;
  htmlFor: string;
  children: ReactNode;
}

function FieldShell({ label, hint, aside, htmlFor, children }: FieldShellProps) {
  return (
    <div className="field">
      <label className="field__label" htmlFor={htmlFor}>
        <span>{label}</span>
        {aside}
      </label>
      {children}
      {hint && <small className="field__hint">{hint}</small>}
    </div>
  );
}

interface TextFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  hint?: ReactNode;
  aside?: ReactNode;
  mono?: boolean;
}

export function TextField({ label, hint, aside, mono = false, className, id, ...rest }: TextFieldProps) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  const classes = ["input", mono ? "input--mono" : "", className ?? ""].filter(Boolean).join(" ");
  return (
    <FieldShell label={label} hint={hint} aside={aside} htmlFor={fieldId}>
      <input id={fieldId} className={classes} {...rest} />
    </FieldShell>
  );
}

interface SelectFieldProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label: string;
  hint?: ReactNode;
  aside?: ReactNode;
  options: Option[];
}

export function SelectField({ label, hint, aside, options, id, ...rest }: SelectFieldProps) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  return (
    <FieldShell label={label} hint={hint} aside={aside} htmlFor={fieldId}>
      <select id={fieldId} className="select" {...rest}>
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.hint ? `${option.label} — ${option.hint}` : option.label}
          </option>
        ))}
      </select>
    </FieldShell>
  );
}
