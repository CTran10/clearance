import { useState } from "react";
import type { FormEvent } from "react";

import { Button } from "../ui/Button.tsx";
import { TextField } from "../ui/Field.tsx";

interface ConnectionBarProps {
  apiBaseUrl: string;
  authValue: string;
  onSave: (apiBaseUrl: string, authValue: string) => void;
}

export function ConnectionBar({ apiBaseUrl, authValue, onSave }: ConnectionBarProps) {
  const [showBearer, setShowBearer] = useState(false);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    onSave(String(form.get("apiBaseUrl") ?? ""), String(form.get("authValue") ?? ""));
  }

  return (
    <form id="connection-bar" className="connection" onSubmit={handleSubmit}>
      <TextField
        label="API base URL"
        name="apiBaseUrl"
        type="url"
        defaultValue={apiBaseUrl}
        autoComplete="url"
        spellCheck={false}
        mono
        hint="Where the Transaction Service is reachable."
      />
      <TextField
        label="Bearer value"
        name="authValue"
        type={showBearer ? "text" : "password"}
        defaultValue={authValue}
        autoComplete="off"
        spellCheck={false}
        mono
        hint="Held in memory only — never written to storage."
        aside={
          <button type="button" className="link-button" onClick={() => setShowBearer((v) => !v)}>
            {showBearer ? "Hide" : "Show"}
          </button>
        }
      />
      <div className="connection__action">
        <Button type="submit" variant="secondary" block>
          Save connection
        </Button>
      </div>
    </form>
  );
}
