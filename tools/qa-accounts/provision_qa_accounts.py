#!/usr/bin/env python3
"""Provision QA test accounts for iKala Nexus (Keycloak + Postgres).

Creates a matrix of login-able test accounts with different permission sets
and account/employee states, so QA can exercise permission boundaries and
edge cases on every page.

Requirements:
  - Python 3.9+ (stdlib only)
  - Go toolchain (reuses tenantctl to seed the canonical built-in forms)
  - `psql` CLI on PATH
  - Keycloak running with realm configured (see ops/docs/keycloak.md):
      * realm `nexus-pro`
      * client `nexus-pro-connect-api` with Direct Access Grants enabled
      * protocol mappers for user attributes `tenant_id` / `account_id`
        (this script will create the mappers automatically if missing)
  - Postgres migrated (make migrate-up)

Usage:
  export DB_HOST=127.0.0.1 DB_PORT=5432 DB_USERNAME=nexus DB_PASSWORD=nexus DB_NAME=nexus_pro_be
  export KEYCLOAK_BASE_URL='http://127.0.0.1:8080/realms/nexus-pro'
  ./provision_qa_accounts.py                 # provision + verify
  ./provision_qa_accounts.py --verify-only   # only run login verification
  ./provision_qa_accounts.py --print-matrix  # print the account matrix and exit

Environment variables (all optional except DB_*):
  DB_HOST/DB_PORT/DB_USERNAME/DB_PASSWORD/DB_NAME/DB_SSLMODE
                         Postgres connection fields (required unless --verify-only/--print-matrix)
  KEYCLOAK_BASE_URL      issuer or server root; default http://127.0.0.1:8080
  KEYCLOAK_REALM         default nexus-pro
  KEYCLOAK_ADMIN_USER    master realm admin username, default admin
  KEYCLOAK_ADMIN_PASS    master realm admin password, default admin
  KEYCLOAK_CLIENT_ID     ROPC client id, default nexus-pro-connect-api
  KEYCLOAK_CLIENT_SECRET ROPC client secret (empty for public client)
  QA_TENANT_ID           default qa
  QA_TENANT_NAME         default "QA Tenant"
  QA_PASSWORD            password for every QA user, default QaTest123!
  API_BASE_URL           backend base url for /v1/me verification, e.g. http://127.0.0.1:18080
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

KC_REALM = os.environ.get("KEYCLOAK_REALM", "nexus-pro")
KC_ADMIN_USER = os.environ.get("KEYCLOAK_ADMIN_USER", "admin")
KC_ADMIN_PASS = os.environ.get("KEYCLOAK_ADMIN_PASS", "admin")
KC_CLIENT_ID = os.environ.get("KEYCLOAK_CLIENT_ID", "nexus-pro-connect-api")
KC_CLIENT_SECRET = os.environ.get("KEYCLOAK_CLIENT_SECRET", "")

TENANT_ID = os.environ.get("QA_TENANT_ID", "qa")
TENANT_NAME = os.environ.get("QA_TENANT_NAME", "QA Tenant")
QA_PASSWORD = os.environ.get("QA_PASSWORD", "QaTest123!")
API_BASE = os.environ.get("API_BASE_URL", "").rstrip("/")

def build_database_url() -> str:
    host = os.environ.get("DB_HOST", "").strip()
    user = os.environ.get("DB_USERNAME", "").strip()
    name = os.environ.get("DB_NAME", "").strip()
    if not host or not user or not name:
        return ""
    port = os.environ.get("DB_PORT", "5432").strip() or "5432"
    password = os.environ.get("DB_PASSWORD", "")
    sslmode = os.environ.get("DB_SSLMODE", "disable").strip() or "disable"
    user_enc = urllib.parse.quote(user, safe="")
    password_enc = urllib.parse.quote(password, safe="")
    return f"postgres://{user_enc}:{password_enc}@{host}:{port}/{name}?sslmode={sslmode}"


def keycloak_server_base(raw: str) -> str:
    value = raw.rstrip("/")
    marker = "/realms/"
    if marker in value:
        return value.split(marker, 1)[0]
    return value

KC_BASE_RAW = os.environ.get("KEYCLOAK_BASE_URL", "http://127.0.0.1:8080")
KC_BASE = keycloak_server_base(KC_BASE_RAW)
DATABASE_URL = build_database_url()
REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))

ROOT_ORG_ID = f"ou-{TENANT_ID}-root"

# ---------------------------------------------------------------------------
# Permission set definitions
#
# Permission JSON structure follows internal/domain/iam.go Permission struct.
# Workspace page requirements (frontend workspaceRoutes.ts):
#   overview/employees/organization/turnover -> hr.employee.read
#   attendance/clock                         -> attendance.clock.read
#   leave-policy                             -> attendance.leave.read
#   forms (design)                           -> workflow.form_template.read
#   admins                                   -> iam.permission_set_assignment.read
#   audit-log                                -> audit.log.read | audit.audit_log.read
# ---------------------------------------------------------------------------


def perm(resource: str, action: str, scope: str = "all", menu_key: str = "") -> dict:
    p = {"resource": resource, "action": action, "scope": scope}
    if menu_key:
        p["menu_key"] = menu_key
    return p


ME_BASIC = [
    perm("me", "read", menu_key="workbench"),
    perm("me", "create", menu_key="workbench"),
    perm("me", "update", menu_key="workbench"),
    perm("me", "delete", menu_key="workbench"),
]

SELF_EMPLOYEE = [
    perm("attendance.clock", "read", "self", "attendance.clock"),
    perm("attendance.clock", "create", "self", "attendance.clock"),
    perm("attendance.correction", "read", "self", "attendance.corrections"),
    perm("attendance.correction", "create", "self", "attendance.corrections"),
    perm("attendance.leave", "read", "self", "attendance.leave"),
    perm("attendance.leave", "create", "self", "attendance.leave"),
    perm("workflow.form_template", "read", "all", "workflow.forms"),
    perm("workflow.form_instance", "read", "self", "workflow.instances"),
    perm("workflow.form_instance", "submit", "self", "workflow.instances"),
    perm("workflow.form_instance", "update", "self", "workflow.instances"),
    perm("workflow.form_instance", "delete", "self", "workflow.instances"),
]

# Wildcard authorizes APIs but page navigation is intentionally projected from
# explicit primary read grants. Mirror the real tenant-admin shape so the QA
# superadmin can exercise every management page without weakening that rule.
SUPERADMIN_PAGE_GRANTS = [
    perm("hr.employee", "read", "all", "workspace.overview"),
    perm("hr.employee", "read", "all", "hr.employees"),
    perm("hr.org_unit", "read", "all", "hr.org_units"),
    perm("hr.position", "read", "all", "hr.positions"),
    perm("hr.employee", "read", "all", "hr.organization"),
    perm("hr.employee", "read", "all", "hr.turnover"),
    perm("attendance.clock", "read", "all", "attendance.overview"),
    perm("attendance.clock", "read", "all", "attendance.clock"),
    perm("attendance.leave", "read", "all", "attendance.leave_policy"),
    perm("workflow.form_template", "read", "all", "workflow.forms"),
    perm("workflow.form_template", "create", "all", "workflow.forms"),
    perm("agent.model", "read", "all", "agents.models"),
    perm("agent.knowledge_base", "read", "all", "agents.knowledge_bases"),
    perm("agent.tool", "read", "all", "agents.tools"),
    perm("agent.definition", "read", "all", "agents.definitions"),
    perm("agent.usage", "read", "all", "agents.usage"),
    perm("iam.permission_set_assignment", "read", "all", "iam.members"),
    perm("iam.user_group", "read", "all", "iam.user_groups"),
    perm("iam.permission_set", "read", "all", "iam.permission_sets"),
    perm("iam.permission_set_assignment", "read", "all", "iam.assignments"),
    perm("iam.assumable_role", "read", "all", "iam.assumable_roles"),
    perm("iam.data_scope", "read", "all", "iam.policies"),
    perm("audit.audit_log", "read", "all", "audit.logs"),
    perm("attendance.leave", "read", "all", "attendance.leave"),
    perm("workflow.form_instance", "read", "all", "workflow.instances"),
    perm("agent.run", "read", "all", "agents.runs"),
]

PERMISSION_SETS = {
    "ps-qa-platform-admin": {
        "name": "QA Platform Admin",
        "permissions": [perm("*", "*", menu_key="workbench")] + ME_BASIC + SUPERADMIN_PAGE_GRANTS,
    },
    "ps-qa-hr-admin": {
        "name": "QA HR Admin",
        "permissions": ME_BASIC
        + [
            perm("hr.employee", a, menu_key="hr.employees")
            for a in [
                "read", "create", "update", "delete", "export", "import",
                "invite", "update_status", "status_transition",
            ]
        ]
        + [
            perm("hr.org_unit", "read", menu_key="hr.org_units"),
            perm("hr.org_unit", "create", menu_key="hr.org_units"),
            perm("hr.org_unit", "update", menu_key="hr.org_units"),
            # These page associations reuse the existing employee read grant;
            # they expose no additional API action or data scope.
            perm("hr.employee", "read", "all", "hr.organization"),
            perm("hr.employee", "read", "all", "hr.turnover"),
        ],
    },
    "ps-qa-attendance-manager": {
        "name": "QA Attendance Manager",
        "permissions": ME_BASIC
        + [
            # The same read grants project the manager into each workspace page;
            # they do not add a broader API action or data scope.
            perm("attendance.clock", "read", menu_key="attendance.overview"),
            perm("attendance.clock", "read", menu_key="attendance.clock"),
            perm("attendance.clock", "create", menu_key="attendance.clock"),
            perm("attendance.correction", "read", menu_key="attendance.corrections"),
            perm("attendance.correction", "approve", menu_key="attendance.corrections"),
            perm("attendance.correction", "update", menu_key="attendance.corrections"),
            perm("attendance.leave", "read", menu_key="attendance.leave_policy"),
            perm("attendance.leave", "read", menu_key="attendance.leave"),
            perm("attendance.leave", "update", menu_key="attendance.leave"),
        ],
    },
    "ps-qa-approver": {
        "name": "QA Workflow Approver",
        "permissions": ME_BASIC
        + SELF_EMPLOYEE
        + [
            perm("workflow.form_instance", "read", menu_key="workflow.instances"),
            perm("workflow.form_instance", "approve", menu_key="workflow.instances"),
            perm("workflow.form_instance", "update", menu_key="workflow.instances"),
        ],
    },
    "ps-qa-employee": {
        "name": "QA Employee (self scope)",
        "permissions": ME_BASIC + SELF_EMPLOYEE + [perm("hr.employee", "read", "self", "hr.employees")],
    },
    "ps-qa-audit": {
        "name": "QA Audit Reader",
        "permissions": [
            perm("me", "read", menu_key="workbench"),
            perm("audit.log", "read", menu_key="audit"),
            perm("audit.audit_log", "read", menu_key="audit"),
        ],
    },
    "ps-qa-noperm": {
        "name": "QA Minimal (me.read only)",
        "permissions": [perm("me", "read", menu_key="workbench")],
    },
}


def permission_set_id(base_id: str) -> str:
    """Return a globally unique permission-set ID for the selected QA tenant."""
    if TENANT_ID == "qa":
        return base_id
    return f"{base_id}-{TENANT_ID}"

# ---------------------------------------------------------------------------
# Account matrix
# ---------------------------------------------------------------------------

ACCOUNTS = [
    {
        "key": "superadmin",
        "email": f"qa-superadmin@{TENANT_ID}.test",
        "name": "QA Super Admin",
        "employee_no": "QA001",
        "permission_sets": ["ps-qa-platform-admin"],
        "account_status": "active",
        "employee_status": "active",
        "expect_login": True,
        "desc": "全權限（wildcard），所有 workspace 頁面可見",
    },
    {
        "key": "hr",
        "email": f"qa-hr@{TENANT_ID}.test",
        "name": "QA HR Admin",
        "employee_no": "QA002",
        "permission_sets": ["ps-qa-hr-admin"],
        "account_status": "active",
        "employee_status": "active",
        "expect_login": True,
        "desc": "HR 權限：員工/組織頁面可見，考勤/表單設計/管理員/審計不可見",
    },
    {
        "key": "attendance",
        "email": f"qa-attendance@{TENANT_ID}.test",
        "name": "QA Attendance Manager",
        "employee_no": "QA003",
        "permission_sets": ["ps-qa-attendance-manager"],
        "account_status": "active",
        "employee_status": "active",
        "expect_login": True,
        "desc": "考勤管理：工時統計/打卡時間/假勤制度可見，可審批補卡",
    },
    {
        "key": "approver",
        "email": f"qa-approver@{TENANT_ID}.test",
        "name": "QA Approver",
        "employee_no": "QA004",
        "permission_sets": ["ps-qa-approver"],
        "account_status": "active",
        "employee_status": "active",
        "expect_login": True,
        "desc": "表單審批人：待辦審核可覈準/駁回/退回；同時是 qa-employee 的主管",
    },
    {
        "key": "employee",
        "email": f"qa-employee@{TENANT_ID}.test",
        "name": "QA Employee",
        "employee_no": "QA005",
        "permission_sets": ["ps-qa-employee"],
        "account_status": "active",
        "employee_status": "active",
        "manager_key": "approver",
        "expect_login": True,
        "desc": "普通員工（self scope）：打卡/請假/提交表單，無任何 workspace 頁面",
    },
    {
        "key": "audit",
        "email": f"qa-audit@{TENANT_ID}.test",
        "name": "QA Audit Reader",
        "employee_no": "QA006",
        "permission_sets": ["ps-qa-audit"],
        "account_status": "active",
        "employee_status": "active",
        "expect_login": True,
        "desc": "僅審計：workspace 只見操作紀錄",
    },
    {
        "key": "noperm",
        "email": f"qa-noperm@{TENANT_ID}.test",
        "name": "QA No Permission",
        "employee_no": "QA007",
        "permission_sets": ["ps-qa-noperm"],
        "account_status": "active",
        "employee_status": "active",
        "expect_login": True,
        "desc": "僅 me.read：能登錄進主頁，任何業務 API 應 403，workspace 全部攔截",
    },
    {
        "key": "disabled",
        "email": f"qa-disabled@{TENANT_ID}.test",
        "name": "QA Disabled Account",
        "employee_no": "QA008",
        "permission_sets": ["ps-qa-employee"],
        "account_status": "disabled",
        "employee_status": "active",
        "expect_login": True,  # Keycloak 會發 token，但後端 API 應拒絕
        "expect_api_ok": False,
        "desc": "賬號已停用：Keycloak 可取得 token，但業務 API 應拒絕（account_inactive）",
    },
    {
        "key": "pending",
        "email": f"qa-pending@{TENANT_ID}.test",
        "name": "QA Pending Invite",
        "employee_no": "QA009",
        "permission_sets": ["ps-qa-employee"],
        "account_status": "pending_invite",
        "employee_status": "onboarding",
        "expect_login": True,
        "expect_api_ok": False,
        "desc": "待邀請激活：同上，後端應拒絕",
    },
    {
        "key": "resigned",
        "email": f"qa-resigned@{TENANT_ID}.test",
        "name": "QA Resigned Employee",
        "employee_no": "QA010",
        "permission_sets": ["ps-qa-employee"],
        "account_status": "active",
        "employee_status": "resigned",
        "expect_login": True,
        "desc": "邊界：賬號 active 但員工已離職——驗證打卡/請假等操作的實際行爲",
    },
    {
        "key": "kc-only",
        "email": f"qa-kc-only@{TENANT_ID}.test",
        "name": "QA Keycloak Only",
        "employee_no": "",
        "permission_sets": [],
        "account_status": None,  # 不寫 DB：僅存在於 Keycloak，無 user_identities 綁定
        "employee_status": None,
        "expect_login": True,
        "expect_api_ok": False,
        "desc": "邊界：Keycloak 有用戶但後端無綁定，API 應 401（identity not linked）",
    },
]


def account_id(key: str) -> str:
    return f"acct-{TENANT_ID}-{key}"


def employee_id(key: str) -> str:
    return f"emp-{TENANT_ID}-{key}"


# ---------------------------------------------------------------------------
# HTTP helpers
# ---------------------------------------------------------------------------


def http(method: str, url: str, body=None, headers=None, form=False):
    data = None
    headers = dict(headers or {})
    if body is not None:
        if form:
            data = urllib.parse.urlencode(body).encode()
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        else:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read()
            return resp.status, json.loads(raw) if raw.strip() else None, dict(resp.headers)
    except urllib.error.HTTPError as e:
        raw = e.read()
        try:
            parsed = json.loads(raw) if raw.strip() else None
        except json.JSONDecodeError:
            parsed = raw.decode(errors="replace")
        return e.code, parsed, dict(e.headers)


def die(msg: str):
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(1)


# ---------------------------------------------------------------------------
# Keycloak admin operations
# ---------------------------------------------------------------------------


def kc_admin_token() -> str:
    status, body, _ = http(
        "POST",
        f"{KC_BASE}/realms/master/protocol/openid-connect/token",
        {"grant_type": "password", "client_id": "admin-cli",
         "username": KC_ADMIN_USER, "password": KC_ADMIN_PASS},
        form=True,
    )
    if status != 200:
        die(f"Keycloak admin login failed ({status}): {body}")
    return body["access_token"]


def kc_headers(token: str) -> dict:
    return {"Authorization": f"Bearer {token}"}


def kc_find_user(token: str, email: str):
    q = urllib.parse.urlencode({"email": email, "exact": "true"})
    status, body, _ = http("GET", f"{KC_BASE}/admin/realms/{KC_REALM}/users?{q}", headers=kc_headers(token))
    if status != 200:
        die(f"Keycloak user lookup failed ({status}): {body}")
    return body[0] if body else None


def kc_ensure_user(token: str, acct: dict) -> str:
    """Create or update the Keycloak user; returns the user id (OIDC sub)."""
    attributes = {"tenant_id": [TENANT_ID]}
    if acct["account_status"] is not None:
        attributes["account_id"] = [account_id(acct["key"])]
        attributes["employee_id"] = [employee_id(acct["key"])]
        if acct["employee_no"]:
            attributes["employee_no"] = [acct["employee_no"]]
    payload = {
        "username": acct["email"],
        "email": acct["email"],
        "firstName": acct["name"],
        "lastName": "QA",
        "enabled": True,
        "emailVerified": True,
        "attributes": attributes,
    }
    existing = kc_find_user(token, acct["email"])
    if existing:
        uid = existing["id"]
        status, body, _ = http("PUT", f"{KC_BASE}/admin/realms/{KC_REALM}/users/{uid}",
                               payload, headers=kc_headers(token))
        if status not in (204, 200):
            die(f"Keycloak user update failed for {acct['email']} ({status}): {body}")
    else:
        status, body, hdrs = http("POST", f"{KC_BASE}/admin/realms/{KC_REALM}/users",
                                  payload, headers=kc_headers(token))
        if status != 201:
            die(f"Keycloak user create failed for {acct['email']} ({status}): {body}")
        uid = hdrs.get("Location", "").rstrip("/").rsplit("/", 1)[-1]
        if not uid:
            uid = kc_find_user(token, acct["email"])["id"]
    # Set a permanent password so ROPC login works.
    status, body, _ = http(
        "PUT",
        f"{KC_BASE}/admin/realms/{KC_REALM}/users/{uid}/reset-password",
        {"type": "password", "value": QA_PASSWORD, "temporary": False},
        headers=kc_headers(token),
    )
    if status not in (204, 200):
        die(f"Keycloak set password failed for {acct['email']} ({status}): {body}")
    return uid


def kc_ensure_protocol_mappers(token: str):
    """Ensure the ROPC client maps tenant_id/account_id user attributes into tokens."""
    q = urllib.parse.urlencode({"clientId": KC_CLIENT_ID})
    status, clients, _ = http("GET", f"{KC_BASE}/admin/realms/{KC_REALM}/clients?{q}", headers=kc_headers(token))
    if status != 200 or not clients:
        die(f"client {KC_CLIENT_ID} not found in realm {KC_REALM}; configure it first (ops/docs/keycloak.md)")
    cid = clients[0]["id"]
    status, mappers, _ = http(
        "GET", f"{KC_BASE}/admin/realms/{KC_REALM}/clients/{cid}/protocol-mappers/models",
        headers=kc_headers(token),
    )
    existing = {m["name"] for m in (mappers or [])}
    for claim in ("tenant_id", "account_id", "employee_id", "employee_no"):
        if claim in existing:
            continue
        payload = {
            "name": claim,
            "protocol": "openid-connect",
            "protocolMapper": "oidc-usermodel-attribute-mapper",
            "config": {
                "user.attribute": claim,
                "claim.name": claim,
                "jsonType.label": "String",
                "id.token.claim": "true",
                "access.token.claim": "true",
                "userinfo.token.claim": "true",
            },
        }
        status, body, _ = http(
            "POST", f"{KC_BASE}/admin/realms/{KC_REALM}/clients/{cid}/protocol-mappers/models",
            payload, headers=kc_headers(token),
        )
        if status not in (201, 204):
            die(f"failed to create protocol mapper {claim} ({status}): {body}")
        print(f"  [keycloak] created protocol mapper: {claim}")


# ---------------------------------------------------------------------------
# SQL generation
# ---------------------------------------------------------------------------


def sql_quote(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def build_sql(subs: dict) -> str:
    """subs: account key -> keycloak sub uuid"""
    lines = [
        "BEGIN;",
        f"SET LOCAL app.tenant_id = {sql_quote(TENANT_ID)};",
        # Tenant + root org unit
        f"""INSERT INTO tenants (id, name, created_at)
