# 第五轮 Review：架构多维度评价 · 逻辑正确性 · 接口契约一致性

> 日期：2026-06-16（第五轮，全量复审）
> 验证基线：`go build` ✅ · `go vet` ✅ · `go test` ✅（全绿）· deadcode 工具已跑 · 关键路径逐行读码确认
> 本轮交付：① 多维度架构评价（架构/分层/框架/目录…优缺点 + 改进）② 深入逻辑正确性（是否有逻辑错误）③ 接口返回结构是否统一
> 约束：**不改代码，仅给结论与方向**

---

## 0. 复审基线：第四轮修复已确认落地

| 第四轮致命项 | 状态 | 证据 |
|---|---|---|
| F1 事务非 panic-safe | ✅ 已修 | `WithTenantTransaction` 现有 `defer recover()+Rollback`，并接收 `execCtx`（`store.go:35-60`）|
| F1 根因 `must()` panic | ✅ 基本消除 | 写方法改为返回 `error`；`must` 已不可达（deadcode 确认）|
| F2 XLSX 主管列丢数据 | ✅ 已修 | `make([]string, 9)` 已移除 |
| F3 Store 无 ctx | ✅ 已修 | `repository/store.go` 全部方法接收 `context.Context` |
| 死代码 | ◑ 部分 | `nextEmployeeNo`/`filterLeaveRequestsByEmployee`/`PathParam` 已清；`must`/`ctx` 变残留（见 §4）|

**结论**：第四轮的"事务/上下文"这组同根致命项已系统性解决，质量上了一个台阶。本轮发现的问题级别明显下降——**没有架构级返工需求**，但仍有 3 个 P0 级逻辑缺陷和一批接口契约不一致。

---

## 1. 多维度架构评价

### 评分卡（10 分制）

| 维度 | 评分 | 一句话 |
|---|:---:|---|
| 整体架构（模块化单体 + 端口适配器）| 8.5 | 定位清晰、边界合理，是该阶段的正确选择 |
| 分层设计（api/service/repository/domain/platform）| 8.0 | 分层干净，唯 service 仍是 God 结构 |
| 框架与技术选型 | 8.5 | Gin/pgx/sqlc/RLS/OFGA/OTel 组合成熟克制 |
| 目录结构 | 8.5 | 符合 Go 社区惯例，`token.go` 位置可商榷 |
| 多租户与授权架构 | 7.5 | 设计完整且强大，但快照失效与 scope 边界有缺陷 |
| 错误处理与可观测性 | 7.5 | 写路径已规范；读路径仍 panic 兜底 |
| 接口契约一致性 | 7.0 | 外层信封统一，内层 list 形状有 4 种 |
| 测试与工程化 | 6.0 | 有 CI、单测全绿，但缺真实 Postgres 集成测试 |

### 1.1 整体架构 — 模块化单体 + Ports & Adapters

**优点**
- 定位明确：`repository.Store` 接口 + memory/postgres 双适配器，是教科书式的端口适配器；`service` 持接口、`platform` 放外部系统适配器（pg/redis/openfga/telemetry），依赖方向干净（domain 不依赖任何上层）。
- 可替换性强：authzSnapshot / relationships / objectStore / tokenResolver 全部经 Options/接口注入，`cmd/api/main.go` 按配置装配，可降级运行（无 DB 用 memory、无 Redis 跳过快照、无 OFGA 仅 RBAC）。

**缺点**
- memory + postgres **双实现的维护税**真实存在（926 / 1255 行），且历史上有"能力靠类型断言"导致的行为分叉风险（现已大多入接口，分叉收敛）。
- memory 适配器主要服务测试，却承担了与生产等价的语义责任——一旦两者语义漂移，单测会给假信心（呼应 §1.8 集成测试缺口）。

**建议**：保持模块化单体；将来按域拆包时（第二轮路线 B），优先让各域依赖**窄接口**而非全量 `Store`，把 memory 的维护面也按域收窄。

### 1.2 分层设计

**优点**
- 四层清晰：`api/v1`（HTTP/解析/鉴权入口）→ `service`（编排+业务+授权）→ `repository`（持久化端口）→ `domain`（实体/错误/枚举）。`platform` 横切。
- HTTP handler 用自定义签名 `func(w,r,ctx) error`，与 Gin 解耦、易测；错误经单点 `writeError` 翻译。

