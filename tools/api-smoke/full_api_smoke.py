#!/usr/bin/env python3
"""Run a full HTTP smoke pass over the public OpenAPI routes.

The default mode starts the API with the in-memory repository, demo seed data,
and header-based request context. Pass --base-url to run against an already
started server instead.
"""

from __future__ import annotations

import argparse
import base64
import contextlib
import dataclasses
import json
import os
import pathlib
import socket
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Callable


ROOT = pathlib.Path(__file__).resolve().parents[2]
ADMIN_HEADERS = {"X-Tenant-ID": "demo", "X-Account-ID": "acct-admin"}
EMPLOYEE_HEADERS = {"X-Tenant-ID": "demo", "X-Account-ID": "acct-employee"}
AUDIT_HEADERS = {"X-Tenant-ID": "demo", "X-Account-ID": "acct-audit"}
APPROVAL_HEADER = {"X-Approval-Confirmed": "true"}
DEFAULT_KEYCLOAK_CLIENT_ID = "nexus-pro-connect-api"
DEFAULT_ROLE_ACCOUNTS = {
    "admin": ("demo", "acct-admin"),
    "employee": ("demo", "acct-employee"),
    "audit": ("demo", "acct-audit"),
}


JsonFactory = Callable[[dict[str, Any]], Any]
PathFactory = Callable[[dict[str, Any]], str]
CheckFunc = Callable[["HTTPResult", dict[str, Any]], None]
CaptureFunc = Callable[["HTTPResult", dict[str, Any]], None]


@dataclasses.dataclass
class HTTPResult:
    status: int
    headers: dict[str, str]
    body: bytes
    json_body: Any | None

    @property
    def text(self) -> str:
        return self.body.decode("utf-8", errors="replace")


@dataclasses.dataclass
class Case:
    name: str
    method: str
    path: str | PathFactory
    expected: int | tuple[int, ...]
    route_key: str | None = None
    auth: str | None = "admin"
    json_body: Any | JsonFactory | None = None
    raw_body: bytes | str | None = None
    content_type: str | None = "application/json"
    headers: dict[str, str] = dataclasses.field(default_factory=dict)
    check: CheckFunc | None = None
    capture: CaptureFunc | None = None


@dataclasses.dataclass
class MatrixCase:
    name: str
    method: str
    path: str | PathFactory
    expected: dict[str, int | tuple[int, ...]]
    json_body: Any | JsonFactory | None = None
    raw_body: bytes | str | None = None
    content_type: str | None = "application/json"
    headers: dict[str, str] = dataclasses.field(default_factory=dict)


@dataclasses.dataclass(frozen=True)
class AuthProfile:
    name: str
    tenant_id: str
    account_id: str
    headers: dict[str, str]


@dataclasses.dataclass(frozen=True)
class LoginProfile:
    name: str
    username: str
    password: str
    tenant_id: str
    account_id: str


class SmokeFailure(Exception):
    pass


def main() -> int:
    args = parse_args()
    base_url = args.base_url.rstrip("/") if args.base_url else ""
    server: subprocess.Popen[str] | None = None
    logs: list[str] = []

    try:
        if not base_url:
            port = free_port()
            base_url = f"http://127.0.0.1:{port}"
            server, logs = start_server(port, args)
        run_cases(base_url, args)
        return 0
    except SmokeFailure as exc:
        print(f"\nFAIL: {exc}", file=sys.stderr)
        if logs:
            print("\nRecent server output:", file=sys.stderr)
            for line in logs[-80:]:
                print(line.rstrip(), file=sys.stderr)
        return 1
    finally:
        if server is not None:
            stop_server(server)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Full HTTP smoke test for nexus-pro-be APIs.")
    parser.add_argument("--base-url", help="Use an existing server instead of starting go run ./cmd/api.")
    parser.add_argument("--start-timeout", type=float, default=90.0, help="Seconds to wait for a started server.")
    parser.add_argument("--auth-mode", choices=("header", "keycloak"), default="header", help="Use header context or real Keycloak password-grant login.")
    parser.add_argument("--tenant", default="demo", help="Tenant id for the default request context.")
    parser.add_argument("--account", default="acct-admin", help="Account id for the default request context.")
    parser.add_argument("--skip-role-matrix", action="store_true", help="Skip the multi-role HTTP authorization matrix.")
    parser.add_argument("--keycloak-issuer-url", default=env_first("SMOKE_KEYCLOAK_ISSUER_URL", "KEYCLOAK_ISSUER_URL"), help="OIDC issuer URL for real login mode.")
    parser.add_argument("--keycloak-token-url", default=env_first("SMOKE_KEYCLOAK_TOKEN_URL"), help="Token endpoint override for real login mode.")
    parser.add_argument("--keycloak-client-id", default=env_first("SMOKE_KEYCLOAK_CLIENT_ID", "KEYCLOAK_CLIENT_ID", fallback=DEFAULT_KEYCLOAK_CLIENT_ID), help="OIDC client id for real login mode.")
    parser.add_argument("--keycloak-client-secret", default=env_first("SMOKE_KEYCLOAK_CLIENT_SECRET", "KEYCLOAK_CLIENT_SECRET"), help="Optional OIDC client secret.")
    parser.add_argument("--keycloak-scope", default=env_first("SMOKE_KEYCLOAK_SCOPE", fallback="openid"), help="OIDC scope for password-grant login.")
    parser.add_argument(
        "--login-profile",
        action="append",
        default=[],
        metavar="ROLE=USERNAME:PASSWORD[:TENANT:ACCOUNT]",
        help="Real-login profile. Repeat for admin/employee/audit, or use SMOKE_<ROLE>_USERNAME/PASSWORD env vars.",
    )
    parser.add_argument("--quiet", action="store_true", help="Only print failures and summary.")
    return parser.parse_args()


