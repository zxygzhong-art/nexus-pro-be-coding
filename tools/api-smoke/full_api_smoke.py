#!/usr/bin/env python3
"""Run behavioral and authentication-boundary checks over the OpenAPI routes.

The default mode starts the API against the configured PostgreSQL database and
uses real Keycloak login profiles. Pass --base-url to run against an already
started server, or --check-coverage to validate the route plan offline.
"""

from __future__ import annotations

import argparse
import base64
import contextlib
import dataclasses
import json
import os
import pathlib
import re
import socket
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from typing import Any, Callable


ROOT = pathlib.Path(__file__).resolve().parents[2]
OPENAPI_HTTP_METHODS = frozenset({"get", "put", "post", "delete", "options", "head", "patch", "trace"})
DEFAULT_KEYCLOAK_CLIENT_ID = "nexus-pro-connect-api"
DEFAULT_ROLE_ACCOUNTS = {
    "admin": ("demo", "acct-admin"),
    "employee": ("demo", "acct-employee"),
    "audit": ("demo", "acct-audit"),
}


JsonFactory = Callable[[dict[str, Any]], Any]
PathFactory = Callable[[dict[str, Any]], str]
BytesFactory = Callable[[dict[str, Any]], bytes]
ContentTypeFactory = Callable[[dict[str, Any]], str]
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
    raw_body: bytes | str | BytesFactory | None = None
    content_type: str | ContentTypeFactory | None = "application/json"
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
    raw_body: bytes | str | BytesFactory | None = None
    content_type: str | ContentTypeFactory | None = "application/json"
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


@dataclasses.dataclass(frozen=True)
class CasePlan:
    behavioral_cases: tuple[Case, ...]
    auth_boundary_cases: tuple[Case, ...]
    openapi_routes: frozenset[str]
    behavioral_routes: frozenset[str]
    auth_boundary_routes: frozenset[str]

    # cases returns the execution order with deep checks before generated
    # authentication-boundary checks.
    @property
    def cases(self) -> tuple[Case, ...]:
        return self.behavioral_cases + self.auth_boundary_cases


class SmokeFailure(Exception):
    pass


def main() -> int:
    args = parse_args()
    base_url = args.base_url.rstrip("/") if args.base_url else ""
    server: subprocess.Popen[str] | None = None
    logs: list[str] = []

    try:
        if args.check_coverage:
            plan = build_case_plan()
            print("API smoke coverage plan is valid.")
            print_coverage_summary(plan)
            return 0
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
    parser.add_argument("--check-coverage", action="store_true", help="Validate OpenAPI route coverage without DB, Keycloak, or HTTP calls.")
    parser.add_argument("--start-timeout", type=float, default=90.0, help="Seconds to wait for a started server.")
    parser.add_argument("--auth-mode", choices=("keycloak",), default="keycloak", help="Use real Keycloak password-grant login.")
    parser.add_argument("--tenant", default="demo", help="Tenant id for the default request context.")
    parser.add_argument("--account", default="acct-admin", help="Account id for the default request context.")
    parser.add_argument("--skip-role-matrix", action="store_true", help="Skip the multi-role HTTP authorization matrix.")
    parser.add_argument("--keycloak-issuer-url", default=env_first("SMOKE_KEYCLOAK_BASE_URL", "KEYCLOAK_BASE_URL"), help="OIDC issuer URL for real login mode.")
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
    if not (env_first("DB_HOST") and env_first("DB_USERNAME") and env_first("DB_NAME")):
        raise SmokeFailure("DB_HOST, DB_USERNAME, and DB_NAME are required because smoke accounts must come from the database")
    for key in ("REDIS_HOST", "OPENFGA_BASE_URL", "OPENFGA_STORE_ID"):
        env.pop(key, None)
    env.update(
        {
            "APP_ENV": "development",
            "HTTP_ADDR": f"127.0.0.1:{port}",
            "OTEL_ENABLED": "false",
            "LOG_LEVEL": "warn",
            "GOCACHE": str(ROOT / ".gocache"),
        }
    )
    if not args.keycloak_issuer_url:
        raise SmokeFailure("--keycloak-issuer-url or SMOKE_KEYCLOAK_BASE_URL is required")
    env["KEYCLOAK_BASE_URL"] = args.keycloak_issuer_url
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
    plan = build_case_plan()
    auth_profiles = build_auth_profiles(args)
    context: dict[str, Any] = {
        "suffix": str(int(time.time() * 1000)),
        "tenant": args.tenant,
        "account": args.account,
        "auth_profiles": auth_profiles,
    }
    failures: list[str] = []
    cases = plan.cases
    matrix_cases = [] if args.skip_role_matrix else build_role_matrix_cases()

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
    print_coverage_summary(plan)
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
        raw_body = case.raw_body(context) if callable(case.raw_body) else case.raw_body
        body = raw_body.encode("utf-8") if isinstance(raw_body, str) else raw_body
    elif case.json_body is not None:
        payload = case.json_body(context) if callable(case.json_body) else case.json_body
        body = json.dumps(payload, ensure_ascii=True).encode("utf-8")
    if body is not None and case.content_type:
        content_type = case.content_type(context) if callable(case.content_type) else case.content_type
        headers.setdefault("Content-Type", content_type)

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
        raw_body = case.raw_body(context) if callable(case.raw_body) else case.raw_body
        body = raw_body.encode("utf-8") if isinstance(raw_body, str) else raw_body
    elif case.json_body is not None:
        payload = case.json_body(context) if callable(case.json_body) else case.json_body
        body = json.dumps(payload, ensure_ascii=True).encode("utf-8")
    if body is not None and case.content_type:
        content_type = case.content_type(context) if callable(case.content_type) else case.content_type
        headers.setdefault("Content-Type", content_type)
    result = request(base_url, case.method, path, headers=headers, body=body)
    expected = case.expected[role] if isinstance(case.expected[role], tuple) else (case.expected[role],)
    if result.status not in expected:
        raise SmokeFailure(f"expected HTTP {expected}, got {result.status}; body={truncate(result.text)}")
    if case.name == "me profile":
        expect_profile(result, context, role)
    return result


