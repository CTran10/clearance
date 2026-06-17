const STORAGE_KEYS = {
  apiBaseUrl: "clearance.apiBaseUrl",
  accessToken: "clearance.accessToken",
  user: "clearance.user",
};

const DEFAULT_API_BASE_URL = "http://127.0.0.1:8000";

let state = {
  apiBaseUrl: DEFAULT_API_BASE_URL,
  token: "",
  user: null,
  merchants: [],
  transactions: [],
  auditEvents: [],
  selectedTransactionId: null,
  idempotencyKey: createIdempotencyKey(),
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

export function buildAuthHeaders(token) {
  if (!token) {
    return {};
  }
  return { Authorization: `Bearer ${token}` };
}

export function summarizeDecisions(transactions) {
  return transactions.reduce(
    (summary, transaction) => {
      const status = transaction.status || "unknown";
      summary.total += 1;
      summary[status] = (summary[status] || 0) + 1;
      return summary;
    },
    { total: 0, approved: 0, declined: 0, review: 0 },
  );
}

export function decisionClass(status) {
  if (status === "approved") {
    return "status status-approved";
  }
  if (status === "declined") {
    return "status status-declined";
  }
  if (status === "review") {
    return "status status-review";
  }
  return "status";
}

export function formatCurrency(amount, currency) {
  const value = Number(amount);
  if (!Number.isFinite(value)) {
    return `${amount} ${currency}`;
  }
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: currency || "USD",
  }).format(value);
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