def start_server(port: int, args: argparse.Namespace) -> tuple[subprocess.Popen[str], list[str]]:
    env = os.environ.copy()
    for key in ("DATABASE_URL", "REDIS_ADDR", "OPENFGA_API_URL", "OPENFGA_STORE_ID"):
        env.pop(key, None)
    if args.auth_mode != "keycloak":
        env.pop("KEYCLOAK_ISSUER_URL", None)
        env.pop("KEYCLOAK_CLIENT_ID", None)
    env.update(
        {
            "APP_ENV": "development",
            "HTTP_ADDR": f"127.0.0.1:{port}",
            "SEED_DEMO": "true",
            "ALLOW_HEADER_CONTEXT": "false" if args.auth_mode == "keycloak" else "true",
            "ALLOW_DEMO_CONTEXT": "false",
            "OTEL_ENABLED": "false",
            "LOG_LEVEL": "warn",
            "GOCACHE": str(ROOT / ".gocache"),
        }
    )
    if args.auth_mode == "keycloak":
        if not args.keycloak_issuer_url:
            raise SmokeFailure("--keycloak-issuer-url or SMOKE_KEYCLOAK_ISSUER_URL is required in keycloak mode")
        env["KEYCLOAK_ISSUER_URL"] = args.keycloak_issuer_url
        env["KEYCLOAK_CLIENT_ID"] = args.keycloak_client_id
    proc = subprocess.Popen(
        ["go", "run", "./cmd/api"],
        cwd=ROOT,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        bufsize=1,
    )
    logs: list[str] = []

    def collect() -> None:
        assert proc.stdout is not None
        for line in proc.stdout:
            logs.append(line)

    threading.Thread(target=collect, daemon=True).start()
    deadline = time.time() + args.start_timeout
    last_error = ""
    while time.time() < deadline:
        if proc.poll() is not None:
            raise SmokeFailure(f"server exited while starting with code {proc.returncode}")
        try:
            result = request(f"http://127.0.0.1:{port}", "GET", "/healthz", headers={})
            if result.status == 200:
                return proc, logs
        except Exception as exc:  # noqa: BLE001 - include last startup error in the final failure.
            last_error = str(exc)
        time.sleep(0.25)
    raise SmokeFailure(f"server did not become healthy within {args.start_timeout:.0f}s; last error: {last_error}")


def stop_server(proc: subprocess.Popen[str]) -> None:
    if proc.poll() is not None:
        return
    proc.terminate()
    with contextlib.suppress(subprocess.TimeoutExpired):
        proc.wait(timeout=5)
        return
    proc.kill()
    proc.wait(timeout=5)


def free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def run_cases(base_url: str, args: argparse.Namespace) -> None:
    auth_profiles = build_auth_profiles(args)
    context: dict[str, Any] = {
        "suffix": str(int(time.time() * 1000)),
        "tenant": args.tenant,
        "account": args.account,
        "auth_profiles": auth_profiles,
    }
    failures: list[str] = []
    cases = build_cases()
    matrix_cases = [] if args.skip_role_matrix else build_role_matrix_cases()
    covered = {case.route_key for case in cases if case.route_key}
    missing = sorted(openapi_route_keys() - covered)
    if missing:
        raise SmokeFailure("script is missing OpenAPI routes: " + ", ".join(missing))

    start = time.time()
    total_checks = len(cases) + sum(len(case.expected) for case in matrix_cases)
    current = 0
    for case in cases:
        current += 1
        try:
            result = run_case(base_url, case, context, args)
            if not args.quiet:
                print(f"PASS {current:02d}/{total_checks:02d} {case.name} -> {result.status}")
        except Exception as exc:  # noqa: BLE001 - keep running to show all failing endpoints.
            failures.append(f"{case.name}: {exc}")
            if not args.quiet:
                print(f"FAIL {current:02d}/{total_checks:02d} {case.name}: {exc}")

    for matrix_case in matrix_cases:
        for role in matrix_case.expected:
            current += 1
            label = f"{matrix_case.name} [{role}]"
            try:
                result = run_matrix_case(base_url, matrix_case, role, context, args)
                if not args.quiet:
                    print(f"PASS {current:02d}/{total_checks:02d} {label} -> {result.status}")
            except Exception as exc:  # noqa: BLE001 - keep running to show the full matrix.
                failures.append(f"{label}: {exc}")
                if not args.quiet:
                    print(f"FAIL {current:02d}/{total_checks:02d} {label}: {exc}")

    elapsed = time.time() - start
    print(f"\nSummary: {total_checks - len(failures)} passed, {len(failures)} failed, {elapsed:.1f}s")
    print(f"OpenAPI route coverage: {len(covered)}/{len(openapi_route_keys())}")
    if not args.skip_role_matrix:
        print(f"Role matrix coverage: {sum(len(case.expected) for case in matrix_cases)} checks across {len(auth_profiles)} profiles")
    if failures:
        raise SmokeFailure("\n".join(failures))


