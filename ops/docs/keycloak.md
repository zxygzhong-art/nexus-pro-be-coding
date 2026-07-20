# Keycloak 配置指南

本指南說明如何在 Nexus Pro 項目中部署、配置 Keycloak，並將其與前端（`nexus-pro-fe`）和後端（`nexus-pro-api`）對接。

所有基礎設施相關的可調項統一在 [`ops/.env`](../.env)。應用層環境變量分別寫在 `nexus-pro-api/.env` 與 `nexus-pro-fe/.env.local`。

## 架構概覽

```text
┌─────────────┐     BFF (app/api/auth/*)      ┌──────────────┐
│ nexus-pro-fe│ ─────────────────────────────▶│   Keycloak   │
│  (Next.js)  │  password / auth-code / SSO │   (OIDC)     │
└──────┬──────┘                               └──────┬───────┘
       │ httpOnly cookie (_t / _rt)                    │
       │ proxy.ts 注入 Authorization: Bearer           │ Admin API
       ▼                                               │ (可選)
┌─────────────┐                                        ▼
│ nexus-pro-api│◀────────────────────────────── 用戶開通 / 邀請
│   (Go API)  │  校驗 JWT（iss / aud / tenant_id / sub）
└─────────────┘
```

職責劃分：

| 組件 | 職責 |
| --- | --- |
| Keycloak | 登錄憑證、OIDC token 簽發、社交登錄、忘記密碼 |
| 前端 BFF | 與 Keycloak 交換 token，寫入 httpOnly cookie，不暴露 token 給瀏覽器 JS |
| 後端 API | 校驗 Bearer token，從 claims 解析 `tenant_id` 與 `account_id`；可選通過 Admin API 開通用戶 |

## 1. 啓動 Keycloak

### 完整觀測棧（含 PostgreSQL）

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-api/ops
./render-configs.sh
docker compose --env-file .env up -d
```

### 僅啓動 Keycloak

PostgreSQL 已就緒時：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-api/ops
COMPOSE_PROFILES=keycloak docker compose --env-file .env up -d --no-deps keycloak
```

### 本地默認訪問地址

| 用途 | 地址 |
| --- | --- |
| Admin Console / OIDC | `http://127.0.0.1:8080` |
| Health / Metrics | `http://127.0.0.1:24990` |

默認 bootstrap 管理員（僅本地開發）：

| 賬號 | 密碼 |
| --- | --- |
| `admin` | `admin` |

> 正式環境必須更換 `KEYCLOAK_ADMIN_PASSWORD`，並將 `KEYCLOAK_COMMAND` 從 `start-dev` 改爲生產模式（`start` + 反向代理 / TLS 配置）。

### 相關 `ops/.env` 變量

| 變量 | 默認值 | 說明 |
| --- | --- | --- |
| `KEYCLOAK_IMAGE` | `quay.io/keycloak/keycloak:26.7.0` | 固定版本，不使用 `latest` |
| `KEYCLOAK_HTTP_HOST_PORT` | `8080` | 對宿主機的 HTTP 端口 |
| `KEYCLOAK_COMMAND` | `start-dev` | 本地開發模式 |
| `KEYCLOAK_DB_NAME` | `keycloak` | PostgreSQL 數據庫名（initdb 自動創建） |
| `KEYCLOAK_FEATURES` | `opentelemetry` | 啓用 OpenTelemetry |
| `KEYCLOAK_TRACING_ENABLED` | `true` | trace 上報到 `tempo` |

## 2. 創建 Realm

1. 打開 Admin Console：`http://127.0.0.1:8080`
2. 左上角下拉 → **Create realm**
3. 填寫：
   - **Realm name**：`nexus-pro`（與項目約定一致）
4. 保存

後續所有配置都在 `nexus-pro` realm 下進行。

**Issuer URL**（前後端必須一致，無尾部斜槓）：

```text
http://127.0.0.1:8080/realms/nexus-pro
```

若通過反向代理或不同端口對外暴露，請使用瀏覽器和後端實際訪問的地址。例如 `.env.example` 中的 `http://127.0.0.1:8080/realms/nexus-pro` 表示宿主映射到了 `8080`——關鍵是 **issuer 與 discovery 端點可達且一致**。

