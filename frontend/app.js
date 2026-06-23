const STORAGE_KEYS = {
  apiBaseUrl: "clearance.apiBaseUrl",
};

const DEFAULT_API_BASE_URL = "http://127.0.0.1:8080";
const SAFE_TOKEN_PATTERN = /^[A-Za-z0-9._:-]{1,128}$/;

let state = {
  apiBaseUrl: DEFAULT_API_BASE_URL,
  authValue: "",
  receipts: [],
  idempotencyKey: createSafeId("idem"),
  correlationId: createSafeId("trace"),
  health: "unknown",
  busy: false,
  message: "",
  error: "",
};

export function normalizeApiBaseUrl(value) {
  const trimmed = String(value || "").trim();
  if (!trimmed) {
    return DEFAULT_API_BASE_URL;
  }
  return trimmed.replace(/\/+$/, "");
}

export function buildTransactionHeaders(authValue, idempotencyKey, correlationId) {
  const headers = {
    "Content-Type": "application/json",
    "Idempotency-Key": String(idempotencyKey || "").trim(),
    "X-Correlation-ID": String(correlationId || "").trim(),
  };
  const bearer = String(authValue || "").trim();
  if (bearer) {
    headers.Authorization = `Bearer ${bearer}`;
  }
  return headers;
}

export function buildTransactionPayload(input) {
  const accountId = String(input.accountId || "").trim();
  const merchantId = String(input.merchantId || "").trim();
  const amountText = String(input.amountCents || "").trim();
  const currency = String(input.currency || "").trim().toUpperCase();

  if (!accountId) {
    throw new Error("account id is required");
  }
  if (!merchantId) {
    throw new Error("merchant id is required");
  }
  if (!SAFE_TOKEN_PATTERN.test(accountId) || !SAFE_TOKEN_PATTERN.test(merchantId)) {
    throw new Error("ids must use safe characters only");
  }
  // all this client-side validation is for being NICE (instant feedback), NOT for security. anyone can curl
  // the api directly and skip this entirely, so the server re-checks everything. learned not to confuse
  // "the form won't let me" with "the system won't let me" — those are very different promises
  if (!/^\d+$/.test(amountText)) {
    throw new Error("amount cents must be a whole number");
  }
  const amountCents = Number(amountText);
  if (!Number.isSafeInteger(amountCents) || amountCents <= 0) {
    throw new Error("amount cents must be greater than zero");
  }
  if (!/^[A-Z]{3}$/.test(currency)) {
    throw new Error("currency must be a three-letter code");
  }

  return {
    account_id: accountId,
    merchant_id: merchantId,
    amount_cents: amountCents,
    currency,
  };
}

// this is JUST a guess shown in the UI so the user isn't surprised — the SERVER is the real judge.
// i duplicated the >$500 rule here on the client, which feels gross (two sources of truth) but it's only cosmetic.
// if these ever disagree, the server wins, always. the frontend never actually decides anything
export function riskPreview(amountCents) {
  if (Number(amountCents) > 50_000) {
    return {
      level: "HIGH",
      outcome: "likely failed",
      reason: "Amount is greater than 500.00.",
    };
  }
  return {
    level: "LOW",
    outcome: "likely authorized",
    reason: "Amount is at or below 500.00.",
  };
}

export function summarizeReceipts(receipts) {
  return receipts.reduce(
    (summary, receipt) => {
      summary.total += 1;
      if (receipt.status === "PENDING") {
        summary.pending += 1;
      }
      if (receipt.previewRisk === "LOW") {
        summary.lowRisk += 1;
      }
      if (receipt.previewRisk === "HIGH") {
        summary.highRisk += 1;
      }
      return summary;
    },
    { total: 0, pending: 0, lowRisk: 0, highRisk: 0 },
  );
}