def run_case(base_url: str, case: Case, context: dict[str, Any], args: argparse.Namespace) -> HTTPResult:
    path = case.path(context) if callable(case.path) else case.path
    headers = headers_for(case.auth, args, context)
    headers.update(case.headers)
    body: bytes | None = None
    if case.raw_body is not None:
        body = case.raw_body.encode("utf-8") if isinstance(case.raw_body, str) else case.raw_body
    elif case.json_body is not None:
        payload = case.json_body(context) if callable(case.json_body) else case.json_body
        body = json.dumps(payload, ensure_ascii=True).encode("utf-8")
    if body is not None and case.content_type:
        headers.setdefault("Content-Type", case.content_type)

    result = request(base_url, case.method, path, headers=headers, body=body)
    expected = case.expected if isinstance(case.expected, tuple) else (case.expected,)
    if result.status not in expected:
        raise SmokeFailure(
            f"expected HTTP {expected}, got {result.status}; body={truncate(result.text)}"
        )
    if case.check is not None:
        case.check(result, context)
    if case.capture is not None:
        case.capture(result, context)
    return result


def run_matrix_case(
    base_url: str,
    case: MatrixCase,
    role: str,
    context: dict[str, Any],
    args: argparse.Namespace,
) -> HTTPResult:
    path = case.path(context) if callable(case.path) else case.path
    headers = headers_for(role, args, context)
    headers.update(case.headers)
    body: bytes | None = None
    if case.raw_body is not None:
        body = case.raw_body.encode("utf-8") if isinstance(case.raw_body, str) else case.raw_body
    elif case.json_body is not None:
        payload = case.json_body(context) if callable(case.json_body) else case.json_body
        body = json.dumps(payload, ensure_ascii=True).encode("utf-8")
    if body is not None and case.content_type:
        headers.setdefault("Content-Type", case.content_type)
    result = request(base_url, case.method, path, headers=headers, body=body)
    expected = case.expected[role] if isinstance(case.expected[role], tuple) else (case.expected[role],)
    if result.status not in expected:
        raise SmokeFailure(f"expected HTTP {expected}, got {result.status}; body={truncate(result.text)}")
    if case.name == "me profile":
        expect_profile(result, context, role)
    return result