**缺点**
- `service` 仍是单一 God `Service`，子服务 `struct{ *Service }` 内嵌——**组织上分了，依赖上没分**，任一子服务可触达全量 store 与他域 helper（如 attendance 直接用 workflow 的 store）。
- 59 个 `*Service` 门面转发方法仍在（纯样板）。
- 授权评估（`evaluateAuthz`）与数据范围过滤逻辑庞大（`authz_runtime.go` 860 行），混了策略代数与编排。

**建议**：按域拆 service 子包 + 窄接口注入；把 `authz_runtime` 的纯策略函数（scope 合并、boundary 交并、permissionMatches）下沉到 `domain/authz`。

### 1.3 框架与技术选型

**优点**
- 选型成熟且克制：Gin（路由）、pgx/sqlc（类型安全 SQL，无重型 ORM）、Postgres RLS（DB 级租户隔离）、Redis（授权快照）、OpenFGA（ReBAC 兜底）、Keycloak OIDC、OTel（trace）。
- sqlc 而非 ORM——SQL 显式、可控、性能可预期，契合后端工程审美。
- goose 迁移与运行时解耦（不进依赖图），干净。

**缺点 / 注意**
- sqlc 读 `db/schema.sql`，运行时迁移在 `db/migrations/`，**双源需手工同步**（漂移风险，建议 CI 加 `sqlc diff`）。
- Keycloak token 验签（`token.go` 392 行 JWKS/OIDC）放在 `api/v1` 包内——属认证**基础设施**，宜迁 `internal/platform/auth`，`v1` 只依赖 `TokenResolver` 接口。
- 自定义 handler 放弃了 Gin 的声明式 binding 校验；现用 `ValidatedInput` 接口补偿（见 §2.7），方向对，但覆盖率依赖每个 input 类型是否实现 `Validate()`。

### 1.4 目录结构

**优点**：`internal/{api/v1, service, repository/{memory,postgres}, domain, domain/authz, platform/*, jobs, config}` 完全符合 Go 社区惯例；生成代码独立在 `platform/postgres/db`；`repository/internal/sliceutil` 用 internal 收窄可见性，规范。

**缺点 / 建议**
- `token.go` 归位 `platform/auth`（见上）。
- `platform/postgres/db`（sqlc 生成）与 `repository/postgres`（适配器）命名易混，可把生成包改名 `…/sqlc`。
- `internal/jobs` 至今未接入任何调用方（见 §4），目录占位但无生命。

### 1.5 多租户与授权架构

**优点**：共享 schema + `tenant_id` + RLS（已加 `WITH CHECK`/`FORCE`，第一轮建议已落地）；授权引擎功能完整——RBAC（直接/组/角色权限集）+ deny 否决 + 权限边界交并 + 数据范围 + 字段脱敏 + 高危审批 + OpenFGA 兜底 + Redis 快照。这是系统最有价值的资产。

**缺点**：见 §3 的 P0-1（快照绕过会话过期）、P1（department_subtree/self 在未关联员工的账号上静默返回 0 行）、快照失效不覆盖 org 结构变更。授权的"强大"目前被"缓存与 scope 边界的正确性缺陷"拖累。

### 1.6 错误处理与可观测性

**优点**：`domain.AppError` 分类清晰（BadRequest/Validation/Forbidden/NotFound/Conflict…），单点 `writeError` 翻译为 `{error:{code,message,...}}`，5xx 不泄漏内部细节、记 trace_id；OTel + 结构化日志 + trace_id 贯穿请求日志。

**缺点**：**读路径仍 panic**——`Get*/List*` 签名是 `(T,bool)`/`[]T` 无 error 通道，postgres 实现对真实 DB 错误调用 `mustNoValue(err)` panic，靠 recovery 中间件兜成 500（事务内则先回滚再 re-panic）。即"写返回 error、读 panic"的二元不一致（见 §3-P1-读吞错）。

**建议**：给读方法也加 `error` 返回（或返回 `(T, bool, error)`），消除 panic-as-control-flow；区分"未找到"与"DB 故障"。

### 1.7 测试与工程化

**优点**：已有 `.github/` CI；单测覆盖 service/api/repository/config，全绿；用 memory 适配器做快速单测。

