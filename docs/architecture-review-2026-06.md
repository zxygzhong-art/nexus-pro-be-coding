# 架构评审与优化方案 — nexus-pro-be

> 评审日期：2026-06-16
> 评审范围：全量代码（85 个 Go 文件、迁移脚本、SQL、配置、测试、ops）
> 评审目标：查漏补缺，给出可落地的重构 / 优化方案（**仅方案，不改代码**）

---

## 0. TL;DR（最重要的 10 件事）

| # | 问题 | 严重度 | 位置 |
|---|------|:---:|------|
| 1 | **对象级（行级）授权缺失**：所有 `/:id` 路由只校验资源类型权限，不校验"这条记录能否被本人访问"，存在 IDOR 风险 | 🔴 高 | `api/v1/authz.go:24` |
| 2 | **存在两套并行且不一致的授权引擎**：HR 走完整引擎（deny/boundary/OpenFGA/审批），考勤与 me 走弱引擎 `collectionScope`，安全保证不对等 | 🔴 高 | `service.go:253`、`attendance_service.go:38` |
| 3 | **数据范围 `department_subtree` / `direct_reports` 实际失效**：`scopeConditions` 从不填充对应条件，`direct_reports` 永远返回 0 行 | 🔴 高 | `authz_runtime.go:636`、`303` |
| 4 | **非 Employee 的 Postgres 方法在出错时 `panic`**（`must`），且全部使用 `context.Background()`，丢失取消/超时/链路追踪 | 🔴 高 | `postgres/store.go:776`、`784` |
| 5 | **RLS 策略缺少 `WITH CHECK` 与 `FORCE`**：写入可携带任意 `tenant_id`，租户隔离完全依赖应用层正确性 | 🔴 高 | `migrations/000001_init.sql:430` |
| 6 | **请假余额扣减是无锁、非事务的读-改-写**，存在丢失更新/超额请假风险；表单+请假+余额三次写入无原子性 | 🔴 高 | `attendance_service.go:160` |
| 7 | **非生产环境默认信任未签名 JWT + Header 上下文**，且空上下文默认落到 `acct-admin` 管理员身份 | 🟠 中 | `token.go:35`、`context.go:32` |
| 8 | **分页仅 Employee 有**：其余所有 `ListX`（尤其 `audit_logs`、`agent_runs`）全表返回，必然随数据增长 OOM | 🟠 中 | `db/queries/core.sql`（多处）|
| 9 | **OpenAPI 与实现严重漂移**：spec 仅描述 ~11 条路由，代码注册 51 条；且 `{session_id}` 与 `:id` 不一致 | 🟠 中 | `docs/openapi.yaml` |
| 10 | **无 CI、无 lint 配置、无真实 Postgres 集成测试**：核心 SQL/RLS/事务路径只在内存实现下被测，给出虚假信心 | 🟠 中 | 仓库根 |

---

## 1. 架构现状

**定位**：多租户 HR 平台的模块化单体（modular monolith）。技术栈 Go 1.26 + Gin + pgx/sqlc + Postgres(RLS) + Redis + OpenFGA(可选) + Keycloak OIDC(可选) + OTel。

**分层**：

```
cmd/api            进程入口、依赖装配、优雅关闭
  └─ internal/api/v1        HTTP 控制器、中间件、Token 解析、上下文解析
       └─ internal/service       业务服务（HR/IAM/Authz/考勤/工作流/Agent/审计/Me）
            └─ internal/repository    Store 接口 + memory / postgres 双实现
                 └─ internal/platform    postgres(sqlc) / redis / openfga / telemetry
  └─ internal/domain        领域模型、错误、上下文、authz 类型与路由策略
```

**值得肯定的设计**：

- ✅ HTTP handler 采用自定义签名 `func(w, r, ctx) error`，与 Gin 解耦，便于 httptest 单测。
- ✅ 错误翻译单点化（`response.go: writeError` + `domain.AppError` 分类），错误码体系清晰。
- ✅ `readJSON` 默认开启 1MiB 限流 + `DisallowUnknownFields`，输入解析基线良好。
- ✅ Store 为接口、`now` 可注入、authz/relationships/objectStore 经 Options 注入，依赖可替换。
- ✅ 授权引擎 `evaluateAuthz` 设计完整：RBAC + 数据范围 + 字段策略 + 权限边界 + 审批 + OpenFGA 兜底 + 快照缓存，是系统最有价值的部分。
- ✅ 生产环境硬性要求 Keycloak 配置，否则进程退出。
- ✅ 多租户采用共享 schema + `tenant_id` + RLS，迁移中索引/CHECK 约束较为完善。

