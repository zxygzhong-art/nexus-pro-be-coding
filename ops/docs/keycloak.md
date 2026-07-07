# Keycloak 配置指南

本指南说明如何在 Nexus Pro 项目中部署、配置 Keycloak，并将其与前端（`nexus-pro-fe`）和后端（`nexus-pro-be`）对接。

所有基础设施相关的可调项统一在 [`ops/.env`](../.env)。应用层环境变量分别写在 `nexus-pro-be/.env` 与 `nexus-pro-fe/.env.local`。

## 架构概览

```text
┌─────────────┐     BFF (app/api/auth/*)      ┌──────────────┐
│ nexus-pro-fe│ ─────────────────────────────▶│   Keycloak   │
│  (Next.js)  │  password / auth-code / SSO │   (OIDC)     │
└──────┬──────┘                               └──────┬───────┘
       │ httpOnly cookie (_t / _rt)                    │
       │ proxy.ts 注入 Authorization: Bearer           │ Admin API
       ▼                                               │ (可选)
┌─────────────┐                                        ▼
│ nexus-pro-be│◀────────────────────────────── 用户开通 / 邀请
│   (Go API)  │  校验 JWT（iss / aud / tenant_id / sub）
└─────────────┘
```

职责划分：

| 组件 | 职责 |
| --- | --- |
| Keycloak | 登录凭证、OIDC token 签发、社交登录、忘记密码 |
| 前端 BFF | 与 Keycloak 交换 token，写入 httpOnly cookie，不暴露 token 给浏览器 JS |
| 后端 API | 校验 Bearer token，从 claims 解析 `tenant_id` 与 `account_id`；可选通过 Admin API 开通用户 |

## 1. 启动 Keycloak

### 完整观测栈（含 PostgreSQL）

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
./render-configs.sh
docker compose --env-file .env up -d
```

### 仅启动 Keycloak

PostgreSQL 已就绪时：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be/ops
COMPOSE_PROFILES=keycloak docker compose --env-file .env up -d --no-deps keycloak
```

### 本地默认访问地址

| 用途 | 地址 |
| --- | --- |
| Admin Console / OIDC | `http://127.0.0.1:8080` |
| Health / Metrics | `http://127.0.0.1:19090` |

默认 bootstrap 管理员（仅本地开发）：

| 账号 | 密码 |
| --- | --- |
| `admin` | `admin` |

> 正式环境必须更换 `KEYCLOAK_ADMIN_PASSWORD`，并将 `KEYCLOAK_COMMAND` 从 `start-dev` 改为生产模式（`start` + 反向代理 / TLS 配置）。

### 相关 `ops/.env` 变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `KEYCLOAK_IMAGE` | `quay.io/keycloak/keycloak:26.6.4` | 固定版本，不使用 `latest` |
| `KEYCLOAK_HTTP_HOST_PORT` | `8080` | 对宿主机的 HTTP 端口 |
| `KEYCLOAK_COMMAND` | `start-dev` | 本地开发模式 |
| `KEYCLOAK_DB_NAME` | `keycloak` | PostgreSQL 数据库名（initdb 自动创建） |
| `KEYCLOAK_FEATURES` | `opentelemetry` | 启用 OpenTelemetry |
| `KEYCLOAK_TRACING_ENABLED` | `true` | trace 上报到 `otel-collector` |

## 2. 创建 Realm

1. 打开 Admin Console：`http://127.0.0.1:8080`
2. 左上角下拉 → **Create realm**
3. 填写：
   - **Realm name**：`nexus-pro`（与项目约定一致）
4. 保存

后续所有配置都在 `nexus-pro` realm 下进行。

**Issuer URL**（前后端必须一致，无尾部斜杠）：

```text
http://127.0.0.1:8080/realms/nexus-pro
```

若通过反向代理或不同端口对外暴露，请使用浏览器和后端实际访问的地址。例如 `.env.example` 中的 `http://localhost:18080/realms/nexus-pro` 表示宿主映射到了 `18080`——关键是 **issuer 与 discovery 端点可达且一致**。

验证 discovery 端点：

```bash
curl -s http://127.0.0.1:8080/realms/nexus-pro/.well-known/openid-configuration | jq .issuer
```

## 3. 配置应用 Client

项目约定 client id 为 **`nexus-pro-connect-api`**，同时服务于前端 BFF 和后端 token 校验。

路径：**Clients → Create client**

### 3.1 基本设置

| 字段 | 值 |
| --- | --- |
| Client type | OpenID Connect |
| Client ID | `nexus-pro-connect-api` |
| Client authentication | 视部署选择（见下） |

### 3.2 Capability config