def request(
    base_url: str,
    method: str,
    path: str,
    *,
    headers: dict[str, str],
    body: bytes | None = None,
) -> HTTPResult:
    url = base_url.rstrip("/") + path
    req = urllib.request.Request(url, data=body, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            raw = resp.read()
            return result_from(int(resp.status), dict(resp.headers.items()), raw)
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        return result_from(int(exc.code), dict(exc.headers.items()), raw)
    except urllib.error.URLError as exc:
        raise SmokeFailure(f"request failed: {exc}") from exc


def result_from(status: int, headers: dict[str, str], body: bytes) -> HTTPResult:
    json_body = None
    content_type = headers.get("Content-Type", "")
    if "json" in content_type:
        with contextlib.suppress(json.JSONDecodeError):
            json_body = json.loads(body.decode("utf-8"))
    return HTTPResult(status=status, headers=headers, body=body, json_body=json_body)


def headers_for(auth: str | None, args: argparse.Namespace, context: dict[str, Any]) -> dict[str, str]:
    if auth is None:
        return {}
    profiles: dict[str, AuthProfile] = context["auth_profiles"]
    if auth in profiles:
        return dict(profiles[auth].headers)
    raise ValueError(f"unknown auth profile: {auth}")


def build_auth_profiles(args: argparse.Namespace) -> dict[str, AuthProfile]:
    if args.auth_mode == "header":
        return {
            "admin": AuthProfile("admin", args.tenant, args.account, {"X-Tenant-ID": args.tenant, "X-Account-ID": args.account}),
            "employee": AuthProfile("employee", "demo", "acct-employee", dict(EMPLOYEE_HEADERS)),
            "audit": AuthProfile("audit", "demo", "acct-audit", dict(AUDIT_HEADERS)),
        }
    login_profiles = parse_login_profiles(args)
    token_url = keycloak_token_url(args)
    profiles: dict[str, AuthProfile] = {}
    for login in login_profiles.values():
        token = login_keycloak(token_url, args, login)
        claims = decode_jwt_claims(token)
        token_tenant = str(claims.get("tenant_id") or claims.get("tid") or "")
        token_account = str(claims.get("account_id") or claims.get("acct") or claims.get("sub") or "")
        if token_tenant != login.tenant_id or token_account != login.account_id:
            raise SmokeFailure(
                f"login profile {login.name} resolved unexpected claims: "
                f"tenant_id={token_tenant!r}, account_id={token_account!r}; "
                f"expected {login.tenant_id!r}/{login.account_id!r}"
            )
        profiles[login.name] = AuthProfile(
            name=login.name,
            tenant_id=login.tenant_id,
            account_id=login.account_id,
            headers={"Authorization": "Bearer " + token},
        )
    return profiles


def parse_login_profiles(args: argparse.Namespace) -> dict[str, LoginProfile]:
    profiles: dict[str, LoginProfile] = {}
    for raw in args.login_profile:
        if "=" not in raw:
            raise SmokeFailure(f"invalid --login-profile {raw!r}; expected ROLE=USERNAME:PASSWORD[:TENANT:ACCOUNT]")
        role, value = raw.split("=", 1)
        parts = value.split(":", 3)
        if len(parts) not in {2, 4}:
            raise SmokeFailure(f"invalid --login-profile {raw!r}; expected ROLE=USERNAME:PASSWORD[:TENANT:ACCOUNT]")
        tenant, account = DEFAULT_ROLE_ACCOUNTS.get(role, ("demo", ""))
        if len(parts) == 4:
            tenant, account = parts[2], parts[3]
        profiles[role] = LoginProfile(role, parts[0], parts[1], tenant, account)
    for role, (tenant, account) in DEFAULT_ROLE_ACCOUNTS.items():
        if role in profiles:
            continue
        prefix = "SMOKE_" + role.upper()
        username = env_first(prefix + "_USERNAME")
        password = env_first(prefix + "_PASSWORD")
        if username or password:
            if not username or not password:
                raise SmokeFailure(f"{prefix}_USERNAME and {prefix}_PASSWORD must be set together")
            profiles[role] = LoginProfile(
                role,
                username,
                password,
                env_first(prefix + "_TENANT_ID", fallback=tenant),
                env_first(prefix + "_ACCOUNT_ID", fallback=account),
            )
    missing = [role for role in DEFAULT_ROLE_ACCOUNTS if role not in profiles]
    if missing:
        env_hint = ", ".join("SMOKE_" + role.upper() + "_USERNAME/PASSWORD" for role in missing)
        raise SmokeFailure(
            "keycloak mode requires login credentials for admin, employee, and audit profiles; "
            f"missing {', '.join(missing)} ({env_hint})"
        )
    return profiles


def keycloak_token_url(args: argparse.Namespace) -> str:
    if args.keycloak_token_url:
        return args.keycloak_token_url
    if not args.keycloak_issuer_url:
        raise SmokeFailure("--keycloak-issuer-url or SMOKE_KEYCLOAK_ISSUER_URL is required in keycloak mode")
    discovery_url = args.keycloak_issuer_url.rstrip("/") + "/.well-known/openid-configuration"
    discovery = request(discovery_url, "GET", "", headers={})
    if discovery.status != 200 or not isinstance(discovery.json_body, dict) or not discovery.json_body.get("token_endpoint"):
        raise SmokeFailure(f"cannot discover Keycloak token endpoint from {discovery_url}: {truncate(discovery.text)}")
    return str(discovery.json_body["token_endpoint"])


def login_keycloak(token_url: str, args: argparse.Namespace, profile: LoginProfile) -> str:
    form = {
        "grant_type": "password",
        "client_id": args.keycloak_client_id,
        "username": profile.username,
        "password": profile.password,
        "scope": args.keycloak_scope,
    }
    if args.keycloak_client_secret:
        form["client_secret"] = args.keycloak_client_secret
    body = urllib.parse.urlencode(form).encode("utf-8")
    result = request(token_url, "POST", "", headers={"Content-Type": "application/x-www-form-urlencoded"}, body=body)
    if result.status != 200 or not isinstance(result.json_body, dict) or not result.json_body.get("access_token"):
        raise SmokeFailure(f"Keycloak login failed for profile {profile.name}: HTTP {result.status} {truncate(result.text)}")
    return str(result.json_body["access_token"])


def decode_jwt_claims(token: str) -> dict[str, Any]:
    parts = token.split(".")
    if len(parts) < 2:
        raise SmokeFailure("access token is not a JWT")
    payload = parts[1]
    payload += "=" * (-len(payload) % 4)
    try:
        raw = base64.urlsafe_b64decode(payload.encode("ascii"))
        claims = json.loads(raw.decode("utf-8"))
    except Exception as exc:  # noqa: BLE001 - keep token parsing failure user-facing.
        raise SmokeFailure(f"cannot decode access token claims: {exc}") from exc
    if not isinstance(claims, dict):
        raise SmokeFailure("access token claims are not a JSON object")
    return claims


def env_first(*names: str, fallback: str = "") -> str:
    for name in names:
        value = os.environ.get(name)
        if value:
            return value
    return fallback


def build_cases() -> list[Case]:
    return [
        Case("healthz", "GET", "/healthz", 200, auth=None, check=expect_data_object),
        Case("readyz", "GET", "/readyz", 200, auth=None, check=expect_data_object),
        Case("openapi spec", "GET", "/openapi.yaml", 200, auth=None, check=expect_text("openapi: 3.0.3")),
        Case("swagger ui", "GET", "/swagger/index.html", 200, auth=None, check=expect_text("swagger-ui")),
        Case("missing auth boundary", "GET", "/v1/me", 401, route_key="GET /v1/me", auth=None),
        Case("me profile", "GET", "/v1/me", 200, route_key="GET /v1/me", check=expect_data_object),
        Case("me menus", "GET", "/v1/me/menus", 200, route_key="GET /v1/me/menus", check=expect_data_object),
        Case(
            "authz check",
            "POST",
            "/v1/authz/check",
            200,
            route_key="POST /v1/authz/check",
            json_body=authz_export_check,
            check=expect_data_object,
        ),
        Case(
            "authz batch-check",
            "POST",
            "/v1/authz/batch-check",
            200,
            route_key="POST /v1/authz/batch-check",
            json_body={"checks": [authz_export_check({}), {"resource": "me", "action": "read"}]},
            check=expect_data_object,
        ),
        Case(
            "authz explain",
            "POST",
            "/v1/authz/explain",
            200,
            route_key="POST /v1/authz/explain",
            json_body=authz_export_check,
            check=expect_data_object,
        ),
        Case(
            "authz simulate requires approval",
            "POST",
            "/v1/authz/simulate",
            403,
            route_key="POST /v1/authz/simulate",
            json_body=authz_export_check,
        ),
        Case(
            "authz simulate confirmed",
            "POST",
            "/v1/authz/simulate",
            200,
            route_key="POST /v1/authz/simulate",
            json_body=authz_export_check,
            headers=APPROVAL_HEADER,
            check=expect_data_object,
        ),
        *list_cases(),
        *iam_write_cases(),
        *hr_cases(),
        *attendance_cases(),
        *workflow_cases(),
        *agent_cases(),
        Case(
            "audit logs require approval",
            "GET",
            "/v1/audit-logs",
            403,
            route_key="GET /v1/audit-logs",
        ),
        Case(
            "audit logs confirmed",
            "GET",
            "/v1/audit-logs?page=1&page_size=5",
            200,
            route_key="GET /v1/audit-logs",
            headers=APPROVAL_HEADER,
            check=expect_page,
        ),
    ]


def build_role_matrix_cases() -> list[MatrixCase]:
    return [
        MatrixCase(
            "me profile",
            "GET",
            "/v1/me",
            {"admin": 200, "employee": 200, "audit": 200},
        ),
        MatrixCase(
            "employee list visibility",
            "GET",
            "/v1/hr/employees?page=1&page_size=5",
            {"admin": 200, "employee": 200, "audit": 403},
        ),
        MatrixCase(
            "employee detail self-scope",
            "GET",
            "/v1/hr/employees/emp-admin",
            {"admin": 200, "employee": 403, "audit": 403},
        ),
        MatrixCase(
            "employee detail own record",
            "GET",
            "/v1/hr/employees/emp-employee",
            {"admin": 200, "employee": 200, "audit": 403},
        ),
        MatrixCase(
            "iam permission sets",
            "GET",
            "/v1/iam/permission-sets?page=1&page_size=5",
            {"admin": 200, "employee": 403, "audit": 200},
        ),
        MatrixCase(
            "audit logs high-risk read",
            "GET",
            "/v1/audit-logs?page=1&page_size=5",
            {"admin": 200, "employee": 403, "audit": 200},
            headers=APPROVAL_HEADER,
        ),
        MatrixCase(
            "agent run list",
            "GET",
            "/v1/agents/runs?page=1&page_size=5",
            {"admin": 200, "employee": 200, "audit": 403},
        ),
        MatrixCase(
            "create hr employee",
            "POST",
            "/v1/hr/employees",
            {"admin": 201, "employee": 403, "audit": 403},
            json_body=lambda c: employee_payload(c, "matrix"),
        ),
        MatrixCase(
            "create iam user group",
            "POST",
            "/v1/iam/user-groups",
            {"admin": 201, "employee": 403, "audit": 403},
            json_body=lambda c: {"name": "Matrix Group " + c["suffix"]},
            headers=APPROVAL_HEADER,
        ),
        MatrixCase(
            "create leave request",
            "POST",
            "/v1/attendance/leave-requests",
            {"admin": 201, "employee": 201, "audit": 403},
            json_body={"employee_id": "emp-employee", "leave_type": "annual", "start_at": "2026-08-01", "end_at": "2026-08-02", "hours": 8, "reason": "matrix"},
        ),
        MatrixCase(
            "create agent run",
            "POST",
            "/v1/agents/runs",
            {"admin": 201, "employee": 201, "audit": 403},
            json_body={"mode": "policy_qa", "prompt": "Matrix role check"},
            headers=APPROVAL_HEADER,
        ),
    ]


def list_cases() -> list[Case]:
    return [
        Case("iam permissions list", "GET", "/v1/iam/permissions?page=1&page_size=5", 200, "GET /v1/iam/permissions", check=expect_page),
        Case("iam permissions bad page_size", "GET", "/v1/iam/permissions?page_size=101", 400, "GET /v1/iam/permissions"),
        Case("iam user groups list", "GET", "/v1/iam/user-groups?page=1&page_size=5", 200, "GET /v1/iam/user-groups", check=expect_page),
        Case("iam permission sets list", "GET", "/v1/iam/permission-sets?page=1&page_size=5", 200, "GET /v1/iam/permission-sets", check=expect_page),
        Case("iam permission assignments list", "GET", "/v1/iam/permission-set-assignments?page=1&page_size=5", 200, "GET /v1/iam/permission-set-assignments", check=expect_page),
        Case("iam data scopes list", "GET", "/v1/iam/data-scopes?page=1&page_size=5", 200, "GET /v1/iam/data-scopes", check=expect_page),
        Case("iam field policies list", "GET", "/v1/iam/field-policies?application_code=hr&resource_type=employee&page=1&page_size=5", 200, "GET /v1/iam/field-policies", check=expect_page),
        Case("iam assumable roles list", "GET", "/v1/iam/assumable-roles?page=1&page_size=5", 200, "GET /v1/iam/assumable-roles", check=expect_page),
        Case("org units list", "GET", "/v1/org/units?page=1&page_size=5", 200, "GET /v1/org/units", check=expect_page),
        Case(
            "create org unit",
            "POST",
            "/v1/org/units",
            201,
            "POST /v1/org/units",
            json_body=lambda c: {"code": "SMK" + c["suffix"][-6:], "name": "Smoke Org " + c["suffix"], "parent_id": "ou-hq"},
            check=expect_data_object,
            capture=capture_id("org_unit_id"),
        ),
        Case("leave balances list", "GET", "/v1/attendance/leave-balances?page=1&page_size=5", 200, "GET /v1/attendance/leave-balances", check=expect_page),
        Case("leave requests list", "GET", "/v1/attendance/leave-requests?page=1&page_size=5", 200, "GET /v1/attendance/leave-requests", check=expect_page),
        Case("form templates list", "GET", "/v1/forms/templates?page=1&page_size=5", 200, "GET /v1/forms/templates", check=expect_page),
        Case("agent runs list", "GET", "/v1/agents/runs?page=1&page_size=5", 200, "GET /v1/agents/runs", check=expect_page),
    ]


def iam_write_cases() -> list[Case]:
    return [
        Case(
            "create user group needs approval",
            "POST",
            "/v1/iam/user-groups",
            403,
            "POST /v1/iam/user-groups",
            json_body=lambda c: {"name": "Smoke Group " + c["suffix"]},
        ),
        Case(
            "create user group confirmed",
            "POST",
            "/v1/iam/user-groups",
            201,
            "POST /v1/iam/user-groups",
            json_body=lambda c: {"name": "Smoke Group " + c["suffix"], "member_account_ids": ["acct-employee"]},
            headers=APPROVAL_HEADER,
            check=expect_data_object,
            capture=capture_id("user_group_id"),
        ),
        Case(
            "create permission set confirmed",
            "POST",
            "/v1/iam/permission-sets",
            201,
            "POST /v1/iam/permission-sets",
            json_body=lambda c: {
                "name": "Smoke Permission Set " + c["suffix"],
                "permissions": [{"resource": "me", "action": "read", "scope": "all", "menu_key": "workbench"}],
            },
            headers=APPROVAL_HEADER,
            check=expect_data_object,
            capture=capture_id("permission_set_id"),
        ),
        Case(
            "create permission set assignment confirmed",
            "POST",
            "/v1/iam/permission-set-assignments",
            201,
            "POST /v1/iam/permission-set-assignments",
            json_body=lambda c: {
                "principal_type": "account",
                "principal_id": "acct-employee",
                "permission_set_id": c["permission_set_id"],
                "effect": "allow",
            },
            headers=APPROVAL_HEADER,
            check=expect_data_object,
            capture=capture_id("permission_assignment_id"),
        ),
        Case(
            "create data scope confirmed",
            "POST",
            "/v1/iam/data-scopes",
            201,
            "POST /v1/iam/data-scopes",
            json_body=lambda c: {
                "code": "smoke_scope_" + c["suffix"],
                "name": "Smoke Scope " + c["suffix"],
                "scope_type": "department",
                "params": {"department_ids": ["ou-hq"]},
            },
            headers=APPROVAL_HEADER,
            check=expect_data_object,
            capture=capture_id("data_scope_id"),
        ),
        Case(
            "create field policy confirmed",
            "POST",
            "/v1/iam/field-policies",
            201,
            "POST /v1/iam/field-policies",
            json_body=lambda c: {
                "application_code": "hr",
                "resource_type": "employee",
                "field_name": "smoke_field_" + c["suffix"],
                "effect": "mask",
                "mask_strategy": "partial",
            },
            headers=APPROVAL_HEADER,
            check=expect_data_object,
            capture=capture_id("field_policy_id"),
        ),
        Case(
            "create assumable role confirmed",
            "POST",
            "/v1/iam/assumable-roles",
            201,
            "POST /v1/iam/assumable-roles",
            json_body=lambda c: {
                "name": "Smoke Assume " + c["suffix"],
                "trusted": True,
                "trust_policy": {"accounts": ["acct-admin"]},
                "permission_boundary": {"allow": ["audit.log.read", "iam.permission_set.read"]},
                "permission_set_ids": ["ps-audit"],
                "session_duration_seconds": 3600,
            },
            headers=APPROVAL_HEADER,
            check=expect_data_object,
            capture=capture_id("assumable_role_id"),
        ),
        Case(
            "assume role confirmed",
            "POST",
            lambda c: "/v1/iam/assumable-roles/" + urllib.parse.quote(c["assumable_role_id"]) + "/assume",
            201,
            "POST /v1/iam/assumable-roles/{id}/assume",
            json_body={"reason": "smoke test"},
            headers=APPROVAL_HEADER,
            check=expect_data_object,
            capture=capture_id("assume_session_id", source_key="session_id"),
        ),
    ]


def hr_cases() -> list[Case]:
    return [
        Case("employees list", "GET", "/v1/hr/employees?page=1&page_size=2&sort=created_at_desc", 200, "GET /v1/hr/employees", check=expect_page),
        Case("employees invalid page", "GET", "/v1/hr/employees?page=abc", 400, "GET /v1/hr/employees"),
        Case("employee stats", "GET", "/v1/hr/employees/stats", 200, "GET /v1/hr/employees/stats", check=expect_data_object),
        Case("employee options", "GET", "/v1/hr/employee-options", 200, "GET /v1/hr/employee-options", check=expect_data_object),
        Case(
            "create employee invalid body",
            "POST",
            "/v1/hr/employees",
            400,
            "POST /v1/hr/employees",
            json_body={},
        ),
        Case(
            "create employee main",
            "POST",
            "/v1/hr/employees",
            201,
            "POST /v1/hr/employees",
            json_body=lambda c: employee_payload(c, "main"),
            check=expect_data_object,
            capture=capture_id("employee_id"),
        ),
        Case(
            "create employee delete target",
            "POST",
            "/v1/hr/employees",
            201,
            "POST /v1/hr/employees",
            json_body=lambda c: employee_payload(c, "delete"),
            check=expect_data_object,
            capture=capture_id("delete_employee_id"),
        ),
        Case(
            "employee detail",
            "GET",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]),
            200,
            "GET /v1/hr/employees/{id}",
            check=expect_data_object,
        ),
        Case(
            "employee patch",
            "PATCH",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]),
            200,
            "PATCH /v1/hr/employees/{id}",
            json_body={"phone": "0911222333", "position": "Smoke Analyst"},
            check=expect_data_object,
        ),
        Case(
            "employee direct status needs approval",
            "PATCH",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/status",
            403,
            "PATCH /v1/hr/employees/{id}/status",
            json_body={"status": "probation"},
        ),
        Case(
            "employee direct status confirmed",
            "PATCH",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/status",
            200,
            "PATCH /v1/hr/employees/{id}/status",
            json_body={"status": "probation"},
            headers=APPROVAL_HEADER,
            check=expect_data_object,
        ),
        Case(
            "employee lifecycle transition confirmed",
            "POST",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/status-transition",
            200,
            "POST /v1/hr/employees/{id}/status-transition",
            json_body={"status": "leave_suspended", "reason": "smoke boundary", "start_date": "2026-07-01", "end_date": "2026-07-02"},
            headers=APPROVAL_HEADER,
            check=expect_data_object,
        ),
        Case(
            "invite employee confirmed",
            "POST",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/invite",
            200,
            "POST /v1/hr/employees/{id}/invite",
            json_body=lambda c: {"email": f"smoke.invite.{c['suffix']}@example.com"},
            headers=APPROVAL_HEADER,
            check=expect_data_object,
        ),
        Case("employee export needs approval", "GET", "/v1/hr/employees/export", 403, "GET /v1/hr/employees/export"),
        Case("employee export csv", "GET", "/v1/hr/employees/export", 200, "GET /v1/hr/employees/export", headers=APPROVAL_HEADER, check=expect_text("Demo Admin")),
        Case("employee export json", "POST", "/v1/hr/employees/export", 200, "POST /v1/hr/employees/export", json_body={"page": 1, "page_size": 5}, headers=APPROVAL_HEADER, check=expect_page),
        Case(
            "employee import preview",
            "POST",
            "/v1/hr/employees/import/preview",
            201,
            "POST /v1/hr/employees/import/preview",
            json_body=lambda c: {
                "filename": "employees.csv",
                "content": f"Employee No,Name,Email,Department,Position,Category,Phone,Status,Hire Date,Manager Employee ID\nSMK{c['suffix']},Import {c['suffix']},smoke.import.{c['suffix']}@example.com,ou-hq,Recruiter,full_time,0911888999,active,2026-06-01,\n",
            },
            headers=APPROVAL_HEADER,
            check=expect_data_object,
            capture=capture_id("import_session_id"),
        ),
        Case(
            "employee import confirm",
            "POST",
            lambda c: "/v1/hr/employees/import/" + urllib.parse.quote(c["import_session_id"]) + "/confirm",
            200,
            "POST /v1/hr/employees/import/{id}/confirm",
            json_body={"mode": "create"},
            headers=APPROVAL_HEADER,
            check=expect_data_object,
        ),
        Case(
            "employee batch delete partial",
            "POST",
            "/v1/hr/employees/batch-delete",
            207,
            "POST /v1/hr/employees/batch-delete",
            json_body=lambda c: {"employee_ids": [c["delete_employee_id"], "emp-smoke-missing"], "reason": "smoke cleanup"},
            headers=APPROVAL_HEADER,
            check=expect_data_object,
        ),
        Case(
            "employee delete confirmed",
            "DELETE",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]),
            200,
            "DELETE /v1/hr/employees/{id}",
            headers=APPROVAL_HEADER,
            check=expect_data_object,
        ),
    ]