---

## 2. 问题清单（按域分类）

### 2.1 安全 / 授权（最高优先级）

**S1. 对象级授权缺失（IDOR）** — `api/v1/authz.go:24`
`authorize` 只传 `{resource, action}`，从不传 `ResourceID`/`TargetEmployeeID`，尽管 `CheckRequest` 支持。因此 `GET /v1/hr/employees/:id` 只校验"能读员工资源"，不校验"能读这一个员工"。跨租户/越权依赖 service 层 `tenantID` 过滤，缺少纵深防御。

**S2. 双授权引擎不一致** — `service.go:253` vs `authz_runtime.go:20`
- 完整引擎 `evaluateAuthz`：处理 deny 短路、权限边界交并、数据范围、字段脱敏、审批、OpenFGA 兜底。
- 弱引擎 `collectionScope`/`resolveAccess`：只算 `all/self` 一个字符串，**忽略 deny、边界、OpenFGA、审批、字段策略**。
- 考勤（`attendance_service.go:38/57/82`）与 Me 走弱引擎 → 考勤授权实质弱于 HR。

**S3. 数据范围实现性缺陷** — `authz_runtime.go:636`、`266`
- `scopeConditions` 只写 `employee_id_source` / `manager_account_id`，**从不填充** `org_unit_ids` 与 `employee_ids`。
- 结果：`department_subtree` 退化为"仅请求者本人所在 org unit"；`direct_reports` 永远命中空集 → **始终返回 0 行**。

**S4. 授权快照可能返回陈旧决策** — `authz_snapshot.go` + `authz_runtime.go:491`
快照 key 含 `permission_version`，但版本仅在 IAM 配置写入时 `touchAuthzConfig` 自增。影响授权结果但不触发自增的操作（员工 `OrgUnitID`/`AccountID` 变更、`AssumeRole` 建会话）会导致最长 5 分钟陈旧决策。`AssumeRole`（`iam_service.go:327`）未做任何失效。

**S5. 非生产默认弱认证** — `token.go:35`、`context.go:32`
- `unsignedJWTResolver` 不验签直接信任 claims，且在 `AllowDemoContext=true` 时自动启用。
- `allowHeaderContext` 下任意请求可用 `X-Tenant-ID`/`X-Account-ID` 冒充。
- 空上下文默认落到 `tenant=demo` / `account=acct-admin`（近管理员）。
- 虽然 gated 在 `Env != production`，但任何非 prod 构建被暴露即是完整认证绕过。

**S6. JWKS `kid` miss 触发无限刷新** — `token.go:107`
攻击者用随机 `kid` 的 token 可迫使重复 OIDC discovery + JWKS 拉取，形成对 IdP 的放大/DoS。无负缓存、无限流。

**S7. RLS 写入无 `WITH CHECK`、无 `FORCE`** — `migrations/000001_init.sql:430`
`USING` 只过滤读取与 UPDATE/DELETE 目标行，`INSERT`/`UPDATE` 可写入任意 `tenant_id`。若应用以表 owner 连接，RLS 被完全绕过。租户写入完整性纯靠应用传对参数。

**S8. CSV 导出文件名头注入** — `hr.go:136`
service 提供的 filename 直接插入 `Content-Disposition`，未转义引号/CRLF。

**S9. 缺少限流 / 请求超时 / CORS / body 大小（非 JSON）控制**。Server 级有 Read/Write 超时，但无 per-handler context deadline，导出/导入/批量删除/批量授权等重操作无节流。

### 2.2 数据持久层

**D1. Store 接口"精神分裂"** — `repository/store.go`
仅 `EmployeeStore`（5 个方法）是 ctx-aware + 返回 error；其余 ~50 个方法既无 ctx 也无 error。导致：
- Postgres 端非 Employee 方法用 `must()` **panic**（`store.go:784`）；
- Memory 端**静默吞错**（如 `AddAccountGroup` 账号不存在直接 return）。

**D2. 非 Employee 查询全部 `context.Background()`** — `postgres/store.go:776`
无取消、无超时、无链路追踪贯穿，HTTP 客户端断开也不会回滚事务。