驗證 discovery 端點：

```bash
curl -s http://127.0.0.1:8080/realms/nexus-pro/.well-known/openid-configuration | jq .issuer
```

## 3. 步驟一：配置登錄 Client `nexus-pro-connect-api`

項目約定 client id 爲 **`nexus-pro-connect-api`**，同時服務於前端 BFF 和後端 token 校驗。

路徑：**Clients → Create client**

### 3.1 基本設置

| 字段 | 值 |
| --- | --- |
| Client type | OpenID Connect |
| Client ID | `nexus-pro-connect-api` |
| Client authentication | 視部署選擇（見下） |

### 3.2 Capability config

| 開關 | 是否開啓 | 原因 |
| --- | --- | --- |
| **Standard flow** | ✅ | SSO / 授權碼 + PKCE 回調 |
| **Direct access grants** | ✅ | 郵箱 + 密碼登錄（`/api/auth/login`） |
| **Implicit flow** | ❌ | 已廢棄 |
| **Service accounts roles** | ❌ | 應用 client 不需要 |

### 3.3 Login settings

| 字段 | 本地開發示例 |
| --- | --- |
| Valid redirect URIs | `http://localhost:3002/api/auth/keycloak/callback`, `http://127.0.0.1:3002/api/auth/keycloak/callback` |
| Valid post logout redirect URIs | `http://localhost:3002/*`, `http://127.0.0.1:3002/*` |
| Web origins | `http://localhost:3002`, `http://127.0.0.1:3002` |

正式環境替換爲實際前端域名，例如 `https://app.example.com/api/auth/keycloak/callback`。

### 3.4 Public vs Confidential

| 類型 | 適用場景 | 前端環境變量 |
| --- | --- | --- |
| **Public** | 本地開發、純 PKCE 流程 | `KEYCLOAK_CLIENT_SECRET` 留空 |
| **Confidential** | 需要 client secret 保護 token 端點 | 填寫 `KEYCLOAK_CLIENT_SECRET` |

前端 `exchangeKeycloakToken` 在配置了 secret 時會自動附帶 `client_secret`。

### 3.5 Protocol Mappers（關鍵）

後端校驗 access token 時，除標準 OIDC claims 外，**必須**包含：

| Claim | 別名 | 用途 |
| --- | --- | --- |
| `tenant_id` | `tid`, `tenant_hint` | 多租戶隔離 |
| `sub` | `account_id`, `acct` | 用戶身份綁定 |

開通用戶時，後端 Admin API 會把 `tenant_id`、`account_id` 寫入 Keycloak 用戶 attributes。需要通過 Protocol Mapper 映射到 token。

