# 结构性重构方案 — nexus-pro-be

> 评审日期：2026-06-16（第二轮，针对你已完成的安全/正确性整改后的代码）
> 评审准则（最高优先级）：**简洁（Simplicity）· 模块化（Modularity）· 高可扩展性（Extensibility）**
> 评审维度：目录/模块划分、命名与职责、上帝函数、性能与异常边界、重构前后对比
> 约束：**不改动代码，仅输出方案**

---

## 0. 先看结论（TL;DR）

第一轮的安全与正确性问题你已基本整改（RLS `WITH CHECK`/`FORCE`、统一授权引擎、分页框架、CI、jobs 骨架、`token.go` 归位前的拆分等）。本轮聚焦**结构质量**，发现 8 类系统性结构债，按"投入产出比"排序：

| # | 结构债 | 量化 | 准则违背 | 修复杠杆 |
|---|--------|------|---------|:---:|
| 1 | **门面转发样板**：`*Service` 上每个方法都转发到 `c.Xxx().Method()` | **59 个**纯转发方法 + 8 个构造器 | 简洁 | ⭐⭐⭐ |
| 2 | **授权前导块重复**：resolveAccount→evaluateAuthz→!Allowed→RequiresApproval→audit 手工内联 | **~16 个**方法逐字重复 | 简洁/可扩展 | ⭐⭐⭐ |
| 3 | **员工字段表三/五重维护**：patch / mask / forbid / import / export 各写一份字段清单 | 3~5 份并行手工表 | 可扩展 | ⭐⭐⭐ |
| 4 | **Store 接口签名割裂**：仅 6/46 方法带 ctx+error，其余 40 个无 ctx 无 error | 40 方法 → postgres `panic`、memory 吞错 | 模块化/异常 | ⭐⭐⭐ |
| 5 | **能力靠运行时类型断言**：扩展查询方法在接口之外，memory 缺 5 个 → 双后端行为分叉 | 7 处断言、memory 缺 5 能力 | 模块化 | ⭐⭐ |
| 6 | **`service.go` 杂物间**：Agent 评分/菜单目录/日期解析混入核心 | 461 行、≥4 类无关职责 | 简洁/模块化 | ⭐⭐ |
| 7 | **机械重复代码**：`list_sort.go` 10 个雷同排序、`PageResponse`==`ListResponse`、跨包 `copyStrings` 等 | 10 函数 + 多处 | 简洁 | ⭐⭐ |
| 8 | **大文件/上帝函数**：`postgres/store.go` 1273 行、`evaluateAuthz` 110 行、`CreateLeaveRequest` 120 行 | 见 §3.8 | 简洁 | ⭐⭐ |

> 核心判断：**当前不是"分层错误"，而是"边界虚设"**——子服务拆了文件却共享同一个 God `*Service`，接口拆了 11 个却签名不统一。重构的主线是**把"组织上的拆分"升级为"依赖上的隔离"**。

---

## 1. 设计思路（重构哲学）

四条贯穿全程的原则：

1. **边界即依赖收窄（Boundary = Narrowed Dependency）**
   一个模块的"独立"不取决于它在哪个文件，而取决于它**只能看到自己需要的依赖**。当前所有子服务 `struct { *Service }` 能摸到全量 store 和他域 helper —— 这是"伪模块化"。真正的模块化要求每个子服务依赖一个**窄接口**（自己的 repo + 共享的 authz/audit 协作者）。

2. **重复的模式要么抽函数、要么抽数据（DRY: Function or Data）**
   - 重复的**控制流**（授权前导块）→ 抽成一个 guard 函数。
   - 重复的**数据结构**（字段清单）→ 抽成一张**元数据表**，让 patch/mask/forbid/import/export 都从表派生。
   - 重复的**算法**（10 个排序）→ 用泛型 + 比较器表收敛。

3. **接口签名一致性优先于一切便利（Uniform Signatures）**
   `ctx context.Context` + `error` 返回值不是可选项。签名不统一直接导致 `panic` 兜底、取消/追踪断链、双后端分叉。这是结构问题的**根因之一**，优先级最高。

4. **简洁是"删代码"，不是"加抽象"（Simplicity = Less, not More）**
   59 个转发方法、10 个排序函数、5 份字段表的正确解法是**让它们消失**，而非再包一层。每个重构任务都要能回答："这次净删了多少行？"