**D3. 能力倾斜导致双后端行为不一致** — `service/employee_limits.go`
扩展查询/分页/取号方法（`ListEmployeePageByQuery`、`NextEmployeeNo` 等）在接口之外，靠运行时类型断言访问。Memory 缺失其中 3 个 → 内存与 Postgres 的分页总数、员工编号分配行为不同，而大多数测试跑在内存实现上。

**D4. `TenantIDFromArgs` 是启发式猜测** — `tenantctx/tenantctx.go:5`
取第一个 string 参数或名为 `TenantID` 的字段作租户。对首参为非租户 string 的查询（如 `GetTenant(ctx, id)`）会把 `id` 当租户设入 `app.tenant_id`；无租户参数时直接在 pool 上裸跑（隐式 fail-closed，但非显式断言）。

**D5. 无跨实体外键** — `migrations/000001_init.sql`
所有外键只指向 `tenants`。`employees.org_unit_id`、`leave_requests.employee_id`、`form_instances.template_id`、authz_* 交叉引用等全是裸 `text` 无 FK（因为用 `NOT NULL DEFAULT ''` 空串哨兵，与标准 FK 不兼容）。可产生孤儿数据，引用完整性纯应用层保证。

**D6. 分页仅 Employee 有，其余全表返回** — `db/queries/core.sql`、`authz.sql`
`ListAuditLogs`、`ListAgentRuns`、`ListPermissionSets`… 全部无 `LIMIT`。审计日志为只增表，全量返回必然 OOM。唯一分页查询用 OFFSET（深翻页退化），无 keyset/游标。

**D7. 全表扫描热点**
- 员工关键词搜索用前导通配 `LIKE '%kw%'`（`core.sql:210`）→ 无法用 btree，顺序扫描；无 pg_trgm/FTS。
- 每次建员工 `MaxEmployeeNoSequence` 对全租户 `employee_no` 做正则扫描（`core.sql:291`），无支撑索引。

**D8. Memory 无事务语义** — `repository/store.go:127`
`WithinTenantTransaction` 对 memory 退化为直接 `fn(store)`，多步操作可留下部分状态，测试无法验证回滚。

**D9. ~7 张 authz 表 + ReBAC tuple 表有 sqlc 查询但无 Store 方法**（`user_identities`、`authz_applications`、`authz_permissions`、`authz_group_memberships`、`authz_policy_conditions`、`authz_relationship_tuples`）→ 数据层是"半成品/前瞻性"schema。

**D10. sqlc 双源维护漂移风险** — `sqlc.yaml`
sqlc 读 `db/schema.sql`，而运行时迁移在 `db/migrations/`，两份 DDL 需手工同步，已存在漂移隐患。

**D11. 取号两次往返 + 兜底取号 O(n) 且竞态** — `postgres/store.go:502`、`employee_model.go:335`
`NextEmployeeNo` 两次查询；非 Postgres 路径 `nextEmployeeNo` 列出全部员工取 max+1，racy。

### 2.3 服务层

**SV1. 单一 God 结构 + 大量门面转发** — `service.go:13`、`hr_service.go:13`
所有子服务都内嵌 `*Service`（无真正封装），每个方法存在两份（`*Service` 转发 + 子服务实现），需手工保持同步，子服务可越界访问全部 store 与他域 helper。

**SV2. 领域逻辑外溢到 service 包**
状态/分类归一、校验、脱敏、scope 排序、取号、CSV/XLSX 解析都在 `service/`，纯策略代数（`mergePolicy`、`intersectPolicyList`、`scopeRank`）混在 819 行的 `authz_runtime.go`，应属 `domain/authz`。

**SV3. 手写 XLSX 解析器** — `employee_import.go:269`
手工解 zip/XML/shared-strings/列索引，假设 `sheet1.xml`、固定 9 列、忽略数字格式与日期，脆弱且重复造轮子。

**SV4. 双语魔法字符串遍地** — `employee.go:195`、`employee_status.go:143` 等
即便 `normalizeEmployeeStatus` 已归一，仍处处同时比较 `"resigned"` 与 `"離職"`，冗余易错；列头、原因、校验文案全硬编码。

**SV5. 批量删除非原子** — `employee_status.go:5`
`BatchDeleteEmployees` 对每条记录开独立事务，中途失败留部分应用，仍返回 200 混合结果。

