const endpoints = [
  { group: "System", method: "GET", path: "/healthz", title: "Health check" },
  { group: "System", method: "GET", path: "/readyz", title: "Readiness check" },
  { group: "System", method: "GET", path: "/openapi.yaml", title: "OpenAPI YAML" },
  { group: "Me", method: "GET", path: "/v1/me", title: "Resolve current account profile" },
  { group: "Me", method: "GET", path: "/v1/me/menus", title: "List current account menus" },
  { group: "Authz", method: "POST", path: "/v1/authz/check", title: "Check one authorization request", sample: { application_code: "hr", resource_type: "employee", action: "export", resource_id: "emp-employee" } },
  { group: "Authz", method: "POST", path: "/v1/authz/batch-check", title: "Check multiple authorization requests", sample: { checks: [{ application_code: "hr", resource_type: "employee", action: "read" }, { application_code: "audit", resource_type: "audit_log", action: "read" }] } },
  { group: "Authz", method: "POST", path: "/v1/authz/explain", title: "Explain authorization decision", sample: { application_code: "hr", resource_type: "employee", action: "read", resource_id: "emp-admin" } },
  { group: "Authz", method: "POST", path: "/v1/authz/simulate", title: "Simulate authorization decision", approval: true, sample: { application_code: "iam", resource_type: "permission_set", action: "create" } },
  { group: "IAM", method: "GET", path: "/v1/iam/permissions", title: "List permissions", query: { page: "1", page_size: "20" } },
  { group: "IAM", method: "GET", path: "/v1/iam/user-groups", title: "List user groups", query: { page: "1", page_size: "20" } },
  { group: "IAM", method: "POST", path: "/v1/iam/user-groups", title: "Create user group", approval: true, sample: { name: "Finance Admin", description: "Created from API tester", permission_set_ids: ["ps-audit"], member_account_ids: ["acct-audit"] } },
  { group: "IAM", method: "GET", path: "/v1/iam/permission-sets", title: "List permission sets", query: { page: "1", page_size: "20" } },
  { group: "IAM", method: "POST", path: "/v1/iam/permission-sets", title: "Create permission set", approval: true, sample: { name: "HR Read Sample", description: "Created from API tester", permissions: [{ resource: "hr.employee", action: "read", scope: "all" }] } },
  { group: "IAM", method: "GET", path: "/v1/iam/permission-set-assignments", title: "List permission set assignments", query: { page: "1", page_size: "20" } },
  { group: "IAM", method: "POST", path: "/v1/iam/permission-set-assignments", title: "Create permission set assignment", approval: true, sample: { principal_type: "account", principal_id: "acct-employee", permission_set_id: "ps-audit" } },
  { group: "IAM", method: "GET", path: "/v1/iam/data-scopes", title: "List data scopes", query: { page: "1", page_size: "20" } },
  { group: "IAM", method: "POST", path: "/v1/iam/data-scopes", title: "Create data scope", approval: true, sample: { code: "sample_scope", name: "Sample Scope", scope_type: "self", params: {} } },
  { group: "IAM", method: "GET", path: "/v1/iam/field-policies", title: "List field policies", query: { application_code: "hr", resource_type: "employee", page: "1", page_size: "20" } },
  { group: "IAM", method: "POST", path: "/v1/iam/field-policies", title: "Create field policy", approval: true, sample: { application_code: "hr", resource_type: "employee", field_name: "basic_info.national_id", effect: "mask", mask_strategy: "partial" } },
  { group: "IAM", method: "GET", path: "/v1/iam/assumable-roles", title: "List assumable roles", query: { page: "1", page_size: "20" } },
  { group: "IAM", method: "POST", path: "/v1/iam/assumable-roles", title: "Create assumable role", approval: true, sample: { name: "Audit Assume", trusted: true, trust_policy: { accounts: ["acct-admin"] }, permission_boundary: { allow: ["audit.log.read", "iam.permission_set.read"] }, permission_set_ids: ["ps-audit"], session_duration_seconds: 3600 } },
  { group: "IAM", method: "POST", path: "/v1/iam/assumable-roles/{id}/assume", title: "Assume role", approval: true, params: { id: "role-id" }, sample: { reason: "local test", duration_minutes: 30 } },
  { group: "HR", method: "GET", path: "/v1/hr/employees", title: "List employees", query: { page: "1", page_size: "20", sort: "created_at_desc" } },
  { group: "HR", method: "POST", path: "/v1/hr/employees", title: "Create employee aggregate", sample: { name: "API Tester Person", company_email: "api.tester.person@example.com", category: "full_time", employment_status: "onboarding", org_unit_id: "ou-hq", position: "Tester" } },
  { group: "HR", method: "GET", path: "/v1/hr/employees/{id}", title: "Get employee detail", params: { id: "emp-admin" } },
  { group: "HR", method: "PATCH", path: "/v1/hr/employees/{id}", title: "Patch employee aggregate", params: { id: "emp-admin" }, sample: { phone: "0911222333", contact_info: { mobile_phone: "0911222333" } } },
  { group: "HR", method: "DELETE", path: "/v1/hr/employees/{id}", title: "Soft delete employee", approval: true, params: { id: "emp-employee" } },
  { group: "HR", method: "GET", path: "/v1/hr/employees/stats", title: "Employee dashboard counts" },
  { group: "HR", method: "GET", path: "/v1/hr/employee-options", title: "Employee form options" },
  { group: "HR", method: "POST", path: "/v1/hr/employees/import/preview", title: "Preview employee CSV import", approval: true, sample: { filename: "employees.csv", content: "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\nE9001,Nina Lin,nina@example.com,ou-hq,Recruiter,全職,0911888999,在職,2026-06-01,E0001\n" } },
  { group: "HR", method: "POST", path: "/v1/hr/employees/import/{id}/confirm", title: "Confirm employee import", approval: true, params: { id: "session-id" }, sample: { mode: "create" } },
  { group: "HR", method: "GET", path: "/v1/hr/employees/export", title: "Export employees CSV", approval: true },
  { group: "HR", method: "POST", path: "/v1/hr/employees/export", title: "Export filtered employees JSON", approval: true, sample: { page: 1, page_size: 100, sort: "created_at_desc" } },
  { group: "HR", method: "POST", path: "/v1/hr/employees/batch-delete", title: "Batch soft delete employees", approval: true, sample: { employee_ids: ["emp-employee", "emp-missing"], reason: "local cleanup test" } },
  { group: "HR", method: "POST", path: "/v1/hr/employees/{id}/invite", title: "Create or resend employee invitation", approval: true, params: { id: "emp-employee" }, sample: { email: "employee@demo.local" } },
  { group: "HR", method: "POST", path: "/v1/hr/employees/{id}/status-transition", title: "Transition employee lifecycle status", approval: true, params: { id: "emp-employee" }, sample: { status: "leave_suspended", reason: "local test", start_date: "2026-06-17", end_date: "2026-06-24" } },
  { group: "HR", method: "PATCH", path: "/v1/hr/employees/{id}/status", title: "Update employee status directly", approval: true, params: { id: "emp-employee" }, sample: { status: "probation" } },
  { group: "Org", method: "GET", path: "/v1/org/units", title: "List organization units", query: { page: "1", page_size: "20" } },
  { group: "Org", method: "POST", path: "/v1/org/units", title: "Create organization unit", sample: { code: "QA", name: "Quality Team", parent_id: "ou-hq" } },
  { group: "Attendance", method: "GET", path: "/v1/attendance/leave-balances", title: "List leave balances", query: { page: "1", page_size: "20" } },
  { group: "Attendance", method: "GET", path: "/v1/attendance/leave-requests", title: "List leave requests", query: { page: "1", page_size: "20" } },
  { group: "Attendance", method: "POST", path: "/v1/attendance/leave-requests", title: "Create leave request", sample: { employee_id: "emp-employee", leave_type: "annual", start_at: "2026-06-18T09:00:00Z", end_at: "2026-06-18T18:00:00Z", hours: 8, reason: "local test" } },
  { group: "Workflow", method: "GET", path: "/v1/forms/templates", title: "List form templates", query: { page: "1", page_size: "20" } },
  { group: "Workflow", method: "POST", path: "/v1/forms/templates", title: "Create form template", sample: { key: "expense-test", name: "Expense Test", description: "Created from API tester", schema: { fields: [{ name: "amount", type: "number" }] } } },
  { group: "Workflow", method: "POST", path: "/v1/workflows/forms/{id}/submit", title: "Submit workflow form", params: { id: "leave-request" }, sample: { template_key: "leave-request", payload: { hours: 8, reason: "local test" } } },
  { group: "Agent", method: "GET", path: "/v1/agents/runs", title: "List agent runs", query: { page: "1", page_size: "20" } },
  { group: "Agent", method: "POST", path: "/v1/agents/runs", title: "Create agent run", approval: true, sample: { mode: "assistant", prompt: "Summarize demo HR data" } },
  { group: "Audit", method: "GET", path: "/v1/audit-logs", title: "List audit logs", approval: true, query: { page: "1", page_size: "20" } },
];