def multipart_body(
    fields: dict[str, str] | None = None,
    files: dict[str, tuple[str, str, bytes]] | None = None,
) -> tuple[bytes, str]:
    boundary = "----nexus-smoke-" + uuid.uuid4().hex
    chunks: list[bytes] = []
    for name, value in (fields or {}).items():
        chunks.extend(
            [
                f"--{boundary}\r\n".encode("ascii"),
                f'Content-Disposition: form-data; name="{name}"\r\n\r\n'.encode("ascii"),
                value.encode("utf-8"),
                b"\r\n",
            ]
        )
    for name, (filename, content_type, content) in (files or {}).items():
        chunks.extend(
            [
                f"--{boundary}\r\n".encode("ascii"),
                (
                    f'Content-Disposition: form-data; name="{name}"; '
                    f'filename="{filename}"\r\n'
                ).encode("ascii"),
                f"Content-Type: {content_type}\r\n\r\n".encode("ascii"),
                content,
                b"\r\n",
            ]
        )
    chunks.append(f"--{boundary}--\r\n".encode("ascii"))
    return b"".join(chunks), "multipart/form-data; boundary=" + boundary


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
        raise SmokeFailure("--keycloak-issuer-url or SMOKE_KEYCLOAK_BASE_URL is required in keycloak mode")
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


# build_case_plan keeps business-level cases explicit while filling documented
# route gaps with side-effect-free unauthenticated registration checks.
def build_case_plan() -> CasePlan:
    behavioral_cases = tuple(build_cases())
    openapi_routes = frozenset(openapi_route_keys())
    behavioral_routes = frozenset(case.route_key for case in behavioral_cases if case.route_key)
    unknown_routes = behavioral_routes - openapi_routes
    if unknown_routes:
        raise SmokeFailure("behavioral cases reference undocumented routes: " + ", ".join(sorted(unknown_routes)))

    auth_boundary_cases = tuple(build_auth_boundary_cases(openapi_routes - behavioral_routes))
    auth_boundary_routes = frozenset(case.route_key for case in auth_boundary_cases if case.route_key)
    missing_routes = openapi_routes - behavioral_routes - auth_boundary_routes
    duplicate_routes = behavioral_routes & auth_boundary_routes
    if missing_routes:
        raise SmokeFailure("coverage plan is missing OpenAPI routes: " + ", ".join(sorted(missing_routes)))
    if duplicate_routes:
        raise SmokeFailure("generated auth-boundary cases overlap behavioral cases: " + ", ".join(sorted(duplicate_routes)))
    if len(auth_boundary_routes) != len(auth_boundary_cases):
        raise SmokeFailure("generated auth-boundary cases contain duplicate or missing route keys")

    return CasePlan(
        behavioral_cases=behavioral_cases,
        auth_boundary_cases=auth_boundary_cases,
        openapi_routes=openapi_routes,
        behavioral_routes=behavioral_routes,
        auth_boundary_routes=auth_boundary_routes,
    )