**SV6. Agent 回答极其朴素** — `service.go:177`、`agent_service.go:47`
关键词 token 袋匹配知识库，无命中则返回 `articles[0]`，截断 120 字，命中"請假/leave"追加硬编码中文提示；`run.Status` 硬编码 `"completed"`，无 LLM/异步/检索排序/流式。

**SV7. 校验薄弱**：email 仅校验非空与含 `@`，无日期区间（入职 vs 离职）、无电话/证件格式校验。

**SV8. 缺少幂等键**（除导入确认靠状态守卫）；导出仅同步，错误信息引用并不存在的"异步导出任务"（`employee_export.go:55`）。

### 2.4 API 层

**A1. 响应信封不一致**：list 多数包 `{"items":[]}`，但 `listEmployees` 直接返回原始结构，create 返回裸对象，无统一成功信封与分页元数据。

**A2. 查询解析吞错** — `hr.go:248`：`page/page_size` 用 `strconv.Atoi` 丢弃 error，`?page=abc` 静默变 0，无上界。

**A3. 可选 body 读取模式在 3 处重复**（`assumeRole`/`confirmEmployeeImport`/`inviteEmployee`），应抽 `readOptionalJSON`。

**A4. 无 API DTO**：handler 直接依赖 `domain.*` 输入/输出结构，领域字段改名即破坏 API 契约，无处施加 API 级校验。

**A5. explain/simulate 是桩** — `authz.go:51/64`：仅转调 `Check` 后重新包装，未真正模拟其他主体/策略，端点有误导性。

**A6. Swagger UI 从 `unpkg.com` 加载** — `swagger.go:17`：内网 API 控制台引入外部 CDN 依赖 / CSP 隐患。

**A7. `gin.ReleaseMode` 与 trusted proxies 硬编码**（`api.go:65`）而非配置驱动。

### 2.5 工程化 / 可观测 / 测试

**E1. 无 CI、无 `.golangci.yml`**：无自动化测试/lint/vet/构建门禁。

**E2. 无真实 Postgres 集成测试**：核心 SQL、RLS、事务、取号竞态、分页只在内存实现下被覆盖 → 与生产路径行为可能分叉。

**E3. 测试集中度**：46 个测试，service_test 985 行、api_test 568 行，集中在 `tests/unit`，对照 git status 显示历史上有结构搬迁。建议测试与被测包同目录（Go 惯例）以利覆盖率与可见性。

**E4. 配置无校验/无类型分组**：`config.go` 无必填校验（除 main 里的 Keycloak 硬 gate）、无 DSN 合法性校验、无配置项文档化（`.env.example` 仅 21 行）。

**E5. `internal/jobs` 仅占位**：导出/Agent/审计归档等显然需要的异步任务无落地框架。

---

## 3. 优化方案（按优先级分阶段）

### 阶段一：安全与正确性止血（P0，建议 1–2 周）

1. **补齐对象级授权（S1）**：在 `authorize` 注入路由路径参数（`:id`）为 `ResourceID`/`TargetEmployeeID`，让 `/:id` 路由进入 `evaluateAuthz` 的 OpenFGA/数据范围分支。或在中间件统一从 `r.PathValue` 抽取资源 ID 注入 `CheckRequest`。
2. **统一授权引擎（S2）**：废弃 `collectionScope`/`resolveAccess` 弱路径，考勤与 Me 改走 `evaluateAuthz`（集合查询用 `ResourceID==""` 走 RBAC + 数据范围）。删除一套引擎，消除安全不对等与双倍测试面。
3. **修复数据范围（S3）**：在 `scopeConditions` 真正填充 `org_unit_ids`（含子树展开）与 `employee_ids`（直属下级），或明确下线这两种 scope 直到实现完整，避免"看似生效实则失效"。
4. **请假并发安全（S6）**：将"读余额→校验→扣减→建请假→建表单"包进单个 `WithTenantTransaction`，余额行用 `SELECT … FOR UPDATE`（或 `UPDATE … SET remaining = remaining - $ WHERE remaining >= $` 原子条件更新）。
5. **RLS 加固（S7）**：所有租户表加 `WITH CHECK (tenant_id = current_setting('app.tenant_id', true))`，对应用连接角色启用 `FORCE ROW LEVEL SECURITY`，并确保应用以非 owner 角色连接。
6. **收紧非生产认证（S5/S6）**：`AllowDemoContext`/`AllowHeaderContext` 改为显式独立开关（默认关闭），空上下文不再默认 `acct-admin`；JWKS `kid` miss 增加负缓存 + 刷新限流。
7. **授权快照失效补全（S4）**：员工 org/account 变更、`AssumeRole` 建会话时调用 `touchAuthzConfig`（或按受影响 account 精准失效）。
8. **持久层错误传播（D1/D2）**：将 Store 全接口统一为 ctx-aware + 返回 error，移除 `must()` panic，所有查询贯穿请求 ctx。这是一次性大改但收益最高（取消/超时/追踪/优雅降级一并解决）。