| 开关 | 是否开启 | 原因 |
| --- | --- | --- |
| **Standard flow** | ✅ | SSO / 授权码 + PKCE 回调 |
| **Direct access grants** | ✅ | 邮箱 + 密码登录（`/api/auth/login`） |
| **Implicit flow** | ❌ | 已废弃 |
| **Service accounts roles** | ❌ | 应用 client 不需要 |

### 3.3 Login settings

| 字段 | 本地开发示例 |
| --- | --- |
| Valid redirect URIs | `http://localhost:3000/api/auth/keycloak/callback` |
| Valid post logout redirect URIs | `http://localhost:3000/*` |
| Web origins | `http://localhost:3000` |

正式环境替换为实际前端域名，例如 `https://app.example.com/api/auth/keycloak/callback`。

### 3.4 Public vs Confidential

| 类型 | 适用场景 | 前端环境变量 |
| --- | --- | --- |
| **Public** | 本地开发、纯 PKCE 流程 | `KEYCLOAK_CLIENT_SECRET` 留空 |
| **Confidential** | 需要 client secret 保护 token 端点 | 填写 `KEYCLOAK_CLIENT_SECRET` |

前端 `exchangeKeycloakToken` 在配置了 secret 时会自动附带 `client_secret`。

## 4. Protocol Mappers（关键）

后端校验 access token 时，除标准 OIDC claims 外，**必须**包含：

| Claim | 别名 | 用途 |
| --- | --- | --- |
| `tenant_id` | `tid`, `tenant_hint` | 多租户隔离 |
| `sub` | `account_id`, `acct` | 用户身份绑定 |

开通用户时，后端 Admin API 会把 `tenant_id`、`account_id` 写入 Keycloak 用户 attributes。需要通过 Protocol Mapper 映射到 token。

在 **`nexus-pro-connect-api` → Client scopes → Dedicated scope → Add mapper → By configuration → User Attribute`** 分别添加：

### Mapper 1：`tenant_id`

| 字段 | 值 |
| --- | --- |
| Name | `tenant_id` |
| User Attribute | `tenant_id` |
| Token Claim Name | `tenant_id` |
| Claim JSON Type | String |
| Add to ID token | On |
| Add to access token | **On** |
| Add to userinfo | On |

### Mapper 2：`account_id`

| 字段 | 值 |
| --- | --- |
| Name | `account_id` |
| User Attribute | `account_id` |
| Token Claim Name | `account_id` |
| Claim JSON Type | String |
| Add to ID token | On |
| Add to access token | **On** |
| Add to userinfo | On |

### Audience 说明

后端还会校验 `aud` 包含 `KEYCLOAK_CLIENT_ID`（即 `nexus-pro-connect-api`）。Keycloak 默认会为目标 client 签发 audience，一般无需额外 mapper。若自定义了 audience 行为，确保 access token 的 `aud` 包含该 client id。

## 5. 配置 Admin Client（用户开通，可选）

当 `KEYCLOAK_PROVISION_USERS=true` 时，后端通过 Keycloak Admin API 在员工创建 / 导入 / 邀请时自动开通用户。需要一个 **Service Account** client。

路径：**Clients → Create client**

| 字段 | 值 |
| --- | --- |
| Client ID | `nexus-pro-admin`（或自定义，与后端环境变量一致） |
| Client authentication | **On** |
| Service accounts roles | **On** |
| Standard flow | Off |
| Direct access grants | Off |

创建后：

1. 进入 **Service account roles** 标签
2. **Assign role** → Filter by clients → 选择 `realm-management`
3. 至少勾选 **`manage-users`**（创建 / 更新用户、发送 required actions 邮件）

在 **Credentials** 标签复制 **Client secret**，填入后端：

```bash
KEYCLOAK_ADMIN_CLIENT_ID=nexus-pro-admin
KEYCLOAK_ADMIN_CLIENT_SECRET=<secret>
```

Admin client 使用 `client_credentials` grant 获取 token（见 `internal/platform/auth/keycloak_admin.go`）。

## 6. 创建测试用户

用于本地联调与 smoke test。每个用户的 attributes 必须包含 `tenant_id` 和 `account_id`。

路径：**Users → Add user**

| 字段 | admin 示例 | employee 示例 |
| --- | --- | --- |
| Username | `local-admin` | `local-employee` |
| Email | `admin@example.com` | `employee@example.com` |
| Email verified | On | On |

保存后进入 **Credentials** 标签设置密码（关闭 Temporary）。

进入 **Attributes** 标签：

**admin 用户：**

| Key | Value |
| --- | --- |
| `tenant_id` | `demo` |
| `account_id` | `acct-admin` |

**employee 用户：**

| Key | Value |
| --- | --- |
| `tenant_id` | `demo` |
| `account_id` | `acct-employee` |

这些值须与后端数据库 `accounts` / `user_identities` 表中的记录一致。Smoke test 约定见 [`tools/api-smoke/README.md`](../../tools/api-smoke/README.md)。

### 验证 token claims

```bash
TOKEN=$(curl -s -X POST "http://127.0.0.1:8080/realms/nexus-pro/protocol/openid-connect/token" \
  -d "grant_type=password" \
  -d "client_id=nexus-pro-connect-api" \
  -d "username=local-admin" \
  -d "password=<password>" \
  -d "scope=openid profile email" \
  | jq -r .access_token)