# build_auth_boundary_cases verifies route registration and mandatory
# authentication without sending credentials or reaching mutation handlers.
def build_auth_boundary_cases(route_keys: frozenset[str] | set[str]) -> list[Case]:
    cases: list[Case] = []
    for route_key in sorted(route_keys):
        method, path = route_key.split(" ", 1)
        if not path.startswith("/v1/"):
            raise SmokeFailure(f"route {route_key} needs an explicit public-route contract case")
        request_path = materialize_openapi_path(path)
        cases.append(
            Case(
                name=f"auth boundary {method} {path}",
                method=method,
                path=request_path,
                expected=401,
                route_key=route_key,
                auth=None,
            )
        )
    return cases


# materialize_openapi_path turns templates into routable sentinel paths while
# preserving the documented route key used by static coverage checks.
def materialize_openapi_path(path: str) -> str:
    request_path = re.sub(r"\{[^/{}]+\}", "smoke-contract", path)
    if "{" in request_path or "}" in request_path:
        raise SmokeFailure(f"cannot materialize OpenAPI path template: {path}")
    return request_path


# print_coverage_summary distinguishes deep behavioral checks from generated
# authentication-boundary checks so the reported coverage is not overstated.
def print_coverage_summary(plan: CasePlan) -> None:
    total = len(plan.openapi_routes)
    print(f"Behavioral OpenAPI route coverage: {len(plan.behavioral_routes)}/{total}")
    print(f"Generated auth-boundary route coverage: {len(plan.auth_boundary_routes)}/{total}")
    print(f"Combined OpenAPI route coverage: {len(plan.behavioral_routes | plan.auth_boundary_routes)}/{total}")


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
            "authz simulate",
            "POST",
            "/v1/authz/simulate",
            200,
            route_key="POST /v1/authz/simulate",
            json_body=authz_export_check,
            check=expect_data_object,
        ),
        *list_cases(),
        *iam_write_cases(),
        *hr_cases(),
        *attendance_cases(),
        *workflow_cases(),
        *agent_cases(),
        Case(
            "audit logs",
            "GET",
            "/v1/audit-logs?page=1&page_size=5",
            200,
            route_key="GET /v1/audit-logs",
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
            "audit logs read",
            "GET",
            "/v1/audit-logs?page=1&page_size=5",
            {"admin": 200, "employee": 403, "audit": 200},
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
            "create user group",
            "POST",
            "/v1/iam/user-groups",
            201,
            "POST /v1/iam/user-groups",
            json_body=lambda c: {"name": "Smoke Group " + c["suffix"], "member_account_ids": ["acct-employee"]},
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
        Case("employee import template csv", "GET", "/v1/hr/employees/import/template?format=csv", 200, "GET /v1/hr/employees/import/template", check=expect_text("員工編號")),
        Case("employee import template bad format", "GET", "/v1/hr/employees/import/template?format=pdf", 400, "GET /v1/hr/employees/import/template"),
        Case(
            "create employee invalid body",
            "POST",
            "/v1/hr/employees",
            400,
            "POST /v1/hr/employees",
            json_body={},
        ),
        Case(
            "preview employee create invalid",
            "POST",
            "/v1/hr/employees/preview",
            200,
            "POST /v1/hr/employees/preview",
            json_body={},
            check=expect_preview_invalid,
        ),
        Case(
            "preview employee create valid",
            "POST",
            "/v1/hr/employees/preview",
            200,
            "POST /v1/hr/employees/preview",
            json_body=lambda c: employee_payload(c, "preview"),
            check=expect_preview_valid,
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
            "employee avatar upload missing file",
            "POST",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/avatar",
            400,
            "POST /v1/hr/employees/{id}/avatar",
            raw_body=empty_multipart_body,
            content_type=empty_multipart_content_type,
        ),
        Case(
            "employee avatar upload",
            "POST",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/avatar",
            200,
            "POST /v1/hr/employees/{id}/avatar",
            raw_body=avatar_multipart_body,
            content_type=avatar_multipart_content_type,
            check=expect_employee_avatar_present,
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
            "employee patch preview",
            "POST",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/preview",
            200,
            "POST /v1/hr/employees/{id}/preview",
            json_body={"position": "Smoke Preview Analyst"},
            check=expect_preview_valid,
        ),
        Case(
            "employee direct status",
            "PATCH",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/status",
            200,
            "PATCH /v1/hr/employees/{id}/status",
            json_body={"status": "probation"},
            check=expect_data_object,
        ),
        Case(
            "employee lifecycle transition",
            "POST",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/status-transition",
            200,
            "POST /v1/hr/employees/{id}/status-transition",
            json_body={"status": "leave_suspended", "reason": "smoke boundary", "start_date": "2026-07-01", "end_date": "2026-07-02"},
            check=expect_data_object,
        ),
        Case(
            "invite employee",
            "POST",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/invite",
            200,
            "POST /v1/hr/employees/{id}/invite",
            json_body=lambda c: {"email": f"smoke.invite.{c['suffix']}@example.com"},
            check=expect_data_object,
        ),
        Case(
            "employee avatar delete",
            "DELETE",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]) + "/avatar",
            200,
            "DELETE /v1/hr/employees/{id}/avatar",
            check=expect_employee_avatar_absent,
        ),
        Case("employee export csv", "GET", "/v1/hr/employees/export", 200, "GET /v1/hr/employees/export", check=expect_text("Demo Admin")),
        Case("employee export json", "POST", "/v1/hr/employees/export", 200, "POST /v1/hr/employees/export", json_body={"page": 1, "page_size": 5}, check=expect_page),
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
            check=expect_data_object,
        ),
        Case(
            "employee batch delete partial",
            "POST",
            "/v1/hr/employees/batch-delete",
            207,
            "POST /v1/hr/employees/batch-delete",
            json_body=lambda c: {"employee_ids": [c["delete_employee_id"], "emp-smoke-missing"], "reason": "smoke cleanup"},
            check=expect_data_object,
        ),
        Case(
            "employee delete confirmed",
            "DELETE",
            lambda c: "/v1/hr/employees/" + urllib.parse.quote(c["employee_id"]),
            200,
            "DELETE /v1/hr/employees/{id}",
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
        Case(
            "approve workflow form",
            "POST",
            lambda c: "/v1/workflows/forms/" + urllib.parse.quote(c["form_instance_id"]) + "/approve",
            200,
            "POST /v1/workflows/forms/{id}/approve",
            json_body={"reason": "smoke approval"},
            check=expect_data_object,
        ),
    ]


