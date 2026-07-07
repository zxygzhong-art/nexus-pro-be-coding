#!/usr/bin/env python3
"""Provision QA test accounts for iKala Nexus (Keycloak + Postgres).

Creates a matrix of login-able test accounts with different permission sets
and account/employee states, so QA can exercise permission boundaries and
edge cases on every page.

Requirements:
  - Python 3.9+ (stdlib only)
  - `psql` CLI on PATH
  - Keycloak running with realm configured (see ops/docs/keycloak.md):
      * realm `nexus-pro`
      * client `nexus-pro-connect-api` with Direct Access Grants enabled
      * protocol mappers for user attributes `tenant_id` / `account_id`
        (this script will create the mappers automatically if missing)
  - Postgres migrated (make migrate-up)

Usage:
  export DATABASE_URL='postgres://nexus:nexus@127.0.0.1:5432/nexus_pro_be?sslmode=disable'
  export KEYCLOAK_BASE_URL='http://127.0.0.1:8080'
  ./provision_qa_accounts.py                 # provision + verify
  ./provision_qa_accounts.py --verify-only   # only run login verification
  ./provision_qa_accounts.py --print-matrix  # print the account matrix and exit

Environment variables (all optional except DATABASE_URL):
  DATABASE_URL           Postgres connection string (required unless --verify-only/--print-matrix)
  KEYCLOAK_BASE_URL      default http://127.0.0.1:8080
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

KC_BASE = os.environ.get("KEYCLOAK_BASE_URL", "http://127.0.0.1:8080").rstrip("/")
KC_REALM = os.environ.get("KEYCLOAK_REALM", "nexus-pro")
KC_ADMIN_USER = os.environ.get("KEYCLOAK_ADMIN_USER", "admin")
KC_ADMIN_PASS = os.environ.get("KEYCLOAK_ADMIN_PASS", "admin")
KC_CLIENT_ID = os.environ.get("KEYCLOAK_CLIENT_ID", "nexus-pro-connect-api")
KC_CLIENT_SECRET = os.environ.get("KEYCLOAK_CLIENT_SECRET", "")

TENANT_ID = os.environ.get("QA_TENANT_ID", "qa")
TENANT_NAME = os.environ.get("QA_TENANT_NAME", "QA Tenant")
QA_PASSWORD = os.environ.get("QA_PASSWORD", "QaTest123!")
API_BASE = os.environ.get("API_BASE_URL", "").rstrip("/")
DATABASE_URL = os.environ.get("DATABASE_URL", "")

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
    perm("workflow.form_instance", "read", "self", "workflow.forms"),
    perm("workflow.form_instance", "submit", "self", "workflow.forms"),
    perm("workflow.form_instance", "update", "self", "workflow.forms"),
    perm("workflow.form_instance", "delete", "self", "workflow.forms"),
]

PERMISSION_SETS = {
    "ps-qa-platform-admin": {
        "name": "QA Platform Admin",
        "permissions": [perm("*", "*", menu_key="workbench")] + ME_BASIC,
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
        ],
    },
    "ps-qa-attendance-manager": {
        "name": "QA Attendance Manager",
        "permissions": ME_BASIC
        + [
            perm("attendance.clock", "read", menu_key="attendance.clock"),
            perm("attendance.clock", "create", menu_key="attendance.clock"),
            perm("attendance.correction", "read", menu_key="attendance.corrections"),
            perm("attendance.correction", "approve", menu_key="attendance.corrections"),
            perm("attendance.correction", "update", menu_key="attendance.corrections"),
            perm("attendance.leave", "read", menu_key="attendance.leave"),
            perm("attendance.leave", "update", menu_key="attendance.leave"),
        ],
    },
    "ps-qa-approver": {
        "name": "QA Workflow Approver",
        "permissions": ME_BASIC
        + SELF_EMPLOYEE
        + [
            perm("workflow.form_instance", "read", menu_key="workflow.forms"),
            perm("workflow.form_instance", "approve", menu_key="workflow.forms"),
            perm("workflow.form_instance", "update", menu_key="workflow.forms"),
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
        "desc": "全权限（wildcard），所有 workspace 页面可见",
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
        "desc": "HR 权限：员工/组织页面可见，考勤/表单设计/管理员/审计不可见",
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
        "desc": "考勤管理：工時統計/打卡時間/假勤制度可见，可审批补卡",
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
        "desc": "表单审批人：待辦審核可核准/驳回/退回；同时是 qa-employee 的主管",
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
        "desc": "普通员工（self scope）：打卡/请假/提交表单，无任何 workspace 页面",
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
        "desc": "仅审计：workspace 只见操作紀錄",
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
        "desc": "仅 me.read：能登录进主页，任何业务 API 应 403，workspace 全部拦截",
    },
    {
        "key": "disabled",
        "email": f"qa-disabled@{TENANT_ID}.test",
        "name": "QA Disabled Account",
        "employee_no": "QA008",
        "permission_sets": ["ps-qa-employee"],
        "account_status": "disabled",
        "employee_status": "active",
        "expect_login": True,  # Keycloak 会发 token，但后端 API 应拒绝
        "expect_api_ok": False,
        "desc": "账号已停用：Keycloak 可取得 token，但业务 API 应拒绝（account_inactive）",
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
        "desc": "待邀请激活：同上，后端应拒绝",
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
        "desc": "边界：账号 active 但员工已离职——验证打卡/请假等操作的实际行为",
    },
    {
        "key": "kc-only",
        "email": f"qa-kc-only@{TENANT_ID}.test",
        "name": "QA Keycloak Only",
        "employee_no": "",
        "permission_sets": [],
        "account_status": None,  # 不写 DB：仅存在于 Keycloak，无 user_identities 绑定
        "employee_status": None,
        "expect_login": True,
        "expect_api_ok": False,
        "desc": "边界：Keycloak 有用户但后端无绑定，API 应 401（identity not linked）",
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
        f"""INSERT INTO org_units (id, tenant_id, code, name, parent_id, path, created_at)