**缺点（最大短板）**：**无真实 Postgres 集成测试**。RLS 的 `WITH CHECK`/`FORCE`、事务回滚、`NextEmployeeNo` 并发、`LIKE` 扫描、分页 SQL 路径、`ReserveLeaveBalance` 原子性——这些只在 memory 实现下被验证，而 memory 与 pg 已被发现存在语义差异（见 §3-P2-11 leaveType 去空格）。**这是当前最该补的工程项**，也是后续所有持久层改动的回归前置。

---

## 2. 接口返回结构一致性

### 2.1 外层信封：✅ 统一
所有 2xx–3xx 经 `writeJSON` 统一包 `{"data": ...}`（`response.go:44-51`）；所有错误经 `writeError` 统一 `{"error":{code,message,[field_errors],[row_errors],[trace_id]}}`。字段命名**全仓 snake_case**，无 camelCase 混用。错误与命名两项**无需改动**。

### 2.2 🟠 内层 list 形状有 **4 种**（最需收敛）
| 形状 | 字段 | 使用处 |
|---|---|---|
| `PageResponse[T]` | items,total,page,page_size,sort | 12 个 list（IAM/考勤/工作流/agent/audit/org）|
| `EmployeeListResponse` | items,total,page,page_size,sort | 仅 `listEmployees`——**与 `PageResponse[Employee]` 逐字节相同**，纯重复类型（`hr.go:65`）|
| `ListResponse[T]` | items,total（无 page/sort）| 仅 `exportEmployees`（`hr.go:172`）|
| 手搓 `map{"items"}` | 仅 items（**无 total**）| 仅 `getMenus`（`me.go:37`）|

**改进**：删 `EmployeeListResponse`、`ListResponse`，统一 `PageResponse[T]`；`getMenus` 若是树结构则单列类型并补 `total` 或明确文档化它非分页。

### 2.3 🟠 分页字段不一致
- `exportEmployees` 解析了 `page`/`page_size` 却在响应里丢弃（`hr.go:157` 解析，`:172` 不回传）——**误导**：客户端以为能分页。
- `getMenus` 无 `total`/分页字段。

### 2.4 🟠 HTTP 状态码
- **批量部分失败仍 200**：`batchDeleteEmployees`（`hr.go:185`）即使部分/全部行失败也返回 200，失败埋在 body 的 per-row `results` 里——只看状态码无法区分全失败与全成功。建议全失败返非 2xx，或采用 207 语义。
- `assumeRole` 创建了会话（`UpsertAssumableRoleSession`）却返回 **200 而非 201**（`iam.go:213`）。
- 全仓**无 204**——`deleteEmployee` 返 200 带墓碑实体（可接受的设计选择，但需一致）。

### 2.5 🟠 无类型 `map[string]any` 载荷
`assumeRole`、`explainAuthz`、`simulateAuthz` 返回手搓 map（`session_id`/`decision`/`simulated`…），**无结构体、无编译期契约**，易随手改漂移。建议提为具名响应类型。

### 2.6 ⚪ 特殊响应
CSV 导出绕过 `writeJSON`（正确），文件名用 `mime.FormatMediaType` 安全转义（第三轮注入隐患已修）。但无共享 `writeAttachment` 助手，未来下载端点需重复这套安全头逻辑。

### 2.7 🟡 校验覆盖不均
`readJSON` 后自动调用 `validateInput`（仅当 input 实现 `ValidatedInput`）。机制一致，但**覆盖率取决于每个 input 类型是否实现 `Validate()`**；且 `employeeImportPreviewInput` 的 **multipart 分支不走 `validateInput`**（只有 JSON 分支走），同一端点按 content-type 校验力度不同。`exportEmployees` 还存在"query + body 双源解析、body 全量覆盖 query"的隐式契约陷阱。

---

## 3. 逻辑正确性（深入代码逻辑）

> 以下为逐路径追踪所得**真实逻辑缺陷**，按影响排序。P0-1 已读码确认；其余建议补针对性测试复现后修复。