def agent_cases() -> list[Case]:
    return [
        Case(
            "create agent run",
            "POST",
            "/v1/agents/runs",
            201,
            "POST /v1/agents/runs",
            json_body={"mode": "policy_qa", "prompt": "What is the leave policy?"},
            check=expect_data_object,
            capture=capture_id("agent_run_id"),
        ),
    ]


def employee_payload(context: dict[str, Any], label: str) -> dict[str, Any]:
    suffix = context["suffix"]
    name = f"Smoke {label} {suffix}"
    email = f"smoke.{label}.{suffix}@example.com"
    national_id = "SMK" + suffix[-7:] + label[:1].upper()
    return {
        "employee_no": f"SMK-{label}-{suffix}",
        "name": name,
        "company_email": email,
        "phone": "0911000999",
        "org_unit_id": "ou-hq",
        "position": "Smoke Analyst",
        "category": "full_time",
        "employment_status": "onboarding",
        "hire_date": "2026-06-01",
        "basic_info": {
            "name": name,
            "company_email": email,
            "nationality_type": "local",
            "national_id": national_id,
        },
        "employment_info": {"org_unit_id": "ou-hq", "position": "Smoke Analyst", "category": "full_time"},
        "education_military_info": {"highest_education": "master", "school": "NTU"},
        "contact_info": {
            "mobile_phone": "0911000999",
            "address": "Taipei",
            "emergency_contact_relation": "spouse",
            "emergency_contact_name": "Smoke Contact",
            "emergency_contact_phone": "0922333444",
        },
        "insurance_info": {
            "labor_insurance_date": "2026-06-01",
            "labor_insurance_level": "L1",
            "labor_insurance_salary": "45800",
            "health_insurance_date": "2026-06-01",
            "health_insurance_level": "H1",
            "health_insurance_amount": "826",
        },
    }