const methods = ["GET", "POST", "PATCH", "DELETE"];
const state = {
  active: endpoints[0],
  params: {},
  query: {},
  lastCurl: "",
};

const el = id => document.getElementById(id);
const els = {
  baseUrl: el("baseUrl"),
  transport: el("transport"),
  token: el("token"),
  tenantId: el("tenantId"),
  accountId: el("accountId"),
  assumedSessionId: el("assumedSessionId"),
  approvalConfirmed: el("approvalConfirmed"),
  endpointSearch: el("endpointSearch"),
  endpointList: el("endpointList"),
  method: el("method"),
  path: el("path"),
  methodBadge: el("methodBadge"),
  endpointTitle: el("endpointTitle"),
  endpointMeta: el("endpointMeta"),
  pathParams: el("pathParams"),
  queryParams: el("queryParams"),
  body: el("body"),
  responseStatus: el("responseStatus"),
  responseMetrics: el("responseMetrics"),
  responseBody: el("responseBody"),
  smokeResults: el("smokeResults"),
};

init();

function init() {
  for (const method of methods) {
    els.method.append(new Option(method, method));
  }
  loadSettings();
  renderEndpointList();
  selectEndpoint(endpoints[0]);
  bindEvents();
}

function bindEvents() {
  for (const input of [els.baseUrl, els.transport, els.token, els.tenantId, els.accountId, els.assumedSessionId, els.approvalConfirmed]) {
    input.addEventListener("input", saveSettings);
    input.addEventListener("change", saveSettings);
  }
  els.endpointSearch.addEventListener("input", renderEndpointList);
  els.method.addEventListener("change", () => {
    state.active = { ...state.active, method: els.method.value };
    syncMethodBadge();
  });
  els.path.addEventListener("input", renderPathParams);
  el("sendBtn").addEventListener("click", sendCurrentRequest);
  el("formatBodyBtn").addEventListener("click", formatBody);
  el("resetBodyBtn").addEventListener("click", () => resetBody(state.active));
  el("clearPathParamsBtn").addEventListener("click", () => {
    state.params = {};
    renderPathParams();
  });
  el("clearQueryBtn").addEventListener("click", () => {
    state.query = {};
    renderQueryParams([]);
  });
  el("copyCurlBtn").addEventListener("click", copyCurl);
  el("smokeBtn").addEventListener("click", runSmoke);
  el("generateTokenBtn").addEventListener("click", generateUnsignedJWT);
}