export function createIdempotencyKey() {
  // big realization: the idempotency key has to be made HERE on the client, once, before we send.
  // if i generated it on the server, every retry would get a fresh key and we'd happily charge twice — the whole
  // point is the retry reuses the SAME key. the client is the only one who knows "this is a retry of that"
  if (globalThis.crypto && typeof globalThis.crypto.randomUUID === "function") {
    return globalThis.crypto.randomUUID();
  }
  // randomUUID isn't on older browsers / non-https origins, so here's a jank-but-fine fallback. Math.random
  // is NOT cryptographically random but for a demo idempotency key nobody's attacking, who cares
  return `demo-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export async function parseApiError(response) {
  let body;
  try {
    body = await response.json();
  } catch {
    return `Request failed with ${response.status}`;
  }

  // FastAPI is sneaky here: "detail" is a plain string for my own HTTPExceptions, but a whole ARRAY of
  // {loc, msg} objects when pydantic validation fails. spent a while confused why my error toast said
  // "[object Object]" until i realized i had to handle both shapes 🙃
  if (typeof body.detail === "string") {
    return body.detail;
  }
  if (Array.isArray(body.detail)) {
    return body.detail
      .map((item) => {
        const field = Array.isArray(item.loc) ? item.loc.join(".") : "field";
        return `${field}: ${item.msg}`;
      })
      .join("; ");
  }
  return `Request failed with ${response.status}`;
}

export function sortNewestFirst(items) {
  return [...items].sort((left, right) => {
    return new Date(right.created_at).getTime() - new Date(left.created_at).getTime();
  });
}

function loadStateFromStorage() {
  state.apiBaseUrl = normalizeApiBaseUrl(localStorage.getItem(STORAGE_KEYS.apiBaseUrl));
  state.token = localStorage.getItem(STORAGE_KEYS.accessToken) || "";

  const storedUser = localStorage.getItem(STORAGE_KEYS.user);
  if (storedUser) {
    try {
      state.user = JSON.parse(storedUser);
    } catch {
      localStorage.removeItem(STORAGE_KEYS.user);
      state.user = null;
    }
  }
}

function persistSession({ accessToken, user }) {
  state.token = accessToken;
  state.user = user;
  localStorage.setItem(STORAGE_KEYS.accessToken, accessToken);
  localStorage.setItem(STORAGE_KEYS.user, JSON.stringify(user));
}

function clearSession() {
  state.token = "";
  state.user = null;
  state.merchants = [];
  state.transactions = [];
  state.auditEvents = [];
  state.selectedTransactionId = null;
  localStorage.removeItem(STORAGE_KEYS.accessToken);
  localStorage.removeItem(STORAGE_KEYS.user);
}

async function apiRequest(path, options = {}) {
  const headers = {
    Accept: "application/json",
    ...buildAuthHeaders(state.token),
    ...(options.headers || {}),
  };

  if (options.body && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }

  const response = await fetch(`${state.apiBaseUrl}${path}`, {
    ...options,
    headers,
  });

  if (!response.ok) {
    throw new Error(await parseApiError(response));
  }

  return response.json();
}

async function loadConsoleData() {
  if (!state.token) {
    return;
  }

  const [merchantData, transactionData, auditData] = await Promise.all([
    apiRequest("/merchants?limit=100"),
    apiRequest("/transactions?limit=100"),
    apiRequest("/audit-events?limit=100"),
  ]);

  state.merchants = merchantData.merchants || [];
  state.transactions = sortNewestFirst(transactionData.transactions || []);
  state.auditEvents = sortNewestFirst(auditData.audit_events || []);

  if (!state.selectedTransactionId && state.transactions.length > 0) {
    state.selectedTransactionId = state.transactions[0].id;
  }
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
    state.error = error instanceof Error ? error.message : "Something went wrong";
  } finally {
    state.busy = false;
    render();
  }
}

function handleApiBaseUrlSubmit(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  state.apiBaseUrl = normalizeApiBaseUrl(form.get("apiBaseUrl"));
  localStorage.setItem(STORAGE_KEYS.apiBaseUrl, state.apiBaseUrl);
  state.message = "API base URL updated.";
  state.error = "";
  render();
}

async function handleRegister(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  await withBusy(async () => {
    await apiRequest("/auth/register", {
      method: "POST",
      body: JSON.stringify({
        email: form.get("email"),
        password: form.get("password"),
      }),
    });
  }, "User registered. You can log in now.");
}

async function handleLogin(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  await withBusy(async () => {
    const data = await apiRequest("/auth/login", {
      method: "POST",
      body: JSON.stringify({
        email: form.get("email"),
        password: form.get("password"),
      }),
    });
    persistSession({ accessToken: data.access_token, user: data.user });
    await loadConsoleData();
  }, "Logged in and console data loaded.");
}

async function handleRefresh() {
  await withBusy(async () => {
    await loadConsoleData();
  }, "Console data refreshed.");
}

function handleLogout() {
  clearSession();
  state.message = "Logged out.";
  state.error = "";
  render();
}

async function handleCreateMerchant(event) {
  event.preventDefault();
  // BUG I JUST SPENT AN HOUR ON: i used to call event.currentTarget.reset() down inside the await callback,
  // and it kept throwing "cannot read reset of null". turns out the browser NULLS OUT event.currentTarget
  // the moment the event handler returns — and `await` makes us return early. so currentTarget is long gone
  // by the time the api call finishes. fix = grab the actual <form> node NOW, while it still exists, into a var.
  const formElement = event.currentTarget;
  const form = new FormData(formElement);
  await withBusy(async () => {
    await apiRequest("/merchants", {
      method: "POST",
      body: JSON.stringify({
        name: form.get("name"),
        category: form.get("category"),
        trust_status: form.get("trustStatus"),
      }),
    });
    formElement.reset();
    await loadConsoleData();
  }, "Merchant created.");
}

async function handleCreateTransaction(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const idempotencyKey = String(form.get("idempotencyKey") || "");
  await withBusy(async () => {
    const data = await apiRequest("/transactions", {
      method: "POST",
      headers: {
        "Idempotency-Key": idempotencyKey,
      },
      body: JSON.stringify({
        merchant_id: Number(form.get("merchantId")),
        amount: form.get("amount"),
        currency: form.get("currency"),
      }),
    });
    state.selectedTransactionId = data.id;
    state.idempotencyKey = idempotencyKey;
    await loadConsoleData();
  }, "Transaction submitted.");
}

function handleNewIdempotencyKey() {
  state.idempotencyKey = createIdempotencyKey();
  state.message = "New idempotency key generated.";
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

function field(label, input) {
  return el("label", { className: "field" }, [
    el("span", { text: label }),
    input,
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
    autocomplete: options.autocomplete,
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
  const summary = summarizeDecisions(state.transactions);

  return el("div", { className: "shell" }, [
    el("header", { className: "topbar" }, [
      el("div", {}, [
        el("p", { className: "eyebrow", text: "Clearance" }),
        el("h1", { text: "Operator Console" }),
      ]),
      el("div", { className: "session" }, [
        el("span", {
          className: "session-user",
          text: state.user ? state.user.email : "No active session",
        }),
        state.token ? button("Refresh", { onClick: handleRefresh }) : null,
        state.token ? button("Log out", { className: "button button-secondary", onClick: handleLogout }) : null,
      ]),
    ]),
    renderApiSettings(),
    renderNotice(),
    state.token ? renderConsole(summary) : renderAuth(),
  ]);
}

function renderApiSettings() {
  return el("form", { className: "api-bar", onSubmit: handleApiBaseUrlSubmit }, [
    field(
      "API base URL",
      textInput("apiBaseUrl", {
        value: state.apiBaseUrl,
        autocomplete: "url",
      }),
    ),
    button("Set API", { type: "submit", className: "button button-secondary" }),
  ]);
}

function renderNotice() {
  if (!state.error && !state.message) {
    if (state.token) {
      return el("div", {
        className: "notice notice-success",
        text: "Live API connected. Review authorization decisions, retry behavior, and audit history.",
      });
    }
    return el("div", {
      className: "notice notice-muted",
      text: "Connect to the local API, then log in to review live authorization behavior.",
    });
  }
  return el("div", {
    className: state.error ? "notice notice-error" : "notice notice-success",
    text: state.error || state.message,
  });
}

function renderAuth() {
  return el("main", { className: "auth-grid" }, [
    panel("Log in", [
      el("form", { className: "stack", onSubmit: handleLogin }, [
        field("Email", textInput("email", { type: "email", autocomplete: "email" })),
        field("Password", textInput("password", { type: "password", autocomplete: "current-password" })),
        button("Log in", { type: "submit" }),
      ]),
    ]),
    panel("Register", [
      el("form", { className: "stack", onSubmit: handleRegister }, [
        field("Email", textInput("email", { type: "email", autocomplete: "email" })),
        field("Password", textInput("password", { type: "password", autocomplete: "new-password" })),
        el("p", {
          className: "hint",
          text: "Password needs 8+ characters, a number, and a special character.",
        }),
        button("Create user", { type: "submit", className: "button button-secondary" }),
      ]),
    ]),
  ]);
}

function renderConsole(summary) {
  return el("main", { className: "console-grid" }, [
    renderSummary(summary),
    renderMerchantPanel(),
    renderTransactionPanel(),
    renderTransactionDetail(),
    renderAuditPanel(),
  ]);
}

function renderSummary(summary) {
  return el("section", { className: "metrics" }, [
    metric("Transactions", summary.total),
    metric("Approved", summary.approved),
    metric("Review", summary.review),
    metric("Declined", summary.declined),
  ]);
}

function metric(label, value) {
  return el("div", { className: "metric" }, [
    el("span", { text: label }),
    el("strong", { text: String(value) }),
  ]);
}

function renderMerchantPanel() {
  return panel("Merchants", [
    el("form", { className: "stack", onSubmit: handleCreateMerchant }, [
      field("Name", textInput("name", { placeholder: "Summit Coffee" })),
      field("Category", textInput("category", { placeholder: "food" })),
      field(
        "Trust status",
        el("select", { name: "trustStatus" }, [
          el("option", { value: "trusted", text: "trusted" }),
          el("option", { value: "untrusted", text: "untrusted" }),
        ]),
      ),
      button("Add merchant", { type: "submit" }),
    ]),
    renderMerchantList(),
  ]);
}

function renderMerchantList() {
  if (state.merchants.length === 0) {
    return el("p", { className: "empty", text: "No merchants yet." });
  }

  return el("div", { className: "list" }, state.merchants.map((merchant) => {
    return el("div", { className: "list-row" }, [
      el("div", {}, [
        el("strong", { text: merchant.name }),
        el("span", { text: `${merchant.category} / ${merchant.trust_status}` }),
      ]),
      el("span", { className: "muted", text: `#${merchant.id}` }),
    ]);
  }));
}