def authz_export_check(_: dict[str, Any]) -> dict[str, Any]:
    return {
        "application_code": "hr",
        "resource_type": "employee",
        "resource_id": "emp-employee",
        "action": "export",
    }


def empty_multipart_body(context: dict[str, Any]) -> bytes:
    body, content_type = multipart_body()
    context["_empty_multipart_content_type"] = content_type
    return body


def empty_multipart_content_type(context: dict[str, Any]) -> str:
    return str(context.pop("_empty_multipart_content_type"))


def avatar_multipart_body(context: dict[str, Any]) -> bytes:
    body, content_type = avatar_multipart()
    context["_avatar_multipart_content_type"] = content_type
    return body


def avatar_multipart_content_type(context: dict[str, Any]) -> str:
    return str(context.pop("_avatar_multipart_content_type"))


def avatar_multipart() -> tuple[bytes, str]:
    return multipart_body(
        files={
            "file": (
                "avatar.png",
                "image/png",
                base64.b64decode(
                    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ"
                    "AAAADUlEQVR42mP8z8BQDwAFgwJ/lQ4N9wAAAABJRU5ErkJggg=="
                ),
            )
        }
    )


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


def expect_preview_valid(result: HTTPResult, context: dict[str, Any]) -> None:
    expect_data_object(result, context)
    data = result.json_body["data"]
    if not isinstance(data, dict) or data.get("valid") is not True or not isinstance(data.get("employee"), dict):
        raise SmokeFailure(f"expected valid employee preview, got {truncate(result.text)}")


def expect_preview_invalid(result: HTTPResult, context: dict[str, Any]) -> None:
    expect_data_object(result, context)
    data = result.json_body["data"]
    if not isinstance(data, dict) or data.get("valid") is not False:
        raise SmokeFailure(f"expected invalid employee preview, got {truncate(result.text)}")
    errors = data.get("field_errors")
    if not isinstance(errors, list) or not errors:
        raise SmokeFailure(f"expected preview field errors, got {truncate(result.text)}")


def expect_employee_avatar_present(result: HTTPResult, context: dict[str, Any]) -> None:
    expect_data_object(result, context)
    data = result.json_body["data"]
    basic_info = data.get("basic_info") if isinstance(data, dict) else None
    avatar = basic_info.get("avatar") if isinstance(basic_info, dict) else None
    if not isinstance(avatar, dict) or not avatar.get("object_key"):
        raise SmokeFailure(f"expected employee avatar metadata, got {truncate(result.text)}")


def expect_employee_avatar_absent(result: HTTPResult, context: dict[str, Any]) -> None:
    expect_data_object(result, context)
    data = result.json_body["data"]
    basic_info = data.get("basic_info") if isinstance(data, dict) else None
    if isinstance(basic_info, dict) and ("avatar" in basic_info or "avatar_object_key" in basic_info):
        raise SmokeFailure(f"expected employee avatar metadata to be removed, got {truncate(result.text)}")


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


# openapi_route_keys reads only Path Item operations so similarly named schema
# fields cannot be mistaken for HTTP methods.
def openapi_route_keys() -> set[str]:
    raw = (ROOT / "docs" / "openapi.yaml").read_text(encoding="utf-8")
    return openapi_route_keys_from_text(raw)


# openapi_route_keys_from_text performs the narrow YAML inventory needed by the
# smoke runner without adding a parser dependency.
def openapi_route_keys_from_text(raw: str) -> set[str]:
    keys: set[str] = set()
    current_path = ""
    in_paths = False
    for line in raw.splitlines():
        if line == "paths:":
            in_paths = True
            current_path = ""
            continue
        if in_paths and line and not line.startswith(" "):
            in_paths = False
            current_path = ""
        if not in_paths:
            continue
        if line.startswith("  /"):
            current_path = line.strip().removesuffix(":")
            continue
        if not current_path:
            continue
        if len(line) - len(line.lstrip(" ")) != 4:
            continue
        method = line.strip().removesuffix(":")
        if method in OPENAPI_HTTP_METHODS:
            keys.add(method.upper() + " " + current_path)
    return keys


def truncate(text: str, limit: int = 500) -> str:
    text = text.replace("\n", "\\n")
    if len(text) <= limit:
        return text
    return text[:limit] + "...<truncated>"


if __name__ == "__main__":
    sys.exit(main())