function loadSettings() {
  const saved = JSON.parse(localStorage.getItem("nexus-api-tester") || "{}");
  els.baseUrl.value = saved.baseUrl || "http://localhost:8080";
  els.transport.value = saved.transport || "proxy";
  els.token.value = saved.token || "";
  els.tenantId.value = saved.tenantId || "demo";
  els.accountId.value = saved.accountId || "acct-admin";
  els.assumedSessionId.value = saved.assumedSessionId || "";
  els.approvalConfirmed.checked = saved.approvalConfirmed ?? false;
}

function saveSettings() {
  localStorage.setItem("nexus-api-tester", JSON.stringify({
    baseUrl: els.baseUrl.value.trim(),
    transport: els.transport.value,
    token: els.token.value.trim(),
    tenantId: els.tenantId.value.trim(),
    accountId: els.accountId.value.trim(),
    assumedSessionId: els.assumedSessionId.value.trim(),
    approvalConfirmed: els.approvalConfirmed.checked,
  }));
}

function renderEndpointList() {
  const query = els.endpointSearch.value.trim().toLowerCase();
  els.endpointList.innerHTML = "";
  endpoints
    .filter(endpoint => !query || `${endpoint.group} ${endpoint.method} ${endpoint.path} ${endpoint.title}`.toLowerCase().includes(query))
    .forEach(endpoint => {
      const item = document.createElement("button");
      item.type = "button";
      item.className = `endpoint-item${sameEndpoint(endpoint, state.active) ? " active" : ""}`;
      item.innerHTML = `
        <span class="endpoint-row">
          <span class="method ${endpoint.method.toLowerCase()}">${endpoint.method}</span>
          <span>${escapeHTML(endpoint.title)}</span>
          ${endpoint.approval ? '<span class="tag">approval</span>' : ""}
        </span>
        <span class="endpoint-path">${escapeHTML(endpoint.path)}</span>
      `;
      item.addEventListener("click", () => selectEndpoint(endpoint));
      els.endpointList.append(item);
    });
}