VALUES ({sql_quote(TENANT_ID)}, {sql_quote(TENANT_NAME)}, now())
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;""",
        f"""INSERT INTO org_units (id, tenant_id, code, name, parent_id, path, created_at, updated_at)
VALUES ({sql_quote(ROOT_ORG_ID)}, {sql_quote(TENANT_ID)}, 'ROOT', {sql_quote(TENANT_NAME)}, '', ARRAY[{sql_quote(ROOT_ORG_ID)}], now(), now())
ON CONFLICT (id) DO NOTHING;""",
    ]

    for ps_id, ps in PERMISSION_SETS.items():
        stored_ps_id = permission_set_id(ps_id)
        perms_json = json.dumps(ps["permissions"], ensure_ascii=False)
        lines.append(
            f"""INSERT INTO permission_sets (id, tenant_id, name, description, permissions, created_at)
VALUES ({sql_quote(stored_ps_id)}, {sql_quote(TENANT_ID)}, {sql_quote(ps["name"])}, 'QA provisioned',
        {sql_quote(perms_json)}::jsonb, now())
ON CONFLICT (id) DO UPDATE SET permissions = EXCLUDED.permissions, name = EXCLUDED.name;"""
        )

    # First pass: accounts (employees reference accounts via trigger check).
    for acct in ACCOUNTS:
        if acct["account_status"] is None:
            continue  # keycloak-only edge case
        key = acct["key"]
        ps_array = ", ".join(sql_quote(permission_set_id(p)) for p in acct["permission_sets"])
        lines.append(
            f"""INSERT INTO accounts (id, tenant_id, display_name, email, employee_id, status,
        direct_permission_set_ids, created_at)
