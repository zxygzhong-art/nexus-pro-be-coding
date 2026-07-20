# QA 測試賬號生成腳本

爲 QA 測試生成一套**可真實登錄**的測試賬號（Keycloak 用戶 + 後端 DB 綁定），覆蓋不同權限集與不同賬號/員工狀態，用於模擬真實用戶測試各頁面的權限邊界。

## 賬號矩陣

統一密碼：`QaTest123!`（可用 `QA_PASSWORD` 覆蓋），租戶默認 `qa`（`QA_TENANT_ID` 覆蓋）。

| 賬號 | 權限集 | 賬號狀態 | 員工狀態 | 用途 |
|---|---|---|---|---|
| `qa-superadmin@qa.test` | Platform Admin（`*.*` + 顯式頁面投影） | active | active | 全部頁面基準對照 |
| `qa-hr@qa.test` | HR Admin（hr.employee.\* + hr.org_unit.\*） | active | active | 員工管理/組織架構/在職分析可見；考勤、表單設計、管理員、審計應被攔 |
| `qa-attendance@qa.test` | 考勤管理（clock/correction/leave read+approve） | active | active | 工時統計/打卡時間/假勤制度；補卡審批 |
| `qa-approver@qa.test` | 表單審批人（form_instance approve） | active | active | 待辦審核覈準/駁回/退回；是 qa-employee 的主管 |
| `qa-employee@qa.test` | 普通員工（self scope） | active | active | 打卡、請假、提交表單；workspace 全部 403 |
| `qa-audit@qa.test` | 僅審計（audit.log.read） | active | active | workspace 僅操作紀錄可見 |
| `qa-noperm@qa.test` | 僅 me.read | active | active | 能登錄進主頁，業務 API 全 403 |
| `qa-disabled@qa.test` | 員工權限 | **disabled** | active | Keycloak 出 token，後端應 401 `account_inactive` |
| `qa-pending@qa.test` | 員工權限 | **pending_invite** | onboarding | 同上 |
| `qa-resigned@qa.test` | 員工權限 | active | **resigned** | 邊界：離職員工還能否打卡/請假 |
| `qa-kc-only@qa.test` | —（無 DB 綁定） | — | — | 邊界：Keycloak 有用戶但後端無 `user_identities`，應 401 identity not linked |

## 前置條件

1. Postgres 已遷移：`make migrate-up DB_HOST=... DB_USERNAME=... DB_NAME=...`
2. Keycloak 已啓動，realm `nexus-pro` 與 client `nexus-pro-connect-api` 已按 `ops/docs/keycloak.md` 配置，且 client 開啓 **Direct Access Grants**（ROPC）。
   - 缺少的 protocol mappers（`tenant_id`/`account_id` 等 attribute → claim）腳本會自動補建。
3. 本機有 `psql`、Python 3.9+（僅標準庫）與 Go toolchain。腳本會複用 `tenantctl` 冪等補齊六個內建表單模板，不覆蓋同 key 的租戶自定義模板。

## 使用

```bash
cd nexus-pro-api/tools/qa-accounts

export DB_HOST=127.0.0.1 DB_PORT=5432 DB_USERNAME=nexus DB_PASSWORD=nexus DB_NAME=nexus_pro_be DB_SSLMODE=disable
export KEYCLOAK_BASE_URL='http://127.0.0.1:8080'
export API_BASE_URL='http://127.0.0.1:18080'   # 可選：附帶 GET /v1/me 驗證

./provision_qa_accounts.py                # 創建 + 自動驗證
./provision_qa_accounts.py --print-matrix # 只看賬號矩陣
./provision_qa_accounts.py --verify-only  # 只跑登錄驗證
```

腳本是**冪等**的：重複執行會更新 Keycloak 用戶與密碼、覆蓋權限集與賬號狀態，可放心重跑（比如手工改壞了狀態後一鍵還原）。

驗證階段會對每個賬號做 ROPC 登錄取 token；配置了 `API_BASE_URL` 時再調 `GET /v1/me`，並按每個賬號的預期（正常 200 / disabled、pending、kc-only 應 401/403）斷言，有不符會以非零退出碼結束。

## 前端登錄

前端 `nexus-pro-fe` 的 `/login` 頁直接用 email + 密碼登錄即可（BFF 走同一個 ROPC client）。注意前端 `.env.local` 需設置：

```bash
KEYCLOAK_BASE_URL=http://127.0.0.1:8080/realms/nexus-pro
KEYCLOAK_CLIENT_ID=nexus-pro-connect-api
NEXUS_API_BASE_URL=http://127.0.0.1:18080   # 走真實後端而非 mock
```

## 環境變量一覽

| 變量 | 默認 | 說明 |
|---|---|---|
| `DB_HOST` / `DB_USERNAME` / `DB_NAME` | （必填） | Postgres 連接字段 |
| `KEYCLOAK_BASE_URL` | `http://127.0.0.1:8080` | Keycloak 地址 |
| `KEYCLOAK_REALM` | `nexus-pro` | realm |
| `KEYCLOAK_ADMIN_USER/PASS` | `admin`/`admin` | master realm 管理員（見 ops/local-credentials.md） |
| `KEYCLOAK_CLIENT_ID` | `nexus-pro-connect-api` | ROPC client |
| `KEYCLOAK_CLIENT_SECRET` | 空 | confidential client 時填寫 |
| `QA_TENANT_ID` | `qa` | 測試租戶 id（換成 `qa2` 可再造一套做跨租戶隔離測試）；非默認租戶的權限集合 ID 會自動附加租戶後綴，避免全局主鍵衝突 |
| `QA_PASSWORD` | `QaTest123!` | 所有賬號統一密碼 |
| `API_BASE_URL` | 空 | 填了則驗證階段調 `/v1/me` |

## 跨租戶隔離測試

用不同 `QA_TENANT_ID` 跑兩次（如 `qa` 與 `qa2`），即可用 `qa2` 的 token 訪問 `qa` 的資源驗證租戶隔離（應 403/404）。