def attendance_cases() -> list[Case]:
    return [
        Case(
            "create leave request invalid hours",
            "POST",
            "/v1/attendance/leave-requests",
            400,
            "POST /v1/attendance/leave-requests",
            json_body={"employee_id": "emp-employee", "leave_type": "annual", "start_at": "2026-07-01", "end_at": "2026-07-02", "hours": 0},
        ),
        Case(
            "create leave request",
            "POST",
            "/v1/attendance/leave-requests",
            201,
            "POST /v1/attendance/leave-requests",
            json_body={"employee_id": "emp-employee", "leave_type": "annual", "start_at": "2026-07-01", "end_at": "2026-07-02", "hours": 8, "reason": "smoke"},
            check=expect_data_object,
            capture=capture_id("leave_request_id"),
        ),
    ]


def workflow_cases() -> list[Case]:
    return [
        Case(
            "create form template",
            "POST",
            "/v1/forms/templates",
            201,
            "POST /v1/forms/templates",
            json_body=lambda c: {"key": "smoke-template-" + c["suffix"], "name": "Smoke Template", "schema": {"type": "object"}},
            check=expect_data_object,
            capture=capture_id("form_template_id"),
        ),
        Case(
            "submit workflow form",
            "POST",
            "/v1/workflows/forms/leave-request/submit",
            201,
            "POST /v1/workflows/forms/{id}/submit",
            json_body={"payload": {"leave_type": "annual", "hours": 1}},
            check=expect_data_object,
            capture=capture_id("form_instance_id"),
        ),
    ]