VALUES ({sql_quote(account_id(key))}, {sql_quote(TENANT_ID)}, {sql_quote(acct["name"])},
        {sql_quote(acct["email"])}, {sql_quote(employee_id(key))}, {sql_quote(acct["account_status"])},
        ARRAY[{ps_array}]::text[], now())
ON CONFLICT (id) DO UPDATE SET
    status = EXCLUDED.status,
    direct_permission_set_ids = EXCLUDED.direct_permission_set_ids,
    email = EXCLUDED.email;"""
        )

    # Second pass: employees (so manager FK targets exist within this txn order).
    ordered = sorted(
        [a for a in ACCOUNTS if a["account_status"] is not None],
        key=lambda a: 0 if not a.get("manager_key") else 1,
    )
    for acct in ordered:
        key = acct["key"]
        manager = acct.get("manager_key")
        manager_sql = sql_quote(employee_id(manager)) if manager else "NULL"
        lines.append(
            f"""INSERT INTO employees (id, tenant_id, employee_no, name, company_email, org_unit_id,
        account_id, manager_employee_id, position, category, status, employment_status,
        hire_date, basic_info, created_at, updated_at)
VALUES ({sql_quote(employee_id(key))}, {sql_quote(TENANT_ID)}, {sql_quote(acct["employee_no"])},
        {sql_quote(acct["name"])}, {sql_quote(acct["email"])}, {sql_quote(ROOT_ORG_ID)},
        {sql_quote(account_id(key))}, {manager_sql}, 'QA Tester', 'full_time',
        {sql_quote(acct["employee_status"])}, {sql_quote(acct["employee_status"])},
        now() - interval '180 days',
        jsonb_build_object('name', {sql_quote(acct["name"])}, 'company_email', {sql_quote(acct["email"])}),
        now(), now())