VALUES ({sql_quote(ROOT_ORG_ID)}, {sql_quote(TENANT_ID)}, 'ROOT', {sql_quote(TENANT_NAME)}, '', ARRAY[{sql_quote(ROOT_ORG_ID)}], now())
ON CONFLICT (id) DO NOTHING;""",
    ]

    for ps_id, ps in PERMISSION_SETS.items():
        perms_json = json.dumps(ps["permissions"], ensure_ascii=False)
        lines.append(
            f"""INSERT INTO permission_sets (id, tenant_id, name, description, permissions, created_at)
VALUES ({sql_quote(ps_id)}, {sql_quote(TENANT_ID)}, {sql_quote(ps["name"])}, 'QA provisioned',
        {sql_quote(perms_json)}::jsonb, now())
ON CONFLICT (id) DO UPDATE SET permissions = EXCLUDED.permissions, name = EXCLUDED.name;"""
        )

    # First pass: accounts (employees reference accounts via trigger check).
    for acct in ACCOUNTS:
        if acct["account_status"] is None:
            continue  # keycloak-only edge case
        key = acct["key"]
        ps_array = ", ".join(sql_quote(p) for p in acct["permission_sets"])
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
    print("\n=== 验证登录（Keycloak ROPC" + (f" + {API_BASE}/v1/me" if API_BASE else "") + "）===")
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
    print("全部账号验证通过。")


def print_matrix():
    print(f"tenant: {TENANT_ID}   password (all users): {QA_PASSWORD}\n")
    print(f"{'email':<38} {'account_status':<15} {'employee':<12} {'permission sets':<28} 说明")
    for acct in ACCOUNTS:
        ps = ",".join(acct["permission_sets"]) or "-"
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
        die("DATABASE_URL is required")

    print(f"=== 1/4 Keycloak admin 登录（{KC_BASE}, realm={KC_REALM}）===")
    token = kc_admin_token()

    print("=== 2/4 确认 protocol mappers（tenant_id/account_id claims）===")
    kc_ensure_protocol_mappers(token)

    print("=== 3/4 创建 Keycloak 用户并设置密码 ===")
    subs = {}
    for acct in ACCOUNTS:
        sub = kc_ensure_user(token, acct)
        subs[acct["key"]] = sub
        print(f"  [keycloak] {acct['email']:<38} sub={sub}")

    print("=== 4/4 写入 Postgres（tenant/permission_sets/accounts/employees/user_identities）===")
    run_psql(build_sql(subs))
    print("  [postgres] done")

    print_matrix()
    if not args.skip_verify:
        verify_accounts()


if __name__ == "__main__":
    main()