### 🔴 P0-1【已确认】授权快照绕过 assumed-role 会话的过期/吊销
**位置**：`authz_runtime.go:22-24`（先查缓存）+ `authz_snapshot.go:17-35`（key）。
**事实**：缓存在 `collectAuthzGrants`/`activeAssumableRole`（会话有效性校验处）**之前**被命中；快照 key 含 `assumed_role_id`（会话 ID）+ `permission_version`，**但不含会话的 `ExpiresAt`/`RevokedAt`**，TTL 5 分钟。
**后果**：assume 一个短于 5 分钟的会话（或中途吊销），在 TTL 窗口内携带同一会话 ID 的请求继续命中缓存、返回 `Allowed=true`——**store 层的过期校验永不触达**。权限配置变更会 bump version 失效，但**会话过期/吊销既不 bump version 也不失效缓存**。
**修复**：快照 key 纳入会话 `ExpiresAt`（或会话指纹）；或对带会话的请求跳过缓存/在命中后校验会话仍有效。

### 🔴 P0-2 员工状态机允许"复活"且账号状态不一致
**位置**：`employee_status.go:108-196`（Transition）、`hr_service.go:250-304`（UpdateStatus）。
**事实**：两条路径都**不校验当前状态**即应用目标状态。`UpdateEmployeeStatus` 仅拦 `resigned`（须走 transition）；`TransitionEmployeeStatus` 接受任意合法目标。
**后果**：`deleted`→`active`、`resigned`→`active` 都能成功；而离职时禁用的关联账号（`employee_status.go:154-159`）在复活时**不会被重新启用**——员工 active 但账号 disabled，状态不一致。
**修复**：定义合法转移表（如 deleted/resigned 不可直接回 active，或回 active 时同步 re-enable 账号），拒绝非法转移。

### 🔴 P0-3 导入确认对"预览通过却确认失败"的行会回滚整批
**位置**：预览 `validateEmployeeImportRow`（`employee_import.go:239-263`）vs 确认 `employeeFromCreateInput`→`validateEmployee`（`employee_model.go:227-290`）。
**事实**：确认期的校验**严于**预览——额外强制 company_email 非空、category/status 合法、**主管存在性**、**外籍证件必填**、store 级唯一性。预览通过（`Valid=true`）的行若在这些维度不合格，`employeeFromCreateInput` 抛硬 error（`employee_import.go:157`）→ **整个事务回滚**，此前所有合法行一并失败，且用户在预览时毫无警告。
**修复**：① 预览与确认共用同一套校验（消除 parity gap）；② 确认改为 per-row 收集结果（像 `BatchDeleteEmployees`），不因单行整批回滚。

### 🟠 P1-4 `department_subtree`/`self` 在"未关联员工的账号"上静默返回 0 行
**位置**：`authz_runtime.go:282-306`、`attendance_service.go:259-275`。
**事实**：scope 条件 `org_unit_ids`/`employee_ids` 源自 `account.EmployeeID` 的员工/组织；若账号未关联员工（服务账号/未绑定管理员），条件为空 → 过滤后 0 行。
**后果**：这类账号在 `department_subtree`/`self` 下看到**零员工/零假期**，而非报错或合理集合；且 HR 列表路径会"回退到本人 org 再推导"，attendance 路径不回退——**两处对同一 decision 产出不同集合**。
**修复**：统一两处的 fallback 行为；对"未关联员工但持子树/自身 scope"的情况返回明确语义（拒绝或空+提示），并对齐 HR/Attendance。

### 🟠 P1-5 读 `(T,bool)=false` 被一律当"未找到"，静默吞掉缺失的权限集/用户组
**位置**：`collectAuthzGrants` 的 `addSet`/group 分支（`authz_runtime.go:~136,~170`）、`linkEmployeeAccount`（`employee_model.go:465-471`）。
**事实**：`GetPermissionSet`/`GetUserGroup`/`GetAccount` 返回 `(T,bool)` 无 error；引用的权限集/用户组缺失时 `continue` 静默跳过 → 主体**静默少授权**，无告警无审计；员工可在 `AccountID` 指向不存在账号的情况下被离职/删除而无人察觉悬挂引用。
**修复**：与 §1.6 一并——读方法加 error 通道，区分"未找到"与"DB 故障"；对"引用的配置缺失"至少记一条 warn 审计。

### 🟠 P1-6 确认期未拦截"文件内 company_email/account_id 重复"
**位置**：`employee_import.go:155-176` + `employee_model.go:292-323`。
**事实**：确认期唯一性校验针对**当前库**（此刻批内均未落库），文件内重复检测只在**预览**做（`:76-82`）；确认期 `reservedEmployeeNos` 只防 employee_no 文件内重复，**不防 company_email/account_id**。
**后果**：两行新邮箱相同（库中均不存在）可双双通过确认入库。
**修复**：确认期对 company_email/account_id 也做批内保留集校验。