function renderTransactionPanel() {
  const hasMerchants = state.merchants.length > 0;

  return panel("Create Transaction", [
    hasMerchants
      ? el("form", { className: "stack", onSubmit: handleCreateTransaction }, [
          field(
            "Merchant",
            el("select", { name: "merchantId" }, state.merchants.map((merchant) => {
              return el("option", { value: String(merchant.id), text: `${merchant.name} #${merchant.id}` });
            })),
          ),
          field("Amount", textInput("amount", { type: "number", min: "0.01", step: "0.01", placeholder: "125.50" })),
          field("Currency", textInput("currency", { value: "USD", placeholder: "USD" })),
          field("Idempotency key", textInput("idempotencyKey", { value: state.idempotencyKey })),
          button("New key", {
            className: "button button-secondary",
            onClick: handleNewIdempotencyKey,
          }),
          el("p", {
            className: "hint",
            text: "Submit once, then submit again with the same key and payload to see the safe retry path.",
          }),
          button("Submit transaction", { type: "submit" }),
        ])
      : el("p", { className: "empty", text: "Create a merchant before submitting a transaction." }),
    renderTransactionList(),
  ]);
}

function renderTransactionList() {
  if (state.transactions.length === 0) {
    return el("p", { className: "empty", text: "No transactions yet." });
  }

  return el("div", { className: "table-wrap" }, [
    el("table", {}, [
      el("thead", {}, [
        el("tr", {}, [
          el("th", { text: "ID" }),
          el("th", { text: "Amount" }),
          el("th", { text: "Decision" }),
          el("th", { text: "Risk" }),
          el("th", { text: "Created" }),
        ]),
      ]),
      el("tbody", {}, state.transactions.map((transaction) => {
        return el("tr", {
          className: transaction.id === state.selectedTransactionId ? "selected-row" : "",
          onClick: () => {
            state.selectedTransactionId = transaction.id;
            render();
          },
        }, [
          el("td", { text: `#${transaction.id}` }),
          el("td", { text: formatCurrency(transaction.amount, transaction.currency) }),
          el("td", {}, [el("span", { className: decisionClass(transaction.status), text: transaction.status })]),
          el("td", { text: String(transaction.risk_score) }),
          el("td", { text: formatDateTime(transaction.created_at) }),
        ]);
      })),
    ]),
  ]);
}

