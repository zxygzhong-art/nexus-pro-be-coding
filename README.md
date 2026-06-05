# nexus-pro-be

多租户 SaaS HR 平台（「产销人发财」人域）的 Go 主业务后端。本仓库当前阶段聚焦
**数据库表设计 + 权限底座（IAM）+ 项目框架脚手架**，不实现具体 HR 业务逻辑。

架构与权限模型参见 `platform-ui/docs/architecture.md` 与 `permission-architecture.md`。

## 技术栈

- **Gin** HTTP 框架 · **GORM** ORM · **golang-migrate** 迁移
- **PostgreSQL + Row Level Security**（租户隔离）
- 外部基建以**接口 + 适配器桩**接入：OpenFGA（鉴权 ReBAC，预留）、Keycloak（OIDC，预留）、Redis（权限快照缓存，预留）
- 当前鉴权由**本地权限引擎**（`internal/authz`）计算，可通过 `AUTHZ_BACKEND=openfga` 切换到 OpenFGA（桩）

## 目录结构

```
cmd/api          HTTP 服务入口（依赖注入总装）
cmd/migrate      golang-migrate CLI 封装
internal/
  config         环境变量配置
  server         Gin 引擎与路由
  middleware     recovery / cors / requestid / principal(开租户事务) / authz(鉴权+审计)
  db             GORM 连接、RLS 租户事务（SET LOCAL app.current_tenant）
  models         iam_* 表的 GORM 模型
  repository     数据访问，实现 authz.DataSource + 管理查询
  authz          本地权限引擎：effective=(直接∪组∪承担)−deny ∩ boundary，scope 交集
  adapters       authorizer(OpenFGA 桩) / identity(header + keycloak 桩) / cache(noop + redis)
  audit          审计记录器（独立事务，拒绝时也留痕）
  iam            service（capabilities / assume）+ handler（runtime + IAM 管理 + 兼容桩）
  hr             HR 业务域骨架（employee / org_unit），service+handler，仅占位不含业务逻辑
migrations       000001..000016（schema + RLS + version 触发器 + 种子 + hr_core 表 + hr 权限）
deploy           docker-compose / Dockerfile / openfga model / keycloak
```

## 本地运行

```bash
make db-up         # 启动 postgres + redis（Docker）
make migrate-up    # 应用迁移（建表 + RLS + app_user + 种子数据）
make run           # 启动 API，监听 :8088
```

迁移以 owner 角色（`MIGRATE_DSN`）执行；应用以无 BYPASSRLS 的 `app_user`（`DB_DSN`）连接，
因此 RLS 始终生效。复制 `.env.example` 为 `.env` 调整配置。

完整栈（含 openfga / keycloak / 一次性 migrate / api）：

```bash
make compose-up
```

## 运行时接口（与现有前端契约兼容）

- `GET /healthz`
- `GET /v1/me` — 账号 + 用户组 + capabilities
- `GET /v1/me/menus` — 按权限裁剪的菜单
- `POST /v1/authz/check` · `POST /v1/authz/batch-check` · `POST /v1/authz/explain` · `POST /v1/authz/simulate`

## IAM 管理接口（`iam.*` 权限门禁）

`/v1/iam/applications` `resource-types` `permissions` `user-groups` `permission-sets`
`permission-set-assignments` `field-policies` `data-scopes` `assumable-roles`
`assumable-roles/:id/assume` `roles`(兼容) `role-bindings`(兼容) · `/v1/audit-logs`

请求头：`X-Tenant-ID`（默认 `tenant-ikala`）、`X-Account-ID`（默认 `acct-hr-admin`）、
可选 `X-Assumed-Role-Session-ID`、`Authorization: Bearer`（Keycloak 启用后）。

## 测试

```bash
go test ./...      # 含 internal/authz 引擎单测（多组并集 / deny 优先 / boundary 收缩 / 字段策略 / 跨租户）
```

## HR 业务域（骨架，依据「员工管理」PRD）

数据库 schema 已按 PRD 落地（DB 设计在本期范围内），业务逻辑仍为 **501 骨架**：

- **`hr_employees`**：忠实映射 PRD 六分页。可查询/状态字段为列（员工编号、姓名、公司
  Email、行动电话、部门 `org_unit_id`、职称、`employment_status`(试用/在职/留停/离职/待加入)、
  `category` 身分类别、到职日、年资、留停/离职条件字段…）；自包含的可选分页（法规身份、
  外籍资料、生理、学历、兵役、通讯、紧急联络人、保险）以 JSONB section 存放（键见迁移注释）。
- **`hr_employee_assignments`**：内部经历/异动历史（1:N，异动原因 新进/转调/升迁/降调/留停复职）。
- **`hr_org_units`**：组织架构（部门/职务下拉来源）。
- 三表均启用 RLS。新增权限点 `hr.employee.import` / `hr.employee.delete`（高危），
  与 `read/write/export` 一起授予 HR 管理组 / 租户管理员组。

路由占位（权限门禁，返回 501）：`GET/POST /v1/hr/employees`、`GET/PUT/DELETE
/v1/hr/employees/:id`、`POST /v1/hr/employees/import`、`GET /v1/hr/employees/export`、
`GET /v1/hr/employees/:id/assignments`、`GET /v1/org/units`。

待实现业务：员工 CRUD（6 分页 Modal）、批次匯入（CSV/XLSX≤500 预览校验）、CSV 匯出、
批次删除、在职状态机、异动历史 —— 每个动作复用 IAM 鉴权中间件做权限校验、数据范围与审计。

## 范围说明

本阶段聚焦**数据库表设计 + 权限底座 + 项目框架**；HR 及 `/v1/forms/*`、`/v1/workflows/*`、
`/v1/agents/runs` 等业务逻辑暂不实现，仅以骨架/占位预留。