在 **`nexus-pro-connect-api` → Client scopes → Dedicated scope → Add mapper → By configuration → User Attribute`** 分別添加：

#### Mapper 1：`tenant_id`

| 字段 | 值 |
| --- | --- |
| Name | `tenant_id` |
| User Attribute | `tenant_id` |
| Token Claim Name | `tenant_id` |
| Claim JSON Type | String |
| Add to ID token | On |
| Add to access token | **On** |
| Add to userinfo | On |

#### Mapper 2：`account_id`

| 字段 | 值 |
| --- | --- |
| Name | `account_id` |
| User Attribute | `account_id` |
| Token Claim Name | `account_id` |
| Claim JSON Type | String |
| Add to ID token | On |
| Add to access token | **On** |
| Add to userinfo | On |

#### Audience 說明

後端還會校驗 `aud` 包含 `KEYCLOAK_CLIENT_ID`（即 `nexus-pro-connect-api`）。Keycloak 默認會爲目標 client 簽發 audience，一般無需額外 mapper。若自定義了 audience 行爲，確保 access token 的 `aud` 包含該 client id。

## 4. 步驟二：配置管理 Client `nexus-pro-admin`

當 `KEYCLOAK_PROVISION_USERS=true` 時，後端通過 Keycloak Admin API 在員工創建 / 導入 / 邀請時自動開通用戶。需要一個 **Service Account** client。

路徑：**Clients → Create client**

| 字段 | 值 |
| --- | --- |
| Client ID | `nexus-pro-admin`（或自定義，與後端環境變量一致） |
| Client authentication | **On** |
| Service accounts roles | **On** |
| Standard flow | Off |
| Direct access grants | Off |

創建後：

1. 進入 **Service account roles** 標籤
2. **Assign role** → Filter by clients → 選擇 `realm-management`
3. 只勾選 **`manage-users`**（創建 / 查詢 / 更新用戶、修改密碼、發送 required actions 郵件）
4. 不要勾選 `manage-realm` 或 `realm-admin`；當前業務不需要修改整個 realm

在 **Credentials** 標籤複製 **Client secret**，填入後端：

```bash
KEYCLOAK_ADMIN_CLIENT_ID=nexus-pro-admin
KEYCLOAK_ADMIN_CLIENT_SECRET=<secret>
```

Admin client 使用 `client_credentials` grant 獲取 token（見 `internal/platform/auth/keycloak_admin.go`）。

`KEYCLOAK_ADMIN_CLIENT_ID` 與 `KEYCLOAK_ADMIN_CLIENT_SECRET` 也用於當前用戶在 Nexus 設置彈窗內變更密碼；此流程先用 `KEYCLOAK_CLIENT_ID` 驗證當前密碼，再只更新登錄賬號綁定的 Keycloak subject。即使 `KEYCLOAK_PROVISION_USERS=false`，要啓用彈窗內改密仍需配置這兩個 Admin client 變量。

### 4.1 驗證 Admin Client

加載後端環境變量，然後用 `client_credentials` 獲取 token：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-api
set -a
source ./.env
set +a

ADMIN_TOKEN_JSON=$(curl -sS -X POST \
  "$KEYCLOAK_BASE_URL/protocol/openid-connect/token" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "client_id=$KEYCLOAK_ADMIN_CLIENT_ID" \
  --data-urlencode "client_secret=$KEYCLOAK_ADMIN_CLIENT_SECRET")

jq '{token_type, expires_in, error, error_description}' <<<"$ADMIN_TOKEN_JSON"
```

成功時 `token_type` 爲 `Bearer`，且 `error` 爲空。繼續驗證 `manage-users` 權限：

```bash
ADMIN_TOKEN=$(jq -r '.access_token // empty' <<<"$ADMIN_TOKEN_JSON")
KEYCLOAK_ROOT="${KEYCLOAK_BASE_URL%%/realms/*}"

curl -sS -o /dev/null \
  -w 'Admin API HTTP %{http_code}\n' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$KEYCLOAK_ROOT/admin/realms/nexus-pro/users?max=1"
```

返回 `200` 表示 Client ID、Client secret 和角色均可用；`401` 通常是 client 憑證或 realm 錯誤，`403` 通常是 `manage-users` 未正確分配。

## 5. 步驟三：配置邀請 Client 與郵件

邀請流程不需要再創建第三個 Keycloak client。`KEYCLOAK_INVITE_CLIENT_ID` 表示邀請鏈接使用哪個登錄 client，推薦直接複用步驟一的 `nexus-pro-connect-api`；真正調用 Keycloak Admin API 發送郵件的仍是步驟二的 `nexus-pro-admin`。

後端邀請用戶時會添加 `UPDATE_PASSWORD` required action。僅當 `KEYCLOAK_SEND_INVITE_EMAIL=true` 時，後端纔會調用 Keycloak 的 `execute-actions-email`；郵件鏈接有效期爲 24 小時。

### 5.1 暫不發送邀請郵件

本地尚未配置 SMTP 時使用：

```bash
KEYCLOAK_SEND_INVITE_EMAIL=false
KEYCLOAK_INVITE_CLIENT_ID=nexus-pro-connect-api
KEYCLOAK_INVITE_REDIRECT_URL=
```

這種模式下 Keycloak 不發送郵件。管理員需要在 **Users → 目標用戶 → Credentials** 中手動設置初始密碼；建議開啓 **Temporary**，讓用戶首次登錄時修改密碼。

### 5.2 啓用邀請郵件

本地開發配置：