function selectEndpoint(endpoint) {
  state.active = endpoint;
  state.params = { ...(endpoint.params || {}) };
  state.query = { ...(endpoint.query || {}) };
  els.method.value = endpoint.method;
  els.path.value = endpoint.path;
  els.endpointTitle.textContent = endpoint.title;
  els.endpointMeta.textContent = `${endpoint.group} / ${endpoint.path}${endpoint.approval ? " / high-risk approval" : ""}`;
  if (endpoint.approval) {
    els.approvalConfirmed.checked = true;
    saveSettings();
  }
  syncMethodBadge();
  renderPathParams();
  renderQueryParams(Object.keys(state.query));
  resetBody(endpoint);
  renderEndpointList();
}

function sameEndpoint(left, right) {
  return Boolean(left && right && left.method === right.method && left.path === right.path && left.title === right.title);
}

function syncMethodBadge() {
  const method = els.method.value;
  els.methodBadge.textContent = method;
  els.methodBadge.className = `method ${method.toLowerCase()}`;
}

function renderPathParams() {
  const names = [...els.path.value.matchAll(/\{([^}]+)\}/g)].map(match => match[1]);
  renderKeyValueList(els.pathParams, names, state.params, renderPathParams);
}

function renderQueryParams(names) {
  const keys = names.length ? names : Object.keys(state.query);
  renderKeyValueList(els.queryParams, keys, state.query, () => renderQueryParams([]), true);
}

function renderKeyValueList(container, names, target, rerender, allowAdd = false) {
  container.innerHTML = "";
  container.classList.toggle("empty-state", names.length === 0 && !allowAdd);
  if (names.length === 0 && !allowAdd) {
    container.textContent = "No params.";
    return;
  }
  for (const name of names) {
    const row = document.createElement("div");
    row.className = "kv-row";
    const label = document.createElement("span");
    label.textContent = name;
    const input = document.createElement("input");
    input.value = target[name] ?? "";
    input.addEventListener("input", () => {
      target[name] = input.value;
    });
    row.append(label, input);
    container.append(row);
  }
  if (allowAdd) {
    container.classList.remove("empty-state");
    const row = document.createElement("div");
    row.className = "kv-row";
    const key = document.createElement("input");
    const value = document.createElement("input");
    key.placeholder = "param";
    value.placeholder = "value";
    key.addEventListener("change", () => {
      const nextKey = key.value.trim();
      if (!nextKey) return;
      target[nextKey] = value.value;
      rerender();
    });
    value.addEventListener("change", () => {
      const nextKey = key.value.trim();
      if (!nextKey) return;
      target[nextKey] = value.value;
      rerender();
    });
    row.append(key, value);
    container.append(row);
  }
}

