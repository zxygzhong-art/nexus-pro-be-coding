# Keycloak 本地联调配置向导

本文记录 `nexus-pro-be` 当前本地联调使用的 Keycloak 配置，以及 `sslRequired` 字段在哪里配置、如何验证。

## Docker 部署文件

本仓库提供了两套独立的本地部署文件，Keycloak 和 OpenFGA 不写在同一个 compose 里：

| 组件 | Dockerfile | docker-compose | 说明 |
| --- | --- | --- | --- |
| Keycloak | `ops/keycloak/Dockerfile` | `ops/keycloak/docker-compose.yml` | Keycloak + 独立 PostgreSQL |
| OpenFGA | `ops/openfga/Dockerfile` | `ops/openfga/docker-compose.yml` | OpenFGA + 独立 PostgreSQL + migrate job |

启动 Keycloak：

```bash
docker compose -f ops/keycloak/docker-compose.yml up -d --build
```

启动 OpenFGA：

```bash
docker compose -f ops/openfga/docker-compose.yml up -d --build
```

默认端口：

| 组件 | 默认地址 |
| --- | --- |
| Keycloak | `http://localhost:18080` |
| Keycloak PostgreSQL | `localhost:15432` |
| OpenFGA HTTP / Playground | `http://localhost:8081` |
| OpenFGA gRPC | `localhost:8082` |
| OpenFGA PostgreSQL | `localhost:15433` |

更详细的启动说明分别在：

- `ops/keycloak/README.md`
- `ops/openfga/README.md`

## 当前已验证配置

当前后端 `.env` 使用：

```env
KEYCLOAK_ISSUER_URL=http://192.168.100.100:18080/realms/nexus-pro
KEYCLOAK_CLIENT_ID=nexus-pro-connect-api
KEYCLOAK_ENABLED=true
```

注意：当前代码实际只读取 `KEYCLOAK_ISSUER_URL` 和 `KEYCLOAK_CLIENT_ID`。两者都有值时，后端会启用 Keycloak RS256/JWKS token resolver；`KEYCLOAK_ENABLED` 目前只是环境文件里的标记，代码没有读取它。

当前 Keycloak 侧已验证：

| 项 | 当前值 |
| --- | --- |
| Keycloak base URL | `http://192.168.100.100:18080` |
| Realm | `nexus-pro` |
| Realm enabled | `true` |
| Realm `sslRequired` | `none` |
| Client ID | `nexus-pro-connect-api` |
| Client protocol | `openid-connect` |
| Client enabled | `true` |
| Client publicClient | `true` |
| Client standardFlowEnabled | `true` |
| Client directAccessGrantsEnabled | `true` |

## `sslRequired` 在哪里

`sslRequired` 是 Keycloak realm 配置字段，属于 Realm Representation，不是后端 `.env` 字段，也不是 Go 代码里的配置项。

它控制 Keycloak 是否允许用 HTTP 访问 realm 的 OIDC 地址，例如：

```text
http://192.168.100.100:18080/realms/nexus-pro/.well-known/openid-configuration
```

常见取值：

| 值 | 含义 | 适用场景 |
| --- | --- | --- |
| `none` | 允许 HTTP | 本地开发、内网临时联调 |
| `external` | 外部地址要求 HTTPS，localhost 等本地地址可放行 | 有反向代理或半本地环境 |
| `all` | 所有访问都要求 HTTPS | 生产环境 |

本地环境如果 `sslRequired=external`，访问 `http://192.168.100.100:18080/realms/nexus-pro/.well-known/openid-configuration` 会返回：

```json
{"error":"invalid_request","error_description":"HTTPS required"}
```

后端在验证真实 Bearer token 时会先请求 OIDC discovery；如果这里返回 403，后端会把 token 判为无效。

## UI 配置位置

如果当前 Keycloak UI 版本展示该字段，通常在：

```text
Realm settings -> Login -> Require SSL
```

中文界面大致是：

```text
领域设置 -> 登录 -> Require SSL / SSL Required
```

当前这套 UI 中该字段可能不会显示出来。遇到 UI 找不到时，推荐直接使用 Admin REST API 配置，下面的命令是确定可用的方式。

## 用 Admin API 查看和修改 `sslRequired`

先准备变量。不要把管理员密码写进仓库文件，建议用 shell 交互输入或本地私有环境变量：

```bash
export KEYCLOAK_BASE="http://192.168.100.100:18080"
export KEYCLOAK_ADMIN_REALM="master"
export KEYCLOAK_REALM="nexus-pro"
export KEYCLOAK_ADMIN_USER="admin"
read -rsp "Keycloak admin password: " KEYCLOAK_ADMIN_PASSWORD
echo
```

获取 admin token：

```bash
ADMIN_TOKEN=$(
  curl -sS -X POST "$KEYCLOAK_BASE/realms/$KEYCLOAK_ADMIN_REALM/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode "grant_type=password" \
    --data-urlencode "client_id=admin-cli" \
    --data-urlencode "username=$KEYCLOAK_ADMIN_USER" \
    --data-urlencode "password=$KEYCLOAK_ADMIN_PASSWORD" \
  | jq -r ".access_token"
)
```