---

## 2. 三种重构路线对比（先选路，再动手）

### 路线 A：渐进式原地重构（Incremental In-Place）
保持现有 `Service` God 结构与目录不动，只做局部清理：抽 authz guard、抽字段元数据表、泛型化排序、统一 Store 签名、合并重复 struct。

- ✅ 改动面可控、可一个 PR 一个，CI 风险低，随时可停。
- ✅ 不触碰调用方（API 层），无契约破坏。
- ❌ 子服务仍共享 God `*Service`，**模块边界依旧虚设**——治标。
- ❌ 新增域仍可随意耦合他域，可扩展性天花板没抬高。

### 路线 B：领域模块化 / 垂直切分（Domain Modules，**推荐**）
按业务域（hr / iam / authz / attendance / workflow / agent）把 service 拆成**子包**，每个子包定义自己的**窄 repo 接口**和 use-case 类型，依赖通过构造函数注入；共享能力（authz 评估、audit、事务）抽成 `internal/service/core`（或 `platform`）协作者注入。`*Service` God 结构降级为**装配根（composition root）**，59 个转发方法删除。

- ✅ 真正实现"边界即依赖收窄"，新增域零耦合他域 store。
- ✅ 每个子包可独立测试（mock 自己的窄接口），测试面收敛。
- ✅ 与现有"模块化单体"定位完全契合，是它的自然演进。
- ✅ 删除量大（59 转发 + 跨域 helper），净简洁度提升明显。
- ⚠️ 一次性改动面中等：需重排包结构、调整 API 层注入点；建议分域灰度（先 hr/attendance，后 iam/authz）。
- ⚠️ 需处理跨域协作（如 attendance 创建 leave 时要用 workflow 的 form）——通过**注入 workflow 的窄接口**而非共享 God 解决。

### 路线 C：完整六边形 / DDD（Aggregates + Interactors）
引入聚合根、值对象、领域服务、use-case interactor、端口/适配器全套；employee 字段建模为值对象，authz 策略代数下沉为领域模型。

- ✅ 理论上扩展性与可测性最高，领域逻辑最纯。
- ❌ 对一个仍在打地基的模块化单体是**过度工程**：样板（DTO↔Entity 映射、interactor 包装）反而违背"简洁"准则。
- ❌ 学习/维护成本陡增，团队收益曲线很长才回正。
- ❌ 改动面最大，CI/回归风险最高。

### 选型建议：**路线 B（领域模块化）为主线，吸收路线 A 的快赢项作为第一阶段**

> 理由：B 直接命中"模块化/可扩展性"两条最高准则，且与"模块化单体"定位同向；C 的纯度收益在当前阶段抵不过它对"简洁"的破坏。落地策略：**先用 A 的低风险快赢项（guard 函数、字段表、泛型排序、Store 签名统一）把代码"压平"，再按 B 切域隔离依赖**——A 是 B 的铺垫，不浪费。

---

## 3. 核心问题诊断 + 重构前后对比

> 下列均为**示意代码**，用于说明方向与收益，非待提交代码。

### 3.1 门面转发样板（59 方法）—— 准则：简洁

**现状**（`hr_service.go:13-91`、`iam_service.go:16-98` 等，共 59 个）：
```go
func (c *Service) CreateEmployee(ctx RequestContext, in CreateEmployeeInput) (Employee, error) {
    return c.HR().CreateEmployee(ctx, in)   // 纯转发，零逻辑
}
// ……再重复 58 次
```
子服务又 `struct{ *Service }` 内嵌，转发只是为了让 `*Service` 成为统一调用面。

**重构后**（路线 B：API 层直接持有子服务，转发层整体消失）：
```go
// 装配根只暴露子服务访问器，不再逐方法转发
type Service struct{ core *Core /* 共享协作者 */ }
func (s *Service) HR() *hr.Service   { return s.hr }
func (s *Service) IAM() *iam.Service { return s.iam }
// API 控制器注入并直接调用 hrSvc.CreateEmployee(...)
```
**提升**：净删 **59 个转发方法 + 8 构造器**（≈ 150~200 行）；新增域不再需要"再写一遍转发"；调用链少一跳。