export function statusClass(status) {
  if (status === "PENDING") {
    return "status status-pending";
  }
  if (status === "AUTHORIZED") {
    return "status status-authorized";
  }
  if (status === "FAILED") {
    return "status status-failed";
  }
  if (status === "LOW") {
    return "status status-low";
  }
  if (status === "HIGH") {
    return "status status-high";
  }
  return "status";
}

export function formatAmountCents(amountCents, currency) {
  const cents = Number(amountCents);
  if (!Number.isFinite(cents)) {
    return `${amountCents} ${currency}`;
  }
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: currency || "USD",
  }).format(cents / 100); // store + send integer cents everywhere, divide by 100 ONLY here at the very last second for human eyes
}

export function formatDateTime(value) {
  if (!value) {
    return "Not available";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

export async function parseApiError(response) {
  let body;
  try {
    body = await response.json();
  } catch {
    return `Request failed with ${response.status}`;
  }

  if (typeof body.error === "string") {
    return body.error;
  }
  if (typeof body.detail === "string") {
    return body.detail;
  }
  return `Request failed with ${response.status}`;
}

function createSafeId(prefix) {
  if (globalThis.crypto && typeof globalThis.crypto.randomUUID === "function") {
    return `${prefix}-${globalThis.crypto.randomUUID()}`;
  }
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function loadStateFromStorage() {
  state.apiBaseUrl = normalizeApiBaseUrl(localStorage.getItem(STORAGE_KEYS.apiBaseUrl));
}

async function apiRequest(path, options = {}) {
  const response = await fetch(`${state.apiBaseUrl}${path}`, options);
  if (!response.ok) {
    throw new Error(await parseApiError(response));
  }
  return response.json();
}

async function withBusy(work, successMessage) {
  state.busy = true;
  state.error = "";
  state.message = "";
  render();

  try {
    await work();
    state.message = successMessage || "";
  } catch (error) {
    state.error = error instanceof Error ? error.message : "Request failed";
  } finally {
    state.busy = false;
    render();
  }
}

function handleSettingsSubmit(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  state.apiBaseUrl = normalizeApiBaseUrl(form.get("apiBaseUrl"));
  state.authValue = String(form.get("authValue") || "");
  localStorage.setItem(STORAGE_KEYS.apiBaseUrl, state.apiBaseUrl);
  state.message = "Connection settings updated. Bearer value stays in this tab.";
  state.error = "";
  render();
}

async function handleHealthCheck() {
  await withBusy(async () => {
    await apiRequest("/healthz");
    state.health = "ok";
  }, "Health check passed.");
}

async function handleCreateTransaction(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);

  await withBusy(async () => {
    const payload = buildTransactionPayload({
      accountId: form.get("accountId"),
      merchantId: form.get("merchantId"),
      amountCents: form.get("amountCents"),
      currency: form.get("currency"),
    });
    const idempotencyKey = String(form.get("idempotencyKey") || "").trim();
    const correlationId = String(form.get("correlationId") || "").trim();
    if (!SAFE_TOKEN_PATTERN.test(idempotencyKey)) {
      throw new Error("idempotency key must use safe characters only");
    }
    if (!SAFE_TOKEN_PATTERN.test(correlationId)) {
      throw new Error("correlation id must use safe characters only");
    }

    const response = await apiRequest("/transactions", {
      method: "POST",
      headers: buildTransactionHeaders(state.authValue, idempotencyKey, correlationId),
      body: JSON.stringify(payload),
    });
    const preview = riskPreview(payload.amount_cents);
    state.receipts = [
      {
        transactionId: response.transaction_id,
        status: response.status,
        correlationId: response.correlation_id || correlationId,
        idempotencyKey,
        accountId: payload.account_id,
        merchantId: payload.merchant_id,
        amountCents: payload.amount_cents,
        currency: payload.currency,
        previewRisk: preview.level,
        previewOutcome: preview.outcome,
        previewReason: preview.reason,
        createdAt: new Date().toISOString(),
      },
      ...state.receipts,
    ].slice(0, 12);
    state.idempotencyKey = idempotencyKey;
    state.correlationId = response.correlation_id || correlationId;
  }, "Transaction accepted as pending.");
}

function handleNewKeys() {
  state.idempotencyKey = createSafeId("idem");
  state.correlationId = createSafeId("trace");
  state.message = "New idempotency and correlation IDs generated.";
  state.error = "";
  render();
}

function el(tagName, attributes = {}, children = []) {
  const element = document.createElement(tagName);

  Object.entries(attributes).forEach(([key, value]) => {
    if (value === false || value === null || value === undefined) {
      return;
    }
    if (key === "className") {
      element.className = value;
      return;
    }
    if (key === "text") {
      element.textContent = value;
      return;
    }
    if (key === "dataset") {
      Object.entries(value).forEach(([dataKey, dataValue]) => {
        element.dataset[dataKey] = dataValue;
      });
      return;
    }
    if (key.startsWith("on") && typeof value === "function") {
      element.addEventListener(key.slice(2).toLowerCase(), value);
      return;
    }
    element.setAttribute(key, value);
  });

  children.forEach((child) => {
    if (child === null || child === undefined) {
      return;
    }
    if (typeof child === "string" || typeof child === "number") {
      element.appendChild(document.createTextNode(String(child)));
      return;
    }
    element.appendChild(child);
  });

  return element;
}

function field(label, input, helper = null) {
  return el("label", { className: "field" }, [
    el("span", { text: label }),
    input,
    helper ? el("small", { text: helper }) : null,
  ]);
}

function textInput(name, options = {}) {
  return el("input", {
    name,
    type: options.type || "text",
    value: options.value,
    placeholder: options.placeholder,
    min: options.min,
    step: options.step,
    required: options.required !== false,
    autocomplete: options.autocomplete || "off",
    inputmode: options.inputmode,
  });
}

function button(label, options = {}) {
  return el("button", {
    type: options.type || "button",
    className: options.className || "button",
    disabled: state.busy || options.disabled,
    onClick: options.onClick,
    text: label,
  });
}

function panel(title, children, actions = null) {
  return el("section", { className: "panel" }, [
    el("div", { className: "panel-header" }, [
      el("h2", { text: title }),
      actions,
    ]),
    ...children,
  ]);
}

function renderShell() {
  const summary = summarizeReceipts(state.receipts);
  return el("div", { className: "shell" }, [
    el("a", { className: "skip-link", href: "#console", text: "Skip to console" }),
    el("header", { className: "topbar" }, [
      el("div", {}, [
        el("p", { className: "eyebrow", text: "Clearance" }),
        el("h1", { text: "Event authorization console" }),
        el("p", {
          className: "lede",
          text: "Submit transactions to the Go platform and follow the idempotency and event-flow receipt.",
        }),
      ]),
      el("div", { className: "health-card" }, [
        el("span", { className: statusClass(state.health === "ok" ? "AUTHORIZED" : "PENDING"), text: state.health }),
        button("Check health", { className: "button button-secondary", onClick: handleHealthCheck }),
      ]),
    ]),
    renderSettings(),
    renderNotice(),
    el("main", { id: "console", className: "console-grid" }, [
      renderMetrics(summary),
      renderTransactionPanel(),
      renderFlowPanel(),
      renderReceiptsPanel(),
    ]),
  ]);
}

function renderSettings() {
  return el("form", { className: "api-bar", onSubmit: handleSettingsSubmit }, [
    field("API base URL", textInput("apiBaseUrl", {
      value: state.apiBaseUrl,
      autocomplete: "url",
    })),
    field("Bearer value", textInput("authValue", {
      type: "password",
      value: state.authValue,
      autocomplete: "off",
    }), "Stored in memory only."),
    button("Save settings", { type: "submit", className: "button button-secondary" }),
  ]);
}

function renderNotice() {
  if (!state.error && !state.message) {
    return el("div", {
      className: "notice notice-muted",
      text: "Start the Compose stack, set the local bearer value, then submit a LOW or HIGH risk transaction.",
    });
  }
  return el("div", {
    className: state.error ? "notice notice-error" : "notice notice-success",
    text: state.error || state.message,
  });
}

function renderMetrics(summary) {
  return el("section", { className: "metrics" }, [
    metric("Receipts", summary.total),
    metric("Pending replies", summary.pending),
    metric("LOW preview", summary.lowRisk),
    metric("HIGH preview", summary.highRisk),
  ]);
}

function metric(label, value) {
  return el("div", { className: "metric" }, [
    el("span", { text: label }),
    el("strong", { text: String(value) }),
  ]);
}

function renderTransactionPanel() {
  return panel("Submit transaction", [
    el("form", { className: "stack", onSubmit: handleCreateTransaction }, [
      field("Account ID", textInput("accountId", { value: "acct_123" })),
      field("Merchant ID", textInput("merchantId", { value: "merchant_123" })),
      field("Amount cents", textInput("amountCents", {
        type: "number",
        min: "1",
        step: "1",
        inputmode: "numeric",
        value: "12550",
      }), "Use 50001 or higher to preview HIGH risk."),
      field("Currency", textInput("currency", { value: "USD" })),
      field("Idempotency key", textInput("idempotencyKey", { value: state.idempotencyKey })),
      field("Correlation ID", textInput("correlationId", { value: state.correlationId })),
      el("div", { className: "button-row" }, [
        button("Submit", { type: "submit" }),
        button("New IDs", {
          className: "button button-secondary",
          onClick: handleNewKeys,
        }),
      ]),
    ]),
  ]);
}

function renderFlowPanel() {
  return panel("Platform flow", [
    el("ol", { className: "flow-list" }, [
      flowStep("Transaction Service", "Validates input, checks Redis rate limit, records PENDING and writes TransactionCreated to the outbox."),
      flowStep("Outbox Publisher", "Publishes TransactionCreated to Redpanda and marks the outbox row published."),
      flowStep("Risk Service", "Consumes the event. Amounts over 500.00 become HIGH risk."),
      flowStep("Ledger Service", "Checks available balance, writes entries for funded LOW risk transactions, and emits the final event."),
    ]),
  ]);
}

function flowStep(title, body) {
  return el("li", {}, [
    el("strong", { text: title }),
    el("span", { text: body }),
  ]);
}

function renderReceiptsPanel() {
  return panel("Local receipts", [
    state.receipts.length === 0
      ? el("p", { className: "empty", text: "No local receipts yet. Submit a transaction to see the accepted response." })
      : el("div", { className: "receipt-list" }, state.receipts.map(renderReceipt)),
  ]);
}

function renderReceipt(receipt) {
  return el("article", { className: "receipt" }, [
    el("div", { className: "receipt-topline" }, [
      el("strong", { text: receipt.transactionId || "Transaction pending" }),
      el("span", { className: statusClass(receipt.status), text: receipt.status || "PENDING" }),
    ]),
    el("div", { className: "receipt-grid" }, [
      detail("Amount", formatAmountCents(receipt.amountCents, receipt.currency)),
      detail("Preview", `${receipt.previewRisk} risk, ${receipt.previewOutcome}`),
      detail("Correlation", receipt.correlationId),
      detail("Idempotency", receipt.idempotencyKey),
      detail("Account", receipt.accountId),
      detail("Merchant", receipt.merchantId),
      detail("Created", formatDateTime(receipt.createdAt)),
      detail("Reason", receipt.previewReason),
    ]),
  ]);
}

function detail(term, description) {
  return el("div", {}, [
    el("dt", { text: term }),
    el("dd", { text: description }),
  ]);
}

function render() {
  const root = document.getElementById("app");
  if (!root) {
    return;
  }
  root.replaceChildren(renderShell());
}

function boot() {
  loadStateFromStorage();
  render();
}

if (typeof document !== "undefined") {
  boot();
}