```bash
KEYCLOAK_PROVISION_USERS=true
KEYCLOAK_SEND_INVITE_EMAIL=true
KEYCLOAK_INVITE_CLIENT_ID=nexus-pro-connect-api
KEYCLOAK_INVITE_REDIRECT_URL=http://localhost:3002/login
```

還需要在 Keycloak 完成以下配置：

1. **Realm settings → Email**：配置 SMTP，並使用測試連接功能確認發送成功
2. **Clients → nexus-pro-connect-api → Settings → Valid redirect URIs**：增加 `http://localhost:3002/login`
3. 若使用 `127.0.0.1` 訪問前端，再增加 `http://127.0.0.1:3002/login`
4. 確認 `nexus-pro-admin` 已按步驟二分配 `realm-management → manage-users`

生產環境必須替換爲實際 HTTPS 前端地址，並在 `Valid redirect URIs` 中註冊完全一致的地址：

```bash
KEYCLOAK_PROVISION_USERS=true
KEYCLOAK_SEND_INVITE_EMAIL=true
KEYCLOAK_INVITE_CLIENT_ID=nexus-pro-connect-api
KEYCLOAK_INVITE_REDIRECT_URL=https://app.example.com/login
```

`KEYCLOAK_SEND_INVITE_EMAIL=true` 依賴 `KEYCLOAK_PROVISION_USERS=true`。`KEYCLOAK_INVITE_CLIENT_ID` 留空時後端會回退到 `KEYCLOAK_CLIENT_ID`，但建議顯式填寫，方便部署檢查。

## 6. 創建測試用戶

用於本地聯調與 smoke test。每個用戶的 attributes 必須包含 `tenant_id` 和 `account_id`。

路徑：**Users → Add user**

| 字段 | admin 示例 | employee 示例 |
| --- | --- | --- |
| Username | `local-admin` | `local-employee` |
| Email | `admin@example.com` | `employee@example.com` |
| Email verified | On | On |

保存後進入 **Credentials** 標籤設置密碼（關閉 Temporary）。

進入 **Attributes** 標籤：

**admin 用戶：**

| Key | Value |
| --- | --- |
| `tenant_id` | `demo` |
| `account_id` | `acct-admin` |

**employee 用戶：**

| Key | Value |
| --- | --- |
| `tenant_id` | `demo` |
| `account_id` | `acct-employee` |

這些值須與後端數據庫 `accounts` / `user_identities` 表中的記錄一致。Smoke test 約定見 [`tools/api-smoke/README.md`](../../tools/api-smoke/README.md)。

### 驗證 token claims

```bash
TOKEN=$(curl -s -X POST "http://127.0.0.1:8080/realms/nexus-pro/protocol/openid-connect/token" \
  -d "grant_type=password" \
  -d "client_id=nexus-pro-connect-api" \
  -d "username=local-admin" \
  -d "password=<password>" \
  -d "scope=openid profile email" \
  | jq -r .access_token)

# 解碼 payload（僅用於調試）
echo "$TOKEN" | cut -d. -f2 | base64 -d 2>/dev/null | jq .
```

確認輸出包含 `"tenant_id": "demo"` 和 `"account_id": "acct-admin"`（或 `"sub"` 有值）。

## 7. 社交登錄（可選）

前端支持 Google / Microsoft SSO，通過 Keycloak Identity Provider 中轉。

路徑：**Identity providers → Add provider**

| Provider | 前端參數 | 默認 IdP alias |
| --- | --- | --- |
| Google | `/api/auth/oidc/google/authorize` | `google` |
| Microsoft | `/api/auth/oidc/microsoft/authorize` | `microsoft` |

在 IdP 配置中填寫 OAuth client id / secret。若 alias 不是默認值，在前端設置：

```bash
KEYCLOAK_GOOGLE_IDP_ALIAS=google
KEYCLOAK_MICROSOFT_IDP_ALIAS=microsoft
```

SSO 流程使用授權碼 + PKCE，回調地址爲 `/api/auth/keycloak/callback`，需在應用 client 的 Valid redirect URIs 中允許。

## 8. 忘記密碼

前端 `/api/auth/reset-password` 會 302 重定向到 Keycloak 託管流程：