function renderTransactionDetail() {
  const selected = state.transactions.find((transaction) => {
    return transaction.id === state.selectedTransactionId;
  });

  return panel("Decision Detail", [
    selected
      ? el("dl", { className: "detail-list" }, [
          detail("Transaction", `#${selected.id}`),
          detail("Merchant", `#${selected.merchant_id}`),
          detail("Status", selected.status),
          detail("Risk score", String(selected.risk_score)),
          detail("Reason", selected.decision_reason),
          detail("Created", formatDateTime(selected.created_at)),
        ])
      : el("p", { className: "empty", text: "Select a transaction to inspect the authorization decision." }),
  ]);
}

function detail(term, description) {
  return el("div", {}, [
    el("dt", { text: term }),
    el("dd", { text: description }),
  ]);
}

function renderAuditPanel() {
  return panel("Audit Events", [
    state.auditEvents.length === 0
      ? el("p", { className: "empty", text: "No audit events yet." })
      : el("div", { className: "list audit-list" }, state.auditEvents.map((event) => {
          return el("div", { className: "list-row" }, [
            el("div", {}, [
              el("strong", { text: event.action }),
              el("span", { text: `${event.entity_type} ${event.entity_id || ""}`.trim() }),
            ]),
            el("span", { className: "muted", text: formatDateTime(event.created_at) }),
          ]);
        })),
  ]);
}

function render() {
  const root = document.getElementById("app");
  if (!root) {
    return;
  }
  root.replaceChildren(renderShell());
}

async function boot() {
  loadStateFromStorage();
  if (state.token) {
    try {
      await loadConsoleData();
    } catch {
      clearSession();
      state.error = "Saved session could not be restored. Please log in again.";
    }
  }
  render();
}

if (typeof document !== "undefined") {
  boot();
}