def agent_cases() -> list[Case]:
    return [
        Case(
            "create agent run needs approval",
            "POST",
            "/v1/agents/runs",
            403,
            "POST /v1/agents/runs",
            json_body={"prompt": "What is the leave policy?"},
        ),
        Case(
            "create agent run confirmed",
            "POST",
            "/v1/agents/runs",
            201,
            "POST /v1/agents/runs",
            json_body={"mode": "policy_qa", "prompt": "What is the leave policy?"},
            headers=APPROVAL_HEADER,
            check=expect_data_object,
            capture=capture_id("agent_run_id"),
        ),
    ]


def employee_payload(context: dict[str, Any], label: str) -> dict[str, Any]:
    suffix = context["suffix"]
    return {
        "employee_no": f"SMK-{label}-{suffix}",
        "name": f"Smoke {label} {suffix}",
        "company_email": f"smoke.{label}.{suffix}@example.com",
        "phone": "0911000999",
        "org_unit_id": "ou-hq",
        "position": "Smoke Analyst",
        "category": "full_time",
        "employment_status": "onboarding",
        "hire_date": "2026-06-01",
        "basic_info": {"name": f"Smoke {label} {suffix}", "company_email": f"smoke.{label}.{suffix}@example.com"},
        "employment_info": {"org_unit_id": "ou-hq", "position": "Smoke Analyst", "category": "full_time"},
        "contact_info": {"mobile_phone": "0911000999"},
    }