```text
{issuer}/login-actions/reset-credentials?client_id=nexus-pro-connect-api
```

要真正發送重置郵件，還需在 Keycloak 配置 SMTP：

路徑：**Realm settings → Email**

填寫 SMTP 服務器、發件人地址，並在 **Realm settings → Login** 中啓用 **Forgot password**。

## 9. 應用環境變量

### 9.1 前端（`nexus-pro-fe/.env.local`）

```bash
# 必填（認證路由執行期校驗）
KEYCLOAK_BASE_URL=http://127.0.0.1:8080/realms/nexus-pro
KEYCLOAK_CLIENT_ID=nexus-pro-connect-api

# confidential client 才需要
KEYCLOAK_CLIENT_SECRET=

# 可選，默認 openid profile email
KEYCLOAK_SCOPE=openid profile email

# 社交登錄 IdP alias
KEYCLOAK_GOOGLE_IDP_ALIAS=google
KEYCLOAK_MICROSOFT_IDP_ALIAS=microsoft
```

認證流程說明：

| 路由 | Grant type | 前置條件 |
| --- | --- | --- |
| `POST /api/auth/login` | `password` | Direct access grants 已開啓 |
| `GET /api/auth/oidc/{provider}/authorize` | 授權碼 + PKCE | Standard flow 已開啓 |
| `GET /api/auth/keycloak/callback` | `authorization_code` | redirect URI 已註冊 |
| `POST /api/auth/refresh` | `refresh_token` | 登錄時寫入了 `_rt` cookie |
| `GET /api/auth/reset-password` | — | 重定向到 Keycloak reset-credentials |

Token 存儲：

- `_t`：access token（httpOnly，path `/`，壽命對齊 `expires_in`）
- `_rt`：refresh token（httpOnly，path `/api/auth`）
- `_session`：會話標記，供 `proxy.ts` 判斷登錄態

### 9.2 後端（`nexus-pro-api/.env`）

**基礎 OIDC 校驗（必填，生產環境啓動時強制）：**

```bash
KEYCLOAK_BASE_URL=http://127.0.0.1:8080/realms/nexus-pro
KEYCLOAK_CLIENT_ID=nexus-pro-connect-api
```

**用戶開通（可選）：**

```bash
KEYCLOAK_PROVISION_USERS=true
KEYCLOAK_ADMIN_CLIENT_ID=nexus-pro-admin
KEYCLOAK_ADMIN_CLIENT_SECRET=<secret>

# 邀請郵件關閉時（啓用方式見步驟三）
KEYCLOAK_SEND_INVITE_EMAIL=false
KEYCLOAK_INVITE_CLIENT_ID=nexus-pro-connect-api
KEYCLOAK_INVITE_REDIRECT_URL=
```

當 `KEYCLOAK_PROVISION_USERS=true` 時，員工創建 / 導入 / 邀請會：

1. 通過 Admin API 在 Keycloak 創建或更新用戶
2. 寫入 attributes：`tenant_id`、`account_id`、`employee_id`、`employee_no`
3. 將 Keycloak `sub` 綁定到 `user_identities` 表

## 10. 端到端驗證

### 10.1 前端登錄

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-fe
pnpm dev
```

瀏覽器打開 `http://localhost:3002/login`，使用測試賬號登錄。成功後在 DevTools → Application → Cookies 中應看到 `_t`、`_rt`、`_session`。