### 阶段二：可扩展性与一致性（P1，建议 2–4 周）

9. **全量分页（D6/A1/A2）**：为所有 `ListX` 引入统一分页（优先 keyset/游标，至少 `LIMIT` + 上界），统一响应信封 `{items, page, total}`，查询参数解析失败返回 400。审计/AgentRun 优先。
10. **拆分 God 结构（SV1）**：按聚合定义 per-aggregate repository 接口（`EmployeeRepo`/`IAMRepo`/…），子服务依赖各自接口而非全量 `Store`；删除门面转发样板。
11. **引入跨实体外键 + 去空串哨兵（D5）**：将 `org_unit_id`/`account_id` 等改为 nullable 并加 FK（`ON DELETE SET NULL/RESTRICT`），消除孤儿数据。
12. **统一 DDL 单源（D10）**：用 `goose`/`atlas` 由迁移生成 `schema.sql` 给 sqlc，或让 sqlc 直接读迁移目录，消除双源漂移。
13. **领域逻辑回归 domain 包（SV2）**：将 authz 策略代数、状态机、校验规则迁入 `domain/authz` 与 `domain` 的纯函数，service 只做编排。
14. **OpenAPI 契约化（A4/A9/E1）**：引入代码生成（oapi-codegen）或契约测试，路由与 spec 强一致；或至少补全 51 条路由文档并修正 `{session_id}` 漂移。

### 阶段三：工程化与产品化（P2，持续）

15. **CI + lint（E1）**：GitHub Actions 跑 `go vet` / `golangci-lint` / `go test -race` / `sqlc diff` / 迁移校验。
16. **真实 Postgres 集成测试（E2）**：用 testcontainers 覆盖 RLS、事务回滚、取号并发、分页、`LIKE`/FTS 路径。
17. **搜索优化（D7）**：员工关键词搜索加 `pg_trgm` GIN 索引或 FTS；取号扫描改为初始化一次后纯走 sequence 表。
18. **异步任务框架（E5/SV8）**：落地 `internal/jobs`（导出、Agent run、审计归档、outbox 投递），导出/Agent 改异步 + 状态轮询。
19. **Agent 真正接入 LLM（SV6）**：用 Claude（最新 Opus/Sonnet）替换关键词袋检索，引入向量检索/重排，run 状态走真实生命周期与流式。
20. **替换手写 XLSX（SV3）**、统一双语为 i18n 资源（SV4）、抽 `readOptionalJSON` 等公共助手（A3）、Swagger 资源本地化（A6）、配置校验（E4）。

---

## 4. 风险与影响评估

| 改动 | 影响面 | 回归风险 | 建议 |
|------|--------|:---:|------|
| 统一授权引擎（#2） | 考勤/Me 行为变更 | 中 | 先加测试快照对比新旧决策，灰度切换 |
| Store 全面 ctx 化（#8） | 全持久层签名 | 高 | 一次性机械改造 + 编译器兜底，配合集成测试 |
| RLS `WITH CHECK`/`FORCE`（#5） | 所有写入 | 中 | 需确认连接角色非 owner，预生产验证 |
| 外键 + 去哨兵（#11） | schema + 映射 | 高 | 需数据回填/清洗迁移，分表渐进 |
| 全量分页（#9） | 所有 list API 契约 | 中 | 统一信封属破坏性变更，建议随 v1→v2 或加版本 |

---

## 5. 一句话结论

授权引擎的设计是这套系统最有价值的资产，但它**只被一半的路由正确使用**；持久层因 Store 接口的"半现代化"（仅 Employee ctx 化）而割裂出 panic/吞错、双后端行为分叉、全表扫描三类系统性问题。**优先级应是：先让安全与正确性在所有路径上对齐（阶段一），再解决可扩展性（阶段二），最后补工程化短板（阶段三）。** 当前最危险的不是缺功能，而是"看似生效实则失效"的授权与隔离机制。