def authz_export_check(_: dict[str, Any]) -> dict[str, Any]:
    return {
        "application_code": "hr",
        "resource_type": "employee",
        "resource_id": "emp-employee",
        "action": "export",
    }


def expect_data_object(result: HTTPResult, _: dict[str, Any]) -> None:
    if not isinstance(result.json_body, dict) or "data" not in result.json_body:
        raise SmokeFailure(f"expected JSON data envelope, got {truncate(result.text)}")


def expect_page(result: HTTPResult, context: dict[str, Any]) -> None:
    expect_data_object(result, context)
    data = result.json_body["data"]
    if not isinstance(data, dict) or "items" not in data or "total" not in data:
        raise SmokeFailure(f"expected page envelope, got {truncate(result.text)}")


def expect_text(needle: str) -> CheckFunc:
    def check(result: HTTPResult, _: dict[str, Any]) -> None:
        if needle not in result.text:
            raise SmokeFailure(f"expected response to contain {needle!r}, got {truncate(result.text)}")

    return check


def expect_profile(result: HTTPResult, context: dict[str, Any], role: str) -> None:
    expect_data_object(result, context)
    profile = context["auth_profiles"][role]
    data = result.json_body["data"]
    tenant = data.get("tenant", {}) if isinstance(data, dict) else {}
    account = data.get("account", {}) if isinstance(data, dict) else {}
    if tenant.get("id") != profile.tenant_id or account.get("id") != profile.account_id:
        raise SmokeFailure(
            f"expected {role} profile {profile.tenant_id}/{profile.account_id}, "
            f"got {tenant.get('id')}/{account.get('id')}"
        )


def capture_id(context_key: str, source_key: str = "id") -> CaptureFunc:
    def capture(result: HTTPResult, context: dict[str, Any]) -> None:
        expect_data_object(result, context)
        data = result.json_body["data"]
        if not isinstance(data, dict) or not data.get(source_key):
            raise SmokeFailure(f"expected data.{source_key}, got {truncate(result.text)}")
        context[context_key] = str(data[source_key])

    return capture


def openapi_route_keys() -> set[str]:
    raw = (ROOT / "docs" / "openapi.yaml").read_text(encoding="utf-8")
    keys: set[str] = set()
    current_path = ""
    for line in raw.splitlines():
        if line.startswith("  /"):
            current_path = line.strip().removesuffix(":")
            continue
        if not current_path:
            continue
        method = line.strip().removesuffix(":")
        if method in {"get", "post", "patch", "delete"}:
            keys.add(method.upper() + " " + current_path)
    return keys


def truncate(text: str, limit: int = 500) -> str:
    text = text.replace("\n", "\\n")
    if len(text) <= limit:
        return text
    return text[:limit] + "...<truncated>"


if __name__ == "__main__":
    sys.exit(main())