### 10.2 後端 API

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-api
# 確保 KEYCLOAK_BASE_URL / KEYCLOAK_CLIENT_ID 已設置
go run ./cmd/api
```

```bash
curl -s http://localhost:18080/healthz
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:18080/v1/...
```

### 10.3 Smoke test

```bash
export SMOKE_KEYCLOAK_BASE_URL="http://127.0.0.1:8080/realms/nexus-pro"
export SMOKE_KEYCLOAK_CLIENT_ID="nexus-pro-connect-api"
export SMOKE_ADMIN_USERNAME="local-admin"
export SMOKE_ADMIN_PASSWORD="..."
tools/api-smoke/full_api_smoke.py
```

## 11. 生產環境檢查清單

- [ ] `KEYCLOAK_COMMAND=start`（非 `start-dev`）
- [ ] 管理員密碼、client secret 已輪換
- [ ] `KEYCLOAK_BASE_URL` 使用 `https://` 公網地址
- [ ] redirect URI / Web origins 僅包含正式域名
- [ ] SMTP 已配置（若啓用邀請郵件或忘記密碼）
- [ ] 邀請郵件啓用時，`KEYCLOAK_INVITE_CLIENT_ID=nexus-pro-connect-api`
- [ ] 邀請郵件啓用時，HTTPS `KEYCLOAK_INVITE_REDIRECT_URL` 已註冊到登錄 client 的 Valid redirect URIs
- [ ] Direct access grants 按安全策略評估（密碼登錄依賴此開關；可僅保留 SSO）
- [ ] Protocol Mapper `tenant_id` / `account_id` 已配置
- [ ] Admin client 的 `manage-users` 權限已最小化授予
- [ ] 後端 `KEYCLOAK_PROVISION_USERS` 與業務流程一致

## 12. 常見問題

### 登錄返回 401「Keycloak 登入失敗」

1. 確認 client 已開啓 **Direct access grants**
2. 確認用戶名 / 密碼正確，用戶未被禁用
3. 用 curl 直接打 token 端點排查（見第 6 節）

### 後端返回 401「invalid bearer token」

1. 檢查 `KEYCLOAK_BASE_URL` 與 token 中 `iss` 完全一致（無尾部 `/`）
2. 檢查 token `aud` 是否包含 `KEYCLOAK_CLIENT_ID`
3. 檢查 token 是否包含 `tenant_id` 和 `sub`（或 `account_id`）——通常是 **Protocol Mapper 未配置**
4. 檢查 token 是否過期

### SSO 回調「登入狀態已失效」

1. 確認 `nexus_keycloak_state` cookie 在回調時仍存在（同站、10 分鐘有效期）
2. 確認 redirect URI 與 Keycloak client 配置完全匹配
3. 確認前端 origin 與 Web origins 一致

### Admin API 開通用戶失敗

1. 確認 `KEYCLOAK_ADMIN_CLIENT_ID` / `SECRET` 正確
2. 確認 service account 已分配 `realm-management → manage-users`
3. 確認 `KEYCLOAK_BASE_URL` 格式爲 `http(s)://host/realms/nexus-pro`

### 邀請郵件未發送或返回 400

1. 確認 `KEYCLOAK_PROVISION_USERS=true` 且 `KEYCLOAK_SEND_INVITE_EMAIL=true`
2. 確認 Keycloak Realm SMTP 已配置並通過測試
3. 確認 `KEYCLOAK_INVITE_CLIENT_ID=nexus-pro-connect-api`
4. 確認 `KEYCLOAK_INVITE_REDIRECT_URL` 與該 client 的 Valid redirect URIs 完全一致
5. 確認 `nexus-pro-admin` 只需並且已經擁有 `realm-management → manage-users`

### Issuer 端口不一致

`ops/.env` 默認映射 `8080`，而 `nexus-pro-api/.env.example` 示例爲 `18080`。兩者不矛盾——選你實際對外暴露的地址即可，前後端必須使用同一個 issuer。

## 相關文件

| 文件 | 說明 |
| --- | --- |
| [`ops/.env`](../.env) | Keycloak 容器部署參數 |
| [`ops/compose.yaml`](../compose.yaml) | Keycloak compose 服務定義 |
| [`nexus-pro-api/.env.example`](../../.env.example) | 後端 Keycloak 環境變量 |
| [`nexus-pro-fe/.env.example`](../../../nexus-pro-fe/.env.example) | 前端 Keycloak 環境變量 |
| [`nexus-pro-fe/app/api/auth/_keycloak.ts`](../../../nexus-pro-fe/app/api/auth/_keycloak.ts) | 前端 BFF Keycloak 集成 |
| [`nexus-pro-api/internal/platform/auth/token.go`](../../internal/platform/auth/token.go) | 後端 JWT 校驗 |
| [`nexus-pro-api/internal/platform/auth/keycloak_admin.go`](../../internal/platform/auth/keycloak_admin.go) | 後端 Admin API 用戶開通 |
