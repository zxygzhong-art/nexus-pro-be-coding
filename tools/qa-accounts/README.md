# QA 测试账号生成脚本

为 QA 测试生成一套**可真实登录**的测试账号（Keycloak 用户 + 后端 DB 绑定），覆盖不同权限集与不同账号/员工状态，用于模拟真实用户测试各页面的权限边界。

## 账号矩阵

统一密码：`QaTest123!`（可用 `QA_PASSWORD` 覆盖），租户默认 `qa`（`QA_TENANT_ID` 覆盖）。

| 账号 | 权限集 | 账号状态 | 员工状态 | 用途 |
|---|---|---|---|---|
| `qa-superadmin@qa.test` | Platform Admin（`*.*`） | active | active | 全部页面基准对照 |
| `qa-hr@qa.test` | HR Admin（hr.employee.\* + hr.org_unit.\*） | active | active | 员工管理/組織架構/在職分析可见；考勤、表單設計、管理員、審計应被拦 |
| `qa-attendance@qa.test` | 考勤管理（clock/correction/leave read+approve） | active | active | 工時統計/打卡時間/假勤制度；补卡审批 |
| `qa-approver@qa.test` | 表单审批人（form_instance approve） | active | active | 待辦審核核准/驳回/退回；是 qa-employee 的主管 |
| `qa-employee@qa.test` | 普通员工（self scope） | active | active | 打卡、请假、提交表单；workspace 全部 403 |
| `qa-audit@qa.test` | 仅审计（audit.log.read） | active | active | workspace 仅操作紀錄可见 |
| `qa-noperm@qa.test` | 仅 me.read | active | active | 能登录进主页，业务 API 全 403 |
| `qa-disabled@qa.test` | 员工权限 | **disabled** | active | Keycloak 出 token，后端应 401 `account_inactive` |
| `qa-pending@qa.test` | 员工权限 | **pending_invite** | onboarding | 同上 |
| `qa-resigned@qa.test` | 员工权限 | active | **resigned** | 边界：离职员工还能否打卡/请假 |
| `qa-kc-only@qa.test` | —（无 DB 绑定） | — | — | 边界：Keycloak 有用户但后端无 `user_identities`，应 401 identity not linked |

## 前置条件

1. Postgres 已迁移：`make migrate-up DATABASE_URL=...`
2. Keycloak 已启动，realm `nexus-pro` 与 client `nexus-pro-connect-api` 已按 `ops/docs/keycloak.md` 配置，且 client 开启 **Direct Access Grants**（ROPC）。
   - 缺少的 protocol mappers（`tenant_id`/`account_id` 等 attribute → claim）脚本会自动补建。
3. 本机有 `psql` 与 Python 3.9+（仅标准库）。

## 使用

```bash
cd nexus-pro-be/tools/qa-accounts

export DATABASE_URL='postgres://nexus:nexus@127.0.0.1:5432/nexus_pro_be?sslmode=disable'
export KEYCLOAK_BASE_URL='http://127.0.0.1:8080'
export API_BASE_URL='http://127.0.0.1:18080'   # 可选：附带 GET /v1/me 验证

./provision_qa_accounts.py                # 创建 + 自动验证
./provision_qa_accounts.py --print-matrix # 只看账号矩阵
./provision_qa_accounts.py --verify-only  # 只跑登录验证
```

脚本是**幂等**的：重复执行会更新 Keycloak 用户与密码、覆盖权限集与账号状态，可放心重跑（比如手工改坏了状态后一键还原）。

验证阶段会对每个账号做 ROPC 登录取 token；配置了 `API_BASE_URL` 时再调 `GET /v1/me`，并按每个账号的预期（正常 200 / disabled、pending、kc-only 应 401/403）断言，有不符会以非零退出码结束。

## 前端登录

前端 `nexus-pro-fe` 的 `/login` 页直接用 email + 密码登录即可（BFF 走同一个 ROPC client）。注意前端 `.env.local` 需设置：

```bash
KEYCLOAK_ISSUER_URL=http://127.0.0.1:8080/realms/nexus-pro
KEYCLOAK_CLIENT_ID=nexus-pro-connect-api
NEXUS_API_PROXY_TARGET=http://127.0.0.1:18080   # 走真实后端而非 mock
```

## 环境变量一览

| 变量 | 默认 | 说明 |
|---|---|---|
| `DATABASE_URL` | （必填） | Postgres 连接串 |
| `KEYCLOAK_BASE_URL` | `http://127.0.0.1:8080` | Keycloak 地址 |
| `KEYCLOAK_REALM` | `nexus-pro` | realm |
| `KEYCLOAK_ADMIN_USER/PASS` | `admin`/`admin` | master realm 管理员（见 ops/local-credentials.md） |
| `KEYCLOAK_CLIENT_ID` | `nexus-pro-connect-api` | ROPC client |
| `KEYCLOAK_CLIENT_SECRET` | 空 | confidential client 时填写 |
| `QA_TENANT_ID` | `qa` | 测试租户 id（换成 `qa2` 可再造一套做跨租户隔离测试） |
| `QA_PASSWORD` | `QaTest123!` | 所有账号统一密码 |
| `API_BASE_URL` | 空 | 填了则验证阶段调 `/v1/me` |

## 跨租户隔离测试

用不同 `QA_TENANT_ID` 跑两次（如 `qa` 与 `qa2`），即可用 `qa2` 的 token 访问 `qa` 的资源验证租户隔离（应 403/404）。