# 解码 payload（仅用于调试）
echo "$TOKEN" | cut -d. -f2 | base64 -d 2>/dev/null | jq .
```

确认输出包含 `"tenant_id": "demo"` 和 `"account_id": "acct-admin"`（或 `"sub"` 有值）。

## 7. 社交登录（可选）

前端支持 Google / Microsoft SSO，通过 Keycloak Identity Provider 中转。

路径：**Identity providers → Add provider**

| Provider | 前端参数 | 默认 IdP alias |
| --- | --- | --- |
| Google | `/api/auth/oidc/google/authorize` | `google` |
| Microsoft | `/api/auth/oidc/microsoft/authorize` | `microsoft` |

在 IdP 配置中填写 OAuth client id / secret。若 alias 不是默认值，在前端设置：

```bash
KEYCLOAK_GOOGLE_IDP_ALIAS=google
KEYCLOAK_MICROSOFT_IDP_ALIAS=microsoft
```

SSO 流程使用授权码 + PKCE，回调地址为 `/api/auth/keycloak/callback`，需在应用 client 的 Valid redirect URIs 中允许。

## 8. 忘记密码

前端 `/api/auth/reset-password` 会 302 重定向到 Keycloak 托管流程：

```text
{issuer}/login-actions/reset-credentials?client_id=nexus-pro-connect-api
```

要真正发送重置邮件，还需在 Keycloak 配置 SMTP：

路径：**Realm settings → Email**

填写 SMTP 服务器、发件人地址，并在 **Realm settings → Login** 中启用 **Forgot password**。

## 9. 应用环境变量

### 9.1 前端（`nexus-pro-fe/.env.local`）

```bash
# 必填（认证路由执行期校验）
KEYCLOAK_ISSUER_URL=http://127.0.0.1:8080/realms/nexus-pro
KEYCLOAK_CLIENT_ID=nexus-pro-connect-api

# confidential client 才需要
KEYCLOAK_CLIENT_SECRET=

# 可选，默认 openid profile email
KEYCLOAK_SCOPE=openid profile email

# 社交登录 IdP alias
KEYCLOAK_GOOGLE_IDP_ALIAS=google
KEYCLOAK_MICROSOFT_IDP_ALIAS=microsoft
```

认证流程说明：

| 路由 | Grant type | 前置条件 |
| --- | --- | --- |
| `POST /api/auth/login` | `password` | Direct access grants 已开启 |
| `GET /api/auth/oidc/{provider}/authorize` | 授权码 + PKCE | Standard flow 已开启 |
| `GET /api/auth/keycloak/callback` | `authorization_code` | redirect URI 已注册 |
| `POST /api/auth/refresh` | `refresh_token` | 登录时写入了 `_rt` cookie |
| `GET /api/auth/reset-password` | — | 重定向到 Keycloak reset-credentials |

Token 存储：

- `_t`：access token（httpOnly，path `/`，寿命对齐 `expires_in`）
- `_rt`：refresh token（httpOnly，path `/api/auth`）
- `_session`：会话标记，供 `proxy.ts` 判断登录态

### 9.2 后端（`nexus-pro-be/.env`）

**基础 OIDC 校验（必填，生产环境启动时强制）：**

```bash
KEYCLOAK_ISSUER_URL=http://127.0.0.1:8080/realms/nexus-pro
KEYCLOAK_CLIENT_ID=nexus-pro-connect-api
```

**用户开通（可选）：**

```bash
KEYCLOAK_PROVISION_USERS=true
KEYCLOAK_ADMIN_CLIENT_ID=nexus-pro-admin
KEYCLOAK_ADMIN_CLIENT_SECRET=<secret>