function resetBody(endpoint) {
  els.body.value = endpoint.sample ? JSON.stringify(endpoint.sample, null, 2) : "";
}

function formatBody() {
  if (!els.body.value.trim()) return;
  try {
    els.body.value = JSON.stringify(JSON.parse(els.body.value), null, 2);
  } catch (error) {
    showLocalError(`JSON format error: ${error.message}`);
  }
}

async function sendCurrentRequest() {
  els.smokeResults.hidden = true;
  await sendRequest(buildRequestFromForm());
}

function buildRequestFromForm() {
  let path = els.path.value.trim() || "/";
  for (const [key, value] of Object.entries(state.params)) {
    path = path.replaceAll(`{${key}}`, encodeURIComponent(value));
  }
  const url = new URL(path, normalizedBaseUrl());
  for (const [key, value] of Object.entries(state.query)) {
    if (value !== "") {
      url.searchParams.set(key, value);
    }
  }
  const method = els.method.value;
  const bodyText = els.body.value.trim();
  return {
    method,
    url: url.toString(),
    body: ["GET", "HEAD"].includes(method) || !bodyText ? null : bodyText,
    headers: requestHeaders(Boolean(bodyText) && !["GET", "HEAD"].includes(method)),
  };
}

function requestHeaders(hasJSONBody) {
  const headers = { Accept: "application/json, text/csv, text/plain, */*" };
  const token = els.token.value.trim();
  if (token) {
    headers.Authorization = token.startsWith("Bearer ") ? token : `Bearer ${token}`;
  }
  if (els.tenantId.value.trim()) headers["X-Tenant-ID"] = els.tenantId.value.trim();
  if (els.accountId.value.trim()) headers["X-Account-ID"] = els.accountId.value.trim();
  if (els.assumedSessionId.value.trim()) headers["X-Assumable-Role-Session-ID"] = els.assumedSessionId.value.trim();
  if (els.approvalConfirmed.checked) headers["X-Approval-Confirmed"] = "true";
  headers["X-Request-ID"] = `api_tester_${Date.now()}`;
  if (hasJSONBody) headers["Content-Type"] = "application/json";
  return headers;
}

async function sendRequest(request) {
  state.lastCurl = toCurl(request);
  setResponseState("Sending", "", "{}");
  const started = performance.now();
  try {
    const result = els.transport.value === "proxy"
      ? await sendViaProxy(request)
      : await sendDirect(request);
    const elapsedMs = Math.round(performance.now() - started);
    const statusClass = result.status >= 200 && result.status < 300 ? "status-ok" : result.status >= 400 ? "status-error" : "status-warn";
    els.responseStatus.innerHTML = `<span class="${statusClass}">${result.status} ${escapeHTML(result.statusText || "")}</span>`;
    els.responseMetrics.textContent = `${result.elapsedMs ?? elapsedMs} ms`;
    els.responseBody.textContent = formatResponse(result);
    return result;
  } catch (error) {
    showLocalError(error.message);
    throw error;
  }
}

async function sendViaProxy(request) {
  const response = await fetch("/__proxy", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request),
  });
  if (!response.ok) {
    throw new Error(await response.text());
  }
  return response.json();
}

async function sendDirect(request) {
  const started = performance.now();
  const response = await fetch(request.url, {
    method: request.method,
    headers: request.headers,
    body: request.body,
  });
  const text = await response.text();
  const headers = {};
  response.headers.forEach((value, key) => {
    headers[key] = value;
  });
  return {
    status: response.status,
    statusText: response.statusText,
    elapsedMs: Math.round(performance.now() - started),
    headers,
    body: text,
    bodyEncoding: "utf8",
  };
}