查看当前 realm 配置：

```bash
curl -sS "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
| jq "{realm, enabled, sslRequired}"
```

本地 HTTP 联调改成 `none`：

```bash
curl -sS -o /tmp/keycloak-realm-update.out -w "http=%{http_code}\n" \
  -X PUT "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"sslRequired":"none"}'
```

期望返回：

```text
http=204
```

再次确认：

```bash
curl -sS "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
| jq "{realm, enabled, sslRequired}"
```

期望：

```json
{
  "realm": "nexus-pro",
  "enabled": true,
  "sslRequired": "none"
}
```

## 验证 OIDC discovery

```bash
curl -sS "$KEYCLOAK_BASE/realms/$KEYCLOAK_REALM/.well-known/openid-configuration" \
| jq "{issuer, jwks_uri, token_endpoint}"
```

期望 `issuer` 必须和后端 `.env` 完全一致：

```json
{
  "issuer": "http://192.168.100.100:18080/realms/nexus-pro",
  "jwks_uri": "http://192.168.100.100:18080/realms/nexus-pro/protocol/openid-connect/certs",
  "token_endpoint": "http://192.168.100.100:18080/realms/nexus-pro/protocol/openid-connect/token"
}
```

如果 `issuer` 和 `KEYCLOAK_ISSUER_URL` 不一致，后端会报 issuer mismatch。

## Client 配置

当前使用的 client：

```text
nexus-pro-connect-api
```

建议本地联调配置：

| 配置项 | 值 |
| --- | --- |
| Client ID | `nexus-pro-connect-api` |
| Client type | OpenID Connect |
| Enabled | On |
| Client authentication | Off，本地 public client |
| Standard flow | On |
| Direct access grants | On，仅用于本地 curl/smoke 测试 |

说明：

- 前端浏览器登录通常走 Authorization Code + PKCE，对应 Standard flow。
- curl 或 smoke 脚本用用户名密码直接换 token，需要 Direct access grants。
- 生产环境是否开启 Direct access grants 要单独评估，一般不建议随便打开。

## Token claim 要求

后端当前 Keycloak token resolver 会校验：

| Claim | 要求 |
| --- | --- |
| `iss` | 必须等于 `.env` 的 `KEYCLOAK_ISSUER_URL` |
| `aud` | 必须包含 `.env` 的 `KEYCLOAK_CLIENT_ID` |
| `exp` | 必须未过期 |
| `nbf` | 如果存在，必须已经生效 |
| `tenant_id` 或 `tid` | 必须存在 |
| `account_id` 或 `acct` 或 `sub` | 必须存在 |

代码位置：

- `internal/config/config.go`：读取 `KEYCLOAK_ISSUER_URL` / `KEYCLOAK_CLIENT_ID`
- `cmd/api/main.go`：两项都有值时创建 Keycloak token resolver
- `internal/platform/auth/token.go`：校验 issuer、audience、过期时间、tenant/account claim

## Protocol mapper 配置

为了让后端通过 token 校验，access token 需要包含 audience、tenant 和 account 信息。

当前本地 smoke 已临时创建过这些 mapper：

| Mapper | 类型 | 当前用途 |
| --- | --- | --- |
| `codex-smoke-audience` | Audience mapper | 把 `nexus-pro-connect-api` 写入 `aud` |
| `codex-smoke-tenant-id` | Hardcoded claim mapper | 写入 `tenant_id=demo` |
| `codex-smoke-account-id` | Hardcoded claim mapper | 写入 `account_id=acct-admin` |

这些 `codex-smoke-*` mapper 只适合本地 smoke，不适合真实多用户环境。真实环境应该改成用户属性 mapper，例如：

| Token claim | Keycloak user attribute |
| --- | --- |
| `tenant_id` | `tenant_id` |
| `account_id` | `account_id` |

也就是说，每个 Keycloak 用户配置自己的 attributes：

```text
tenant_id = demo
account_id = acct-admin
```

然后通过 User Attribute mapper 写入 access token。不要在生产环境把所有用户都 hardcode 成同一个 `tenant_id/account_id`。

### 创建 audience mapper

```bash
CLIENT_UUID=$(
  curl -sS "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/clients?clientId=nexus-pro-connect-api" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
  | jq -r ".[0].id"
)

curl -sS -o /tmp/keycloak-mapper.out -w "http=%{http_code}\n" \
  -X POST "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/clients/$CLIENT_UUID/protocol-mappers/models" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nexus-pro-audience",
    "protocol": "openid-connect",
    "protocolMapper": "oidc-audience-mapper",
    "consentRequired": false,
    "config": {
      "included.client.audience": "nexus-pro-connect-api",
      "access.token.claim": "true",
      "id.token.claim": "false"
    }
  }'
```

### 创建用户属性 mapper

`tenant_id` mapper：