### 🟡 P2-7 PG 的 `ReserveLeaveBalance` 用未去空格的 leaveType，与 memory 行为分叉
**位置**：`postgres/store.go:~592` vs `memory/store.go:~635`。
**事实**：PG 条件 UPDATE 用原始 `leaveType`，未命中后回退扫描却用 `EqualFold(TrimSpace(...))`；带尾空格的 leaveType 在 UPDATE 不匹配、在扫描却匹配 → 误报"余额不足"（实为类型空格不一致）。memory 一致去空格，**两后端分叉**。
**修复**：两端统一对 leaveType 去空格归一。

### 🟡 P2-8 `EmployeeStats` 把 deleted 计入 Resigned，但 deleted 已被查询层过滤 → 死分支
**位置**：`employee.go:217-219` + `filterEmployeeQuery`（`:44`）。
**事实**：stats 不带状态过滤，`filterEmployeeQuery` 默认剔除 deleted，故 `case deleted→Resigned++` 永不触发，期望"deleted 计入离职"会少计。
**修复**：明确 deleted 是否计入统计，去掉死分支或调整过滤。

### 正确性已确认无误的点（避免误改）
- 员工编号生成与导入保留集**无碰撞**（序列持久自增 + reserved 集），re-confirm 幂等由 `Status=="confirmed"→Conflict` + 过期校验守住。
- 事务 panic-safety 装配（defer recover+rollback、ctx 贯穿、`next:=*c` 克隆 tx store）正确。
- 双语状态归一（`ParseEmployeeStatus`/`validEmployeeStatus`）一致，中英文 round-trip 正常。

---

## 4. 残留的非阻塞项（清理即可）

- **死代码**：`postgres/store.go` 的 `must` 与 `ctx`（`context.Background()` 包装）已不可达（deadcode 确认），可删；读方法仍用 `mustNoValue`（见 §1.6，建议改 error 而非保留）。
- **`internal/jobs` 整包仍未接入**任何调用方——要么本期接入（导出/Agent/审计归档异步化），要么先删，避免未测腐化。
- **59 个门面转发**仍在（非阻塞，按需做路线 B）。
- **枚举如仍以 `string(AppHR)` 转回**（第四轮 O1），建议收敛为"字段即具名类型"以拿到编译期安全，或退回无类型常量——别停在中间。

---

## 5. 优先级与总结

| 优先级 | 事项 | 类别 |
|:---:|---|---|
| P0 | 快照纳入会话过期（P0-1）；状态机合法转移 + 账号同步（P0-2）；导入确认 per-row 不整批回滚 + 预览/确认校验对齐（P0-3）| 逻辑/安全 |
| P1 | 读方法加 error 通道、消除 panic 兜底与静默吞错（P1-5、§1.6）；department_subtree/self 边界对齐（P1-4）；确认期 email/account 批内查重（P1-6）| 逻辑/健壮 |
| P1 | 接口契约收敛：统一 list 形状为 `PageResponse`、导出/菜单补齐分页字段、批量部分失败的状态码、无类型 map 提类型（§2）| 契约 |
| P1 | **补真实 Postgres 集成测试**（RLS/事务/并发取号/分页/leave 预留）——所有持久层改动的回归前置 | 工程 |
| P2 | leaveType 去空格对齐双后端（P2-7）、stats 死分支（P2-8）、删死代码、jobs 接入或删除、枚举收敛、token.go 归位 | 优化/清理 |

### 一句话总结
项目经过五轮整改已达到**结构清晰、依赖干净、可编译可测、无架构级返工**的良好状态，模块化单体 + 端口适配器 + RLS + 完整授权引擎的选型成熟而克制。当前**没有致命的崩溃类缺陷**（第四轮已修复），剩下的是更隐蔽的**逻辑正确性**问题——最关键三件：**授权快照会服务已过期/吊销的临时角色会话**、**员工状态机允许"复活"且账号状态不同步**、**导入确认会因单行预览/确认校验不一致而回滚整批**。接口层外层信封已统一，但内层"列表"有 4 种形状需收敛。**最高杠杆的两步：先补一张真实 Postgres 集成测试网，再修这三个 P0 逻辑缺陷**——之后此代码库即可视为生产就绪的稳固基线。