---

### 3.2 授权前导块重复（~16 方法）—— 准则：简洁 + 可扩展

**现状**（`hr_service.go:151-166` 为典型，另见 `employee.go`、`employee_status.go`、`employee_import.go` 共 ~16 处）：
```go
account, _, err := c.resolveAccount(ctx)
if err != nil { return nil, err }
decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode:"hr", ResourceType:"employee", Action:"export"})
if err != nil { return nil, err }
if !decision.Allowed {
    c.auditAuthzDecision(ctx, "hr.employee.export", "employee_collection", "", decision)
    return nil, Forbidden(decision.Reason)
}
if decision.RequiresApproval && !ctx.ApprovalConfirmed {
    c.auditAuthzDecision(ctx, "hr.employee.export", "employee_collection", "", decision)
    return nil, Forbidden("high-risk action requires approval")
}
```
> 危险点：有的方法 denial 时审计、有的不审计；有的查 approval、有的不查 —— **不一致本身就是 bug 温床**。`agent_tools.go:29-56` 的 `authzToolGateway.Call` 已经证明这块可以收敛成一个 gateway。

**重构后**（统一 guard，内部完成审计与拒绝）：
```go
// 一次调用拿到 account + decision；不通过则已审计并返回 *AppError
func (c *core) Authorize(ctx RequestContext, req CheckRequest, a AuditTarget) (Account, CheckResult, error)

// 业务方法瘦身为：
account, decision, err := c.Authorize(ctx, exportReq, AuditTarget{Action:"hr.employee.export", Resource:"employee_collection"})
if err != nil { return nil, err }
// 直接进入业务……
```
**提升**：每个方法**删 8~12 行**，~16 处 ≈ 净删 150+ 行；审计/审批策略**强制一致**；未来加"二次确认""风险分级"只改 1 处。

---

### 3.3 员工字段表三/五重维护 —— 准则：高可扩展性（最伤扩展性的一项）

**现状**：同一份员工字段清单被手写 3~5 遍：
- `applyEmployeePatch`（`employee_model.go:66-128`）
- `forbiddenEmployeePatchFields`（`employee_model.go:130-193`，14 个 `if input.X != nil`）
- `maskEmployee`（`authz_runtime.go:325-375`，13 臂 switch）
- 导入列映射（`employee_import.go:300-311`）+ 导出表头（`employee_export.go:20-33`）

> 加一个员工字段 = 改 5 个地方，漏一个就出隐性 bug。这是**可扩展性的头号杀手**。

**重构后**（单一字段元数据表驱动）：
```go
type EmployeeField struct {
    Key       string                 // "company_email"
    Tab       string                 // "contact_info"
    Sensitive bool                   // 参与 mask
    Patchable bool                   // 参与 patch/forbid
    Get       func(Employee) any
    Set       func(*Employee, any)
    Mask      func(any) any          // nil = 默认脱敏
}
var EmployeeSchema = []EmployeeField{ /* 唯一真源 */ }

// patch / forbid / mask / import / export 全部遍历 EmployeeSchema 派生
```
**提升**：新增字段**只改 1 处**；patch/mask/forbid/import/export 自动一致；消除"漏改某张表"整类 bug。属于**为未来扩展投资**的最高价值重构。

---

### 3.4 Store 接口签名割裂（40/46 无 ctx/error）—— 准则：模块化 + 异常处理（根因级）

**现状**（`repository/store.go`）：只有 `EmployeeStore`（5）+ `ReserveLeaveBalance`（1）带 `ctx+error`；其余 **40 个**方法形如：
```go
GetTenant(id string) (domain.Tenant, bool)        // 无 ctx、无 error
IncrementPermissionVersion(tenantID string) int64 // 连 bool 都没有
```
后果链：postgres 实现只能 `must()` **panic**（`store.go:857-866`，~38 处）→ DB 错误只能被 API panic 中间件兜成 500，无法重试/包装；memory 实现**静默吞错**；请求取消/超时/链路追踪在 40 个方法处**断链**。