# 邀请邮件（需 SMTP + manage-users 权限）
KEYCLOAK_SEND_INVITE_EMAIL=false
KEYCLOAK_INVITE_CLIENT_ID=nexus-pro-connect-api
KEYCLOAK_INVITE_REDIRECT_URL=http://localhost:3000/
```

当 `KEYCLOAK_PROVISION_USERS=true` 时，员工创建 / 导入 / 邀请会：

1. 通过 Admin API 在 Keycloak 创建或更新用户
2. 写入 attributes：`tenant_id`、`account_id`、`employee_id`、`employee_no`
3. 将 Keycloak `sub` 绑定到 `user_identities` 表

## 10. 端到端验证

### 10.1 前端登录

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-fe
pnpm dev
```

浏览器打开 `http://localhost:3000/login`，使用测试账号登录。成功后在 DevTools → Application → Cookies 中应看到 `_t`、`_rt`、`_session`。

### 10.2 后端 API

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be
# 确保 KEYCLOAK_ISSUER_URL / KEYCLOAK_CLIENT_ID 已设置
go run ./cmd/api
```

```bash
curl -s http://localhost:8080/v1/health
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/v1/...
```

### 10.3 Smoke test

```bash
export SMOKE_KEYCLOAK_ISSUER_URL="http://127.0.0.1:8080/realms/nexus-pro"
export SMOKE_KEYCLOAK_CLIENT_ID="nexus-pro-connect-api"
export SMOKE_ADMIN_USERNAME="local-admin"
export SMOKE_ADMIN_PASSWORD="..."
tools/api-smoke/full_api_smoke.py
```

## 11. 生产环境检查清单

- [ ] `KEYCLOAK_COMMAND=start`（非 `start-dev`）
- [ ] 管理员密码、client secret 已轮换
- [ ] `KEYCLOAK_ISSUER_URL` 使用 `https://` 公网地址
- [ ] redirect URI / Web origins 仅包含正式域名
- [ ] SMTP 已配置（若启用邀请邮件或忘记密码）
- [ ] Direct access grants 按安全策略评估（密码登录依赖此开关；可仅保留 SSO）
- [ ] Protocol Mapper `tenant_id` / `account_id` 已配置
- [ ] Admin client 的 `manage-users` 权限已最小化授予
- [ ] 后端 `KEYCLOAK_PROVISION_USERS` 与业务流程一致

## 12. 常见问题

### 登录返回 401「Keycloak 登入失敗」

1. 确认 client 已开启 **Direct access grants**
2. 确认用户名 / 密码正确，用户未被禁用
3. 用 curl 直接打 token 端点排查（见第 6 节）

### 后端返回 401「invalid bearer token」

1. 检查 `KEYCLOAK_ISSUER_URL` 与 token 中 `iss` 完全一致（无尾部 `/`）
2. 检查 token `aud` 是否包含 `KEYCLOAK_CLIENT_ID`
3. 检查 token 是否包含 `tenant_id` 和 `sub`（或 `account_id`）——通常是 **Protocol Mapper 未配置**
4. 检查 token 是否过期

### SSO 回调「登入狀態已失效」

1. 确认 `nexus_keycloak_state` cookie 在回调时仍存在（同站、10 分钟有效期）
2. 确认 redirect URI 与 Keycloak client 配置完全匹配
3. 确认前端 origin 与 Web origins 一致

### Admin API 开通用户失败

1. 确认 `KEYCLOAK_ADMIN_CLIENT_ID` / `SECRET` 正确
2. 确认 service account 已分配 `realm-management → manage-users`
3. 确认 `KEYCLOAK_ISSUER_URL` 格式为 `http(s)://host/realms/nexus-pro`

### Issuer 端口不一致

`ops/.env` 默认映射 `8080`，而 `nexus-pro-be/.env.example` 示例为 `18080`。两者不矛盾——选你实际对外暴露的地址即可，前后端必须使用同一个 issuer。

## 相关文件

| 文件 | 说明 |
| --- | --- |
| [`ops/.env`](../.env) | Keycloak 容器部署参数 |
| [`ops/compose.yaml`](../compose.yaml) | Keycloak compose 服务定义 |
| [`nexus-pro-be/.env.example`](../../.env.example) | 后端 Keycloak 环境变量 |
| [`nexus-pro-fe/.env.example`](../../../nexus-pro-fe/.env.example) | 前端 Keycloak 环境变量 |
| [`nexus-pro-fe/app/api/auth/_keycloak.ts`](../../../nexus-pro-fe/app/api/auth/_keycloak.ts) | 前端 BFF Keycloak 集成 |
| [`nexus-pro-be/internal/platform/auth/token.go`](../../internal/platform/auth/token.go) | 后端 JWT 校验 |
| [`nexus-pro-be/internal/platform/auth/keycloak_admin.go`](../../internal/platform/auth/keycloak_admin.go) | 后端 Admin API 用户开通 |