```bash
curl -sS -o /tmp/keycloak-mapper.out -w "http=%{http_code}\n" \
  -X POST "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/clients/$CLIENT_UUID/protocol-mappers/models" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nexus-pro-tenant-id",
    "protocol": "openid-connect",
    "protocolMapper": "oidc-usermodel-attribute-mapper",
    "consentRequired": false,
    "config": {
      "user.attribute": "tenant_id",
      "claim.name": "tenant_id",
      "jsonType.label": "String",
      "access.token.claim": "true",
      "id.token.claim": "true",
      "userinfo.token.claim": "true"
    }
  }'
```

`account_id` mapper：

```bash
curl -sS -o /tmp/keycloak-mapper.out -w "http=%{http_code}\n" \
  -X POST "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/clients/$CLIENT_UUID/protocol-mappers/models" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nexus-pro-account-id",
    "protocol": "openid-connect",
    "protocolMapper": "oidc-usermodel-attribute-mapper",
    "consentRequired": false,
    "config": {
      "user.attribute": "account_id",
      "claim.name": "account_id",
      "jsonType.label": "String",
      "access.token.claim": "true",
      "id.token.claim": "true",
      "userinfo.token.claim": "true"
    }
  }'
```

## 创建联调用户

示例：创建一个能映射到 demo 租户和后端 demo 管理员账号的用户。

```bash
curl -sS -o /tmp/keycloak-user-create.out -w "http=%{http_code}\n" \
  -X POST "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/users" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "local-admin",
    "enabled": true,
    "email": "local-admin@example.local",
    "emailVerified": true,
    "firstName": "Local",
    "lastName": "Admin",
    "attributes": {
      "tenant_id": ["demo"],
      "account_id": ["acct-admin"]
    },
    "requiredActions": []
  }'
```

给用户设置本地测试密码：

```bash
USER_ID=$(
  curl -sS "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/users?username=local-admin&exact=true" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
  | jq -r ".[0].id"
)

read -rsp "Local test user password: " LOCAL_TEST_PASSWORD
echo

curl -sS -o /tmp/keycloak-password-reset.out -w "http=%{http_code}\n" \
  -X PUT "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/users/$USER_ID/reset-password" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"password\",\"value\":\"$LOCAL_TEST_PASSWORD\",\"temporary\":false}"
```

## 换 token 并测试后端

```bash
ACCESS_TOKEN=$(
  curl -sS -X POST "$KEYCLOAK_BASE/realms/$KEYCLOAK_REALM/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode "grant_type=password" \
    --data-urlencode "client_id=nexus-pro-connect-api" \
    --data-urlencode "username=local-admin" \
    --data-urlencode "password=$LOCAL_TEST_PASSWORD" \
  | jq -r ".access_token"
)
```

查看 token 里的关键 claim：

```bash
printf "%s\n" "$ACCESS_TOKEN" \
| jq -R 'split(".")[1] | @base64d | fromjson | {iss,aud,tenant_id,account_id,sub}'
```

请求后端：

```bash
curl -sS http://localhost:8080/v1/me \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
| jq
```

期望返回 `data.tenant.id=demo`，`data.account.id=acct-admin`。

## 常见问题

### discovery 返回 `HTTPS required`

原因：realm `sslRequired` 不是 `none`，并且当前访问 URL 是 HTTP 外部地址。

处理：

```bash
curl -sS -X PUT "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"sslRequired":"none"}'
```

生产环境不要这样做。生产应配置 HTTPS，然后使用 `https://.../realms/nexus-pro` 作为 `KEYCLOAK_ISSUER_URL`。

### 后端返回 `invalid bearer token`

按顺序检查：

1. `.env` 的 `KEYCLOAK_ISSUER_URL` 是否和 token 的 `iss` 完全一致。
2. token 的 `aud` 是否包含 `nexus-pro-connect-api`。
3. token 是否包含 `tenant_id` 或 `tid`。
4. token 是否包含 `account_id` 或 `acct`；没有时后端会退到 `sub`，但通常 `sub` 不是后端账号 ID。
5. `tenant_id/account_id` 对应的账号是否存在于后端数据库。

### token 兑换返回 `Account is not fully set up`

常见原因：

- 用户缺少必填资料，例如 first name / last name / email。
- 用户还有 required actions，例如更新密码、验证邮箱、配置用户资料。

处理方式：

```bash
curl -sS -X PUT "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/users/$USER_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "local-admin",
    "enabled": true,
    "email": "local-admin@example.local",
    "emailVerified": true,
    "firstName": "Local",
    "lastName": "Admin",
    "requiredActions": []
  }'
```

## 生产环境建议

本地可以用：

```text
sslRequired=none
KEYCLOAK_ISSUER_URL=http://192.168.100.100:18080/realms/nexus-pro
```

生产建议：

```text
sslRequired=external 或 all
KEYCLOAK_ISSUER_URL=https://<keycloak-domain>/realms/nexus-pro
```

并确认：

- 反向代理正确转发 `X-Forwarded-Proto=https`。
- Keycloak hostname/front-end URL 配置和外部访问域名一致。
- OIDC discovery 返回的 `issuer` 与后端 `KEYCLOAK_ISSUER_URL` 完全一致。
- 不使用 hardcoded `tenant_id/account_id` mapper。