**重构后**（签名统一）：
```go
GetTenant(ctx context.Context, id string) (domain.Tenant, error)  // NotFound 用 error 表达
IncrementPermissionVersion(ctx context.Context, tenantID string) (int64, error)
```
**提升**：删除 `must/mustNoValue` panic 模式；错误可被上层优雅处理；ctx 贯穿全持久层（取消/超时/trace）；memory 与 postgres 行为对齐。**这是性价比最高的"根因级"重构**——它同时解开 §3.5 的分叉与异常处理短板。
**注意**：改动面大但机械，编译器会兜住每个调用点；务必配合真实 Postgres 集成测试。

---

### 3.5 能力靠运行时类型断言 → 双后端分叉 —— 准则：模块化

**现状**：扩展查询方法（`ListEmployeePageByQuery`/`NextEmployeeNo`/`ListAgentRunPage`/`ListAuditLogPage`/`ListEmployeesByQuery`）**不在 Store 接口里**，靠 `employee_limits.go:15-35` 等定义的窄接口 + 7 处运行时断言访问；**memory 缺这 5 个能力**，于是走"全量加载再内存过滤/分页"的另一条路径。
> 结果：memory（大多数单测用它）与 postgres 的**排序/分页/取号语义不同**，单测给出虚假信心。

**重构后**：把这 5 个能力**提升进** `EmployeeStore`/`AgentStore`/`AuditStore` 接口 → 编译器强制 memory 实现 → 7 处断言塌缩为直接调用，两后端路径合一。
**提升**：删除断言分支与 fallback 路径；双后端语义一致；测试不再骗人。

---

### 3.6 `service.go` 杂物间（461 行）—— 准则：简洁 + 模块化

**现状**：`service.go` 混入与"服务编排"无关的内容：
- Agent/知识库：`answerAgentPrompt`/`articleMatchScore`/`sortKnowledgeMatches`/`tokenize`/`truncateRunes`（应去 `agent_*`）
- 菜单：`defaultMenuCatalog`/`filterMenus`/`menuKeysFromPermissions`（仅 `me_service` 用，应去 `me_service.go`）
- 日期：`parseDate`/`parseDateTime`（通用工具，且 `optionalDateTime` 又在 `authz_runtime.go:851`，日期函数散落两处）

**重构后**：`service.go` 只保留 `Service`/`New`/`withTenantTransaction`/`resolveAccount`/`audit`/`goContext`/`Now`。其余按归属迁出，日期工具集中到 `internal/service/timeutil.go` 或 `domain`。
**提升**：核心文件回归单一职责；Agent/菜单逻辑与其域聚拢，配合路线 B 自然落入各子包。

---

### 3.7 机械重复 —— 准则：简洁（快赢）

- **`list_sort.go` 10 个雷同排序**（`sortUserGroups`…`sortOrgUnits`，每个都是 copy→switch→sortSlice）→ 用泛型 + 每类型一张**比较器表**收敛：
  ```go
  type Comparators[T any] map[string]func(a, b T) bool
  func sortBy[T any](items []T, key string, cmps Comparators[T], def string) []T
  // 每个实体只声明： {"name_asc": ..., "created_at_asc": ...}
  ```
  **提升**：10 个函数（139 行）→ 1 个泛型 + N 张小表，逻辑唯一。
- **`PageResponse[T]` 与 `ListResponse[T]` 字段完全相同**（`domain/pagination.go:15-29`）→ 合并为一个；删除冗余类型。
- **跨包重复 helper**：`copyStrings`/`containsString`/`removeString` 在 `memory/` 与 `postgres/` 各一份（且 `removeString` 两份语义还略有差异）→ 抽到 `internal/utils`，**消除潜在不一致**。
- **两份 unique-check**（`employeeUniqueFieldErrors` vs `…FromList`，`employee_model.go:296/329`）与**死代码** `filterLeaveBalancesByEmployee`（`service.go:247`，已被 `…Employees` 复数版取代）→ 删/并。

---

### 3.8 上帝函数 / 大文件 —— 准则：简洁

