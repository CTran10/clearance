import { useState } from "react";
import type { FormEvent } from "react";
import type { useConsole } from "../../state/useConsole.ts";
import { Button } from "../ui/Button.tsx";
import { TextField } from "../ui/Field.tsx";

export function ConnectionSettings({ console }: { console: ReturnType<typeof useConsole> }) {
  const [error, setError] = useState<string | null>(null);

  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const fields = new FormData(event.currentTarget);
    try {
      console.actions.configureConnection({
        apiBaseUrl: String(fields.get("apiBaseUrl") ?? ""),
        authValue: String(fields.get("authValue") ?? ""),
        fundingAuthValue: String(fields.get("fundingAuthValue") ?? ""),
      });
      setError(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Invalid connection.");
    }
  }

  return (
    <details className="connection">
      <summary>Connection</summary>
      <form onSubmit={save}>
        <p>Bearer values stay in this tab. Enter the credentials for the operations you use.</p>
        <fieldset disabled={console.state.submitting} className="connection__fields" key={console.state.apiBaseUrl}>
          <TextField label="API URL" name="apiBaseUrl" type="url" required defaultValue={console.state.apiBaseUrl} />
          <TextField label="Transaction bearer" name="authValue" type="password" autoComplete="off" defaultValue={console.state.authValue} />
          <TextField label="Funding bearer" name="fundingAuthValue" type="password" autoComplete="off" defaultValue={console.state.fundingAuthValue} />
          <Button type="submit" variant="secondary">Save connection</Button>
        </fieldset>
        {error && <p role="alert">{error}</p>}
      </form>
    </details>
  );
}