ON CONFLICT (id) DO UPDATE SET
    status = EXCLUDED.status,
    employment_status = EXCLUDED.employment_status,
    manager_employee_id = EXCLUDED.manager_employee_id;"""
        )

    # Identity bindings: Keycloak sub -> account.
    for acct in ACCOUNTS:
        if acct["account_status"] is None:
            continue
        key = acct["key"]
        sub = subs[key]
        lines.append(
            f"""INSERT INTO user_identities (id, tenant_id, account_id, provider, subject, email, created_at)
VALUES ({sql_quote('uid-' + TENANT_ID + '-' + key)}, {sql_quote(TENANT_ID)}, {sql_quote(account_id(key))},
        'keycloak', {sql_quote(sub)}, {sql_quote(acct["email"])}, now())
ON CONFLICT (tenant_id, provider, subject) DO UPDATE SET account_id = EXCLUDED.account_id;"""
        )

    # Bump permission version so any cached authz snapshots invalidate.
    lines.append(
        f"""INSERT INTO authz_permission_versions (tenant_id, version, updated_at)
VALUES ({sql_quote(TENANT_ID)}, 1, now())
ON CONFLICT (tenant_id) DO UPDATE SET version = authz_permission_versions.version + 1, updated_at = now();"""
    )
    lines.append("COMMIT;")
    return "\n\n".join(lines)


def run_psql(sql: str):
    proc = subprocess.run(
        ["psql", DATABASE_URL, "-v", "ON_ERROR_STOP=1"],
        input=sql, capture_output=True, text=True,
    )
    if proc.returncode != 0:
        print(proc.stdout)
        die(f"psql failed:\n{proc.stderr}")


def ensure_default_form_templates():
    """Backfill canonical built-in forms without overwriting tenant customizations."""
    proc = subprocess.run(
        [
            "go",
            "run",
            "./cmd/tenantctl",
            "ensure-default-form-templates",
            "--tenant-id",
            TENANT_ID,
            "--database-url",
            DATABASE_URL,
        ],
        cwd=REPO_ROOT,
        text=True,
        capture_output=True,
    )
    if proc.returncode != 0:
        die(proc.stderr.strip() or "default form template backfill failed")
    try:
        result = json.loads(proc.stdout)
    except json.JSONDecodeError:
        die("tenantctl returned an invalid default form template result")
    print(f"  [forms] defaults ready (created={result.get('created', 0)})")


# ---------------------------------------------------------------------------
# Verification
# ---------------------------------------------------------------------------


def ropc_login(email: str):
    body = {
        "grant_type": "password",
        "client_id": KC_CLIENT_ID,
        "username": email,
        "password": QA_PASSWORD,
        "scope": "openid profile email",
    }
    if KC_CLIENT_SECRET:
        body["client_secret"] = KC_CLIENT_SECRET
    return http("POST", f"{KC_BASE}/realms/{KC_REALM}/protocol/openid-connect/token", body, form=True)


def verify_accounts():
    print("\n=== 驗證登錄（Keycloak ROPC" + (f" + {API_BASE}/v1/me" if API_BASE else "") + "）===")
    failures = 0
    for acct in ACCOUNTS:
        status, body, _ = ropc_login(acct["email"])
        token_ok = status == 200
        line = f"  {acct['email']:<38} token={'OK' if token_ok else f'FAIL({status})'}"
        if token_ok and API_BASE:
            access = body["access_token"]
            api_status, _, _ = http("GET", f"{API_BASE}/v1/me", headers={"Authorization": f"Bearer {access}"})
            expect_ok = acct.get("expect_api_ok", True)
            api_ok = (api_status == 200) == expect_ok
            line += f"  /v1/me={api_status} (expect {'200' if expect_ok else '401/403'}) {'OK' if api_ok else 'UNEXPECTED'}"
            if not api_ok:
                failures += 1
        elif not token_ok:
            if acct["expect_login"]:
                failures += 1
                line += "  <- expected login to succeed"
        print(line)
    if failures:
        die(f"{failures} account(s) behaved unexpectedly")
    print("全部賬號驗證通過。")


def print_matrix():
    print(f"tenant: {TENANT_ID}   password (all users): {QA_PASSWORD}\n")
    print(f"{'email':<38} {'account_status':<15} {'employee':<12} {'permission sets':<28} 說明")
    for acct in ACCOUNTS:
        ps = ",".join(permission_set_id(p) for p in acct["permission_sets"]) or "—"
        print(f"{acct['email']:<38} {str(acct['account_status']):<15} {str(acct['employee_status']):<12} {ps:<28} {acct['desc']}")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main():
    parser = argparse.ArgumentParser(description="Provision QA test accounts (Keycloak + Postgres)")
    parser.add_argument("--verify-only", action="store_true", help="only run login verification")
    parser.add_argument("--print-matrix", action="store_true", help="print account matrix and exit")
    parser.add_argument("--skip-verify", action="store_true", help="provision without verification")
    args = parser.parse_args()

    if args.print_matrix:
        print_matrix()
        return
    if args.verify_only:
        verify_accounts()
        return

    if not DATABASE_URL:
        die("DB_HOST, DB_USERNAME, and DB_NAME are required")

    print(f"=== 1/5 Keycloak admin 登錄（{KC_BASE}, realm={KC_REALM}）===")
    token = kc_admin_token()

    print("=== 2/5 確認 protocol mappers（tenant_id/account_id claims）===")
    kc_ensure_protocol_mappers(token)

    print("=== 3/5 創建 Keycloak 用戶並設置密碼 ===")
    subs = {}
    for acct in ACCOUNTS:
        sub = kc_ensure_user(token, acct)
        subs[acct["key"]] = sub
        print(f"  [keycloak] {acct['email']:<38} sub={sub}")

    print("=== 4/5 寫入 Postgres（tenant/permission_sets/accounts/employees/user_identities）===")
    run_psql(build_sql(subs))
    print("  [postgres] done")

    print("=== 5/5 補齊內建表單模板（保留既有自定義）===")
    ensure_default_form_templates()

    print_matrix()
    if not args.skip_verify:
        verify_accounts()


if __name__ == "__main__":
    main()