async function runSmoke() {
  els.smokeResults.hidden = false;
  els.smokeResults.innerHTML = "";
  const smokeEndpoints = [
    { method: "GET", path: "/healthz", title: "healthz" },
    { method: "GET", path: "/readyz", title: "readyz" },
    { method: "GET", path: "/v1/me", title: "me" },
    { method: "GET", path: "/v1/hr/employees?page=1&page_size=2", title: "employees" },
    { method: "GET", path: "/v1/iam/permissions?page=1&page_size=5", title: "permissions" },
  ];
  for (const item of smokeEndpoints) {
    const url = new URL(item.path, normalizedBaseUrl());
    const request = { method: item.method, url: url.toString(), body: null, headers: requestHeaders(false) };
    const row = document.createElement("div");
    row.className = "smoke-row";
    row.innerHTML = `<span>${item.method}</span><span>${escapeHTML(item.title)}</span><strong>...</strong>`;
    els.smokeResults.append(row);
    try {
      const result = await sendRequest(request);
      const ok = result.status >= 200 && result.status < 400;
      row.querySelector("strong").className = ok ? "status-ok" : "status-error";
      row.querySelector("strong").textContent = String(result.status);
    } catch {
      row.querySelector("strong").className = "status-error";
      row.querySelector("strong").textContent = "ERR";
    }
  }
}

function normalizedBaseUrl() {
  const raw = els.baseUrl.value.trim() || "http://localhost:8080";
  return raw.endsWith("/") ? raw : `${raw}/`;
}

function formatResponse(result) {
  const headers = result.headers || {};
  let body = result.body || "";
  if (result.bodyEncoding === "base64") {
    body = `[base64]\n${body}`;
  } else {
    try {
      body = JSON.stringify(JSON.parse(body), null, 2);
    } catch {
      // Keep text, CSV, and YAML responses as-is.
    }
  }
  return [
    `${result.status} ${result.statusText || ""}`.trim(),
    "",
    "Headers:",
    JSON.stringify(headers, null, 2),
    "",
    "Body:",
    body || "(empty)",
  ].join("\n");
}

function setResponseState(status, metrics, body) {
  els.responseStatus.textContent = status;
  els.responseMetrics.textContent = metrics;
  els.responseBody.textContent = body;
}

function showLocalError(message) {
  els.responseStatus.innerHTML = '<span class="status-error">Client Error</span>';
  els.responseMetrics.textContent = "";
  els.responseBody.textContent = message;
}

function toCurl(request) {
  const parts = ["curl", "-i", "-X", shellQuote(request.method)];
  for (const [key, value] of Object.entries(request.headers || {})) {
    parts.push("-H", shellQuote(`${key}: ${value}`));
  }
  if (request.body != null) {
    parts.push("--data", shellQuote(request.body));
  }
  parts.push(shellQuote(request.url));
  return parts.join(" ");
}

async function copyCurl() {
  if (!state.lastCurl) {
    state.lastCurl = toCurl(buildRequestFromForm());
  }
  await navigator.clipboard.writeText(state.lastCurl);
  els.responseMetrics.textContent = "cURL copied";
}

function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}

function generateUnsignedJWT() {
  const header = { alg: "none", typ: "JWT" };
  const payload = {
    tenant_id: els.tenantId.value.trim() || "demo",
    account_id: els.accountId.value.trim() || "acct-admin",
    exp: Math.floor(Date.now() / 1000) + 86400,
  };
  els.token.value = `${base64URL(header)}.${base64URL(payload)}.`;
  saveSettings();
}

function base64URL(value) {
  const raw = JSON.stringify(value);
  const bytes = new TextEncoder().encode(raw);
  let binary = "";
  bytes.forEach(byte => {
    binary += String.fromCharCode(byte);
  });
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replaceAll("=", "");
}

function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, char => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;",
  }[char]));
}