| 函数/文件 | 位置 | 规模 | 混合的职责 | 拆分建议 |
|---|---|---|---|---|
| `postgres/store.go`（文件） | 全文件 | **1273 行** | Store 类型+事务、~50 CRUD、分页/归一 helper、panic helper、pgtype 转换、JSON codec、17 个 `fromXxx` 映射 | 拆 `store.go`/`mappers.go`/`pgtypes.go` |
| `evaluateAuthz` | `authz_runtime.go:19-129` | ~110 行 | 快照缓存 + 收集 grant + 匹配/deny/boundary + 审批升级 + scope 选择 + OpenFGA fallback + 组装 | 抽 `collectGrants`/`matchGrants`/`fallbackReBAC`/`assembleDecision` |
| `CreateLeaveRequest` | `attendance_service.go:100-219` | ~120 行 | 授权 + scope + 校验 + 事务内**懒创建 form 模板/实例**（workflow 职责越界） | 授权用 guard；form 协作走注入的 workflow 窄接口 |
| `validateEmployee` | `employee_model.go:231-294` | ~64 行 | 必填 + 枚举 + org/manager 存在性 + 外籍分支 + 唯一性派发 | 校验规则表化（与 §3.3 同源） |
| `maskEmployee` | `authz_runtime.go:325-375` | ~50 行 | 13 臂字段脱敏 + default 扇出 5 个子对象 | 由 §3.3 元数据表驱动 |
| `ListEmployeePageByQuery` | `postgres/store.go:454-488` | ~35 行 | 分页默认值（魔数与 `normalizePageRequest` 不一致）+ 参数 struct 二次拷贝 + 两查询 | 复用统一分页默认；参数构造提取 |

---

### 3.9 目录/包布局（基本合规，2 处可优化）

- **`token.go`（393 行 Keycloak JWKS/OIDC 验签）放在 `api/v1`** —— 这是认证**基础设施**而非 API 表面，应迁 `internal/platform/auth`，`v1` 只依赖 `TokenResolver` 接口。
- **`internal/platform/postgres/db`（sqlc 生成）与 `internal/repository/postgres`（适配器）命名易混** —— 可把生成包重命名为 `…/sqlc` 提升可读性。
- 其余 `internal/{api,service,repository,platform,domain}` 划分**符合 Go 社区惯例**（ports-and-adapters），保持即可。

---

## 4. 详细执行计划（分阶段，可逐 PR 落地）

> 总策略：**先压平（阶段一，低风险快赢，路线 A），再隔离（阶段二，路线 B 切域），后收尾（阶段三）**。每个任务标注【准则】【风险】【验证】，建议每个任务一个独立 PR。

### 阶段一：压平重复（1~1.5 周，纯内部重构，无契约破坏）

| 任务 | 内容 | 准则 | 风险 | 验证 |
|---|---|---|---|---|
| **T1.1 授权 guard** | 抽 `core.Authorize(ctx, req, auditTarget) (Account, CheckResult, error)`，迁移 ~16 处前导块；以 `authzToolGateway` 为蓝本 | 简洁 | 中 | 新增 guard 单测；逐方法对比迁移前后审计/拒绝行为快照 |
| **T1.2 泛型排序** | `list_sort.go` 10 函数 → `sortBy[T]` + 比较器表 | 简洁 | 低 | 既有排序单测保持绿 |
| **T1.3 合并分页类型** | `PageResponse`==`ListResponse` 合一；统一分页默认魔数（与 `postgres/store.go:467` 对齐 `normalizePageRequest`） | 简洁 | 低 | 编译 + API 响应快照 |
| **T1.4 抽 utils** | `copyStrings`/`containsString`/`removeString` 跨包去重，统一 `removeString` 语义 | 简洁 | 低 | 新增 utils 单测覆盖空/重复/不存在 |
| **T1.5 清死代码/双实现** | 删 `filterLeaveBalancesByEmployee` 等死代码；合并两份 unique-check | 简洁 | 低 | `go vet` + 覆盖率不降 |
| **T1.6 service.go 瘦身** | Agent/菜单/日期 helper 迁出至各归属文件 | 模块化 | 低 | 编译 + 对应域单测 |

### 阶段二：字段元数据 + 接口统一（2~3 周，根因级）

