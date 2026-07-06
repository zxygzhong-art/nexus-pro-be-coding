#!/usr/bin/env bash
# Live workflow E2E against a running API + Keycloak.
# Usage:
#   cd nexus-pro-be
#   set -a && source .env && set +a
#   export SMOKE_ADMIN_USERNAME='admin@demo.local'
#   export SMOKE_ADMIN_PASSWORD='...'
#   export SMOKE_EMPLOYEE_USERNAME='employee@demo.local'
#   export SMOKE_EMPLOYEE_PASSWORD='...'
#   export SMOKE_AUDIT_USERNAME='audit@demo.local'
#   export SMOKE_AUDIT_PASSWORD='...'
#   ./tools/workflow-live-e2e.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE_URL="${WORKFLOW_E2E_BASE_URL:-http://localhost:8080}"

cd "$ROOT"
python3 tools/api-smoke/full_api_smoke.py \
  --base-url "$BASE_URL" \
  --skip-role-matrix \
  --quiet \
  2>&1 | rg 'workflow|submit workflow|approve workflow|PASS|FAIL|SmokeFailure' || true

python3 - <<'PY'
import json, os, sys, urllib.parse, urllib.request

def env(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        raise SystemExit(f"missing env {name}")
    return value

issuer = env("KEYCLOAK_ISSUER_URL").rstrip("/")
client_id = os.environ.get("KEYCLOAK_CLIENT_ID", "nexus-pro-connect-api")
username = env("SMOKE_ADMIN_USERNAME")
password = env("SMOKE_ADMIN_PASSWORD")
base = os.environ.get("WORKFLOW_E2E_BASE_URL", "http://localhost:8080").rstrip("/")

with urllib.request.urlopen(issuer + "/.well-known/openid-configuration") as resp:
    token_url = json.load(resp)["token_endpoint"]

form = urllib.parse.urlencode({
    "grant_type": "password",
    "client_id": client_id,
    "username": username,
    "password": password,
    "scope": "openid",
}).encode()
req = urllib.request.Request(token_url, data=form, headers={"Content-Type": "application/x-www-form-urlencoded"})
with urllib.request.urlopen(req) as resp:
    token = json.load(resp)["access_token"]

headers = {"Authorization": "Bearer " + token, "Content-Type": "application/json"}

def call(method: str, path: str, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req) as resp:
        return resp.status, json.load(resp)

status, submit = call("POST", "/v1/workflows/forms/leave-request/submit", {"payload": {"leave_type": "annual", "hours": 4, "reason": "live e2e"}})
form_id = submit["data"]["id"]
print(f"submit {status} form_id={form_id}")

status, state = call("GET", f"/v1/workflows/forms/{urllib.parse.quote(form_id)}/workflow")
print(f"workflow state {status} can_act={state['data'].get('can_act')} run_status={state['data'].get('run_status')}")

status, reviews = call("GET", "/v1/workflows/reviews")
pending = len(reviews["data"].get("pending_review") or [])
print(f"reviews {status} pending={pending}")

status, approved = call("POST", f"/v1/workflows/forms/{urllib.parse.quote(form_id)}/approve", {"reason": "live e2e ok"})
print(f"approve {status} form_status={approved['data'].get('status')}")

status, final = call("GET", f"/v1/workflows/forms/{urllib.parse.quote(form_id)}/workflow")
print(f"final state {status} run_status={final['data'].get('run_status')}")
print("workflow live e2e ok")
PY