| 任务 | 内容 | 准则 | 风险 | 验证 |
|---|---|---|---|---|
| **T2.1 员工字段元数据表** | 建 `EmployeeSchema`，让 patch/forbid/mask/import/export **全部派生**（§3.3） | 可扩展 | 中 | 字段级表驱动单测；导入/导出/脱敏回归 |
| **T2.2 Store 签名统一** | 40 个非 ctx/error 方法补 `ctx+error`；删除 `must/mustNoValue` panic（§3.4） | 模块化/异常 | **高** | **真实 Postgres 集成测试**（testcontainers）覆盖错误传播/取消/回滚 |
| **T2.3 能力入接口** | 5 个扩展能力提升进 `EmployeeStore`/`AgentStore`/`AuditStore`，删 7 处类型断言与 memory fallback（§3.5） | 模块化 | 中 | 同一套用例跑 memory + postgres，断言**语义一致** |
| **T2.4 拆 postgres/store.go** | 1273 行 → `store.go`/`mappers.go`/`pgtypes.go`（§3.8） | 简洁 | 低 | 纯文件移动，编译即验证 |
| **T2.5 拆 evaluateAuthz** | 110 行 → `collectGrants`/`matchGrants`/`fallbackReBAC`/`assembleDecision`（§3.8） | 简洁 | 中 | 授权决策快照测试（含 deny/boundary/approval/OpenFGA 分支） |

### 阶段三：领域模块化切域（3~4 周，路线 B 主体，灰度推进）

| 任务 | 内容 | 准则 | 风险 | 验证 |
|---|---|---|---|---|
| **T3.1 抽共享 core** | `evaluateAuthz`/`audit`/`withTenantTransaction`/`resolveAccount` 收敛为 `core` 协作者，供各子包注入 | 模块化 | 中 | core 独立单测 |
| **T3.2 先切 hr + attendance** | 拆 `service/hr`、`service/attendance` 子包，各自定义**窄 repo 接口**；attendance 对 workflow 的依赖改为**注入 workflow 窄接口**（消除 `CreateLeaveRequest` 越界，§3.8） | 模块化/可扩展 | 高 | 子包用 mock 窄接口独立测试；端到端回归 |
| **T3.3 再切 iam/authz/agent/workflow** | 同模式逐域迁移 | 模块化 | 高 | 分域灰度，每域一 PR |
| **T3.4 删转发层** | 子服务成为注入式 use-case，删除 59 转发方法，`Service` 降级为装配根（§3.1） | 简洁 | 中 | API 层改注入点；全量回归 |
| **T3.5 token.go 归位** | 迁 `internal/platform/auth`，`v1` 仅依赖 `TokenResolver` 接口（§3.9） | 模块化 | 低 | 编译 + token 单测 |

### 里程碑与净收益预估
- 阶段一完成：净删 **~400 行**重复代码，授权/排序/分页逻辑唯一化。
- 阶段二完成：字段扩展从"改 5 处"降到"改 1 处"；持久层异常可处理、双后端语义一致。
- 阶段三完成：模块边界由"组织级"升级为"依赖级"，新增业务域**零耦合**他域 store，转发层消失。

---

## 5. 风险控制与验证基线

1. **回归安全网先行**：阶段二的 Store 签名统一（T2.2）改动面最大，**必须先有真实 Postgres 集成测试**（RLS、事务回滚、取消、取号并发、分页语义），否则不开工。
2. **逐域灰度**：阶段三按域拆 PR，hr/attendance 先行验证模式，再推 iam/authz。
3. **行为快照**：授权（T1.1/T2.5）与脱敏（T2.1）属安全敏感，迁移前后用决策/输出快照逐项比对，杜绝隐性放权。
4. **每个 PR 可独立回滚**：阶段一各任务彼此解耦；阶段二 T2.1/T2.2/T2.3 有依赖，按序合并。
5. **CI 门禁**：补 `golangci-lint`（gocyclo 限制上帝函数复发）、`go test -race`、`sqlc diff`（防 `schema.sql` 与迁移再次漂移）。

---

## 6. 一句话总结

你已经把"安全与正确性"补齐；这一轮要补的是**"边界的真实性"**——当前的子服务、子接口都拆在了文件层面，却没拆在依赖层面。沿"**先压平重复（A），再按域隔离依赖（B）**"推进，用一张员工字段元数据表、一处授权 guard、一套统一的 Store 签名，把"简洁、模块化、高可扩展性"从口号落到编译器能强制保证的结构上。**最值得先做的三件事：T1.1（授权 guard）、T2.1（字段元数据表）、T2.2（Store 签名统一）**——它们分别命中简洁、可扩展、模块化三条准则的根因。
