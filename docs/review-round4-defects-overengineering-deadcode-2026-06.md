# 第四轮 Review：致命缺陷 · 过度设计 · 无效代码 · 优化空间

> 日期：2026-06-16（第四轮，全量复审）
> 验证基线：`go build ./...` ✅ · `go vet ./...` ✅ · `go test ./...` ✅（全绿）· `deadcode` 工具已跑
> 关注点：致命缺陷 / 过度设计 / 无效代码 / 优化空间
> 约束：**不改代码，仅给结论与修复方向**

---

## 0. 总评

代码已经过三轮整改，**能编译、vet 干净、测试全绿**，整体结构清晰。本轮用"编译器 + deadcode 工具 + 逐路径追踪"做了一次冷静复审，结论分四类：

| 类别 | 结论 | 最严重项 |
|---|---|---|
| 🔴 致命缺陷 | **有 2 个已确认的真实 Bug + 4 个高危** | 事务非 panic-safe（连接泄漏）；XLSX 主管列静默丢数据 |
| 🟠 过度设计 | **有，集中在"半成品抽象"** | 枚举半连线（worst-of-both）、jobs 整包未接入、59 个门面转发 |
| ⚪ 无效代码 | **有，可安全删除** | 整个 `jobs` 包 + 4 个死函数 + 24 个未用常量 |
| 🟢 优化空间 | **有，非阻塞** | 分页仍"全量加载再切片"、审计双写、排序样板 |

---

## 1. 🔴 致命缺陷（按严重度）

### F1【已确认·最高】事务不是 panic-safe + 30 个 store 方法 `must()` panic → 连接泄漏 + 请求 goroutine 崩溃
**证据**（`internal/repository/postgres/store.go:35-54`，已逐行确认）：
```go
func (s *Store) WithTenantTransaction(...) error {
    tx, err := s.pool.Begin(execCtx)
    ...
    if err := fn(txStore); err != nil {     // ← 只在“返回 error”时回滚
        _ = tx.Rollback(execCtx)
        return err
    }
    return tx.Commit(execCtx)               // ← 没有 defer tx.Rollback()
}
```
而事务闭包里大量调用的 store 方法（`UpsertLeaveRequest`、`UpsertFormInstance`、`UpsertAccount`、`AppendAuditLog`、`AppendAuthzOutboxEvent`、`IncrementPermissionVersion`…）**无 error 返回，靠 `must()` 直接 `panic`**（`store.go:858-867`）。

**触发场景**：`CreateLeaveRequest`（`attendance_service.go`）在事务内 `UpsertFormInstance`/`UpsertLeaveRequest`/`audit`。任一遇到约束冲突、序列化失败、连接抖动 → `panic` → **既不走 `tx.Rollback()` 也不走 `tx.Commit()`**，pgx 事务连接被泄漏（直到健康检查回收），同时请求 goroutine 崩溃（除非顶层有 recover）。高并发下连接池会被逐步抽干。
**修复方向**：
1. `WithTenantTransaction` 加 `defer func(){ if p:=recover(); p!=nil { _=tx.Rollback(execCtx); panic(p) } }()`，或改成标准 `defer tx.Rollback()`（Commit 后 Rollback 是 no-op）。
2. **根因**：消除 `must()` panic 模式——把这 30 个方法补齐 `error` 返回（即第一/二轮就提的"Store 签名统一"），让 DB 错误以 error 传播、由事务正常回滚。这是同一根因，**优先做**。

### F2【已确认·数据丢失】XLSX 导入时"主管员工ID"列被静默丢弃
**证据**（已确认）：`employee_import.go:389` 读 sheet 时 `record := make([]string, 9)`（下标 0–8），但 `employeeRowsFromRecords` 在 `padRecord(record, 10)` 后读 **`record[9]`**（`:311` 的 `"主管員工ID"`）。XLSX 路径永远只填 0–8，第 10 列单元格命中 `col >= len(record)`（`:395`）被 `continue` 丢弃，padRecord 再补一个空串。
**后果**：**所有 XLSX 导入的"主管员工ID"恒为空**——经理关系静默丢失（CSV 路径用 `ReadAll` 不受影响）。
**修复**：`readXLSXSheet` 的 `make([]string, 9)` 改为列数与 schema 一致（10），或由表头列数动态决定，并对"列数不足"返回行级错误而非静默吞掉。

### F3【确认·仍在】非 Employee store 方法全部 `context.Background()` → 无取消/超时，授权评估首当其冲
**证据**：`store.go:834` 的 `ctx()` 返回 `context.Background()`，被 ~40 个方法使用（`GetPermissionSet`/`ListPermissionSetAssignmentsForPrincipal`/`GetUserGroup`/`GetDataScope`/`AppendAuditLog`…）。
**后果**：`evaluateAuthz` 在**每个请求**都走这些方法 → 客户端断开或 deadline 到期后查询仍继续，`tenantDBTX` 为每条语句开的事务一直占着连接，压测下放大为连接池耗尽。
**修复**：与 F1 同根——Store 方法统一接收 `ctx context.Context`，由请求上下文贯穿。

### F4【高】`withTenantTransaction` 之外的多写操作不是原子的
**证据**：`tenantDBTX`（`tenant_dbtx.go:21-68`）为**每条语句各开一个事务并立即提交**。因此未包在 `withTenantTransaction` 里的多写 service 方法不是一个工作单元。例：`CreateOrgUnit`（`hr_service.go:133-134`）先 `UpsertOrgUnit`（提交），再 `audit`（另一个事务）；若 audit 失败/ panic，org unit 已落库。
**修复**：所有"多写"语义的 service 方法显式包 `withTenantTransaction`；或把"读-改-写""写+审计"约定为必须在事务内。

### F5【高】`department_subtree` / `direct_reports` 数据范围：选择顺序相关 + org 重组后缓存陈旧
**证据**：`chooseScope`（`authz_runtime.go:615-623`）只按 rank 取**单个**最高 scope 且严格 `>`，并发/ map 序导致同 rank 的 grant 谁胜不确定，条件（`org_unit_ids`）不在等价 grant 间合并；同时 `CreateOrgUnit` **不调用** `touchAuthzConfig`，org 树重组后授权快照（含 `org_unit_ids` 子树，TTL 5 分钟）陈旧 → 经理看到错误的部门子树集合。
**后果**：数据范围过滤可能返回**过多或过少**员工，且与请求时序/缓存有关，难复现。
**修复**：等价/更高 rank 的 scope 条件应**合并（并集）**而非取一；org 单元结构变更（创建/重挂父）也要 `touchAuthzConfig` 失效相关快照。

### F6【中高】成功路径的"授权决策审计"在 create/update/import 上被丢弃
**证据**：`Delete/Invite/Transition/Export/UpdateStatus` 走新 `Authorize`+`AuthzAudit.Commit` 会记录成功决策；但 `CreateEmployeeAggregate`/`UpdateEmployee`/`QueryEmployees`/`PreviewEmployeeImport`/`ConfirmEmployeeImport` 仍直接用旧 `evaluateAuthz`，**成功时只写业务审计、不写授权决策审计**（`employee.go:81,96` 等）。
**后果**：审计追踪"拒绝记录详尽、成功的高权限创建却缺授权上下文"——合规与取证缺口。
**修复**：把这些方法也迁到 `Authorize`+`Commit`，统一成功/失败两路都记授权决策（呼应第三轮 R1 的 guard 收尾器）。

> 说明：上面 F1、F2 已通过直接读码确认为真实 Bug；F3、F4 是结构性确认项；F5、F6 是逐路径推断的高危项，建议加针对性测试复现后修复。

---

## 2. 🟠 过度设计（"抽象的成本 > 收益"）

### O1【最典型】枚举"半连线"——12 个类型 + 60 个常量，但字段仍是 `string`，等于 worst-of-both
**现状**：`domain/enums.go` 定义了 `Effect/Severity/Scope/Action/...` 12 个具名类型与常量，但 `CheckRequest` 等结构体字段**仍是 `string`**（`iam.go:25-30`）。于是调用处写成：
```go
CheckRequest{ApplicationCode: string(AppHR), ResourceType: string(ResourceEmployee), Action: string(ActionExport)}
```
- **比原来更啰嗦**（`string(AppHR)` 比 `"hr"` 长），却**没有编译期安全**——字段是 `string`，`Action: "expart"` 照样编译通过。
- **采纳不一致**：`hr_service.go` 用 `string(AppHR)`，但 `employee_import.go:22` 仍是裸 `"hr"`/`"employee"`/`"import"`。全仓 **83 处** domain 裸串残留（11 个 service 文件）。
- **24 个常量定义了但从未使用**（见 §3）。

**这是典型过度设计**：抽象建了一半，既没拿到类型安全，又增加了样板与认知负担。
**两条出路（二选一，别停在中间）**：
- **A（推荐）真枚举**：把 `CheckRequest.ApplicationCode/ResourceType/Action/Scope`、`Employee.Status` 等字段**直接改成具名类型**。这样赋值无需 `string(...)`，`Action: ActionExport` 直接成立，拼错不编译；JSON 序列化值不变，前端无感。
- **B 收手**：若决定字段保持 `string`，就**删掉这些类型**，只保留一组 `const X = "hr"`（无类型），调用处直接 `ApplicationCode: AppHR`（无 `string()`），并补 linter 防裸串。
> 现状（类型存在但靠 `string()` 转回）是最差选项，务必收敛到 A 或 B。

### O2 `jobs` 包：完整异步框架，零接入
`internal/jobs/runner.go`（136 行：Runner/Register/Enqueue/Run/Get/copyJob）**整包不可达**（deadcode 工具确认，见 §3）。这是为"异步导出/Agent/审计归档"预留的骨架，但没有任何调用方，且 `Run` 是手动同步调用、无 worker 池、无持久化。
**判断**：属"提前抽象"。要么**本期接入**（导出/Agent 改异步走它），要么**先删**，等真要做时再加——留着是未测、会腐化的死代码。

### O3 59 个门面转发方法仍在（第二轮路线 B 未做）
`func (c *Service) X(...) { return c.HR().X(...) }` 仍有 **59 个**纯转发（`grep` 确认）。子服务 `struct{ *Service }` 内嵌，不构成真实边界。
**判断**：不是致命问题，但属"为统一调用面付出的重复成本"。可按第二轮方案让 API 直接持有子服务、删转发；本轮可暂缓。

### O4 `AuditEvent()` 设计了但几乎没用
`CheckRequest.AuditEvent()`（`enums.go:129`，含 `splitResourceName` 反推逻辑）**只有 1 处调用**（`service.go:128`）。这个"从 Resource 反向拆 app/resource"的兜底分支为极少调用面承载了不小复杂度。
**判断**：若审计事件名能由 `Authorize` 的入参直接拼出，`AuditEvent()` 的反推分支可大幅简化或内联。

### O5 双 store（memory + postgres）+ 能力类型断言的维护税
memory 926 行、postgres 1255 行，外加 `employeeQueryStore` 等**靠运行时类型断言**暴露扩展能力。这是"可测试性"换来的真实成本（双份 CRUD + 行为易分叉）。
**判断**：是**有意识的权衡**，非纯过度设计；但应通过"能力入接口（编译器强制 parity）"降低分叉风险，否则随功能增长，维护税持续上升。

---

## 3. ⚪ 无效代码（可安全删除 / deadcode 工具实测）

`deadcode ./...` 报告的不可达函数（main 可达性分析，已排除测试）：

| 死代码 | 位置 | 处置 |
|---|---|---|
| **整个 `jobs` 包** | `jobs/runner.go`（NewRunner/Register/Enqueue/Run/Get/copyJob）| 接入或删除（见 O2）|
| `PathParam` | `api/v1/routes.go:37` | 删除（已有 `ResourceID`/`TargetEmployeeID` 具体构造器）|
| `Service.nextEmployeeNo` | `employee_model.go:352` | 删除（memory/pg 都已实现 `NextEmployeeNo`）|
| `employeeNoSequence` | `employee_model.go:401` | 删除（同上，旧扫描式实现）|
| `filterLeaveRequestsByEmployee` | `service.go:275` | 删除（复数版 `...Employees` 才是在用的）|

**24 个未使用的枚举常量**（grep 实测 0 外部引用）：
`SeverityLow/SeverityCritical`、`AppAgent/AppWorkflow/AppAudit`、`ResourceOrgUnit/ResourceLeave/ResourceUserGroup/ResourcePermissionSet/ResourcePermissionAssign/ResourceDataScope/ResourceFieldPolicy/ResourceAssumableRole/ResourceTool`、`ActionAssume/ActionSubmit/ActionCall`、`FieldPolicyEffect*`（全部 5 个）、`AccountStatusActive`、`EmployeeCategoryFullTime`。
**处置**：与 O1 一起决策——若走"真枚举（A）"，这些会在替换裸串时被用上；若短期不替换，应删除以免误导。**当前状态（定义了却不用）是 deadcode**。

**其它**：
- `agent_knowledge.go:12` 的"占位 Agent Run"仍是关键词袋 stub，占着真实 `/v1/agents/runs` 端点（非死代码，但是**功能性占位**，应在路线上明确"何时接入真实 LLM/检索"）。
- `object_store` **不是**死代码（`employee_import.go:49` 在用），勿删。

---

## 4. 🟢 优化空间（非阻塞，按价值排序）

1. **分页仍"全量加载再内存切片"**：除 Employee/Agent/Audit 走了 SQL 分页，多数 `ListXxxPage`（org_unit、iam 系列、leave）仍是"`ListAll` → `sortXxx` → `pageResponse` 内存切片"。数据增长后是 O(n) 内存与带宽浪费。**方向**：下沉 `LIMIT/OFFSET`（或 keyset）到 SQL，对审计/agent 已做，推广到其余。
2. **审计双写**（第三轮 R1 残留）：成功路径 `Authorize` 已带 `AuditTarget`，业务末尾又手写 `auditAuthzDecision(...)` 重复同串。用 guard 的 `Commit` 收尾，消除重复（也顺带修 F6 一致性）。
3. **`list_sort.go` 10 个雷同排序**：用"比较器表 + 泛型 `sortBy[T]`"收敛（第三轮 R6/§4.4 已给示例），115 行 → 1 泛型 + N 张小表。
4. **员工关键词搜索 `LIKE '%kw%'`**：前导通配无法走索引，全表扫描；大租户加 `pg_trgm` GIN 或 FTS（第一轮已提，仍未做）。
5. **`tenantDBTX` 每条语句一个事务**的 chattiness：读多写少场景下 N 语句 = N 个 `BEGIN/SET/COMMIT`。可评估"按请求一个事务 + 一次 set_config"的连接级方案。
6. **`employeeRowsFromRecords` 列映射用魔法下标 `record[0..9]`**：与导出表头各写一份（第二轮提的"字段元数据表"未做）。表化后顺带根治 F2 类越界。

---

## 5. 处置优先级（建议顺序）

| 优先级 | 任务 | 类别 | 风险 |
|:---:|---|---|:---:|
| P0 | **F1**：`WithTenantTransaction` 加 defer-rollback / recover；并推进消除 `must()` panic（Store 签名统一） | 致命 | 中 |
| P0 | **F2**：修 XLSX `make([]string,9)`→列数对齐，列不足报行错 | 致命/数据 | 低 |
| P1 | **F3+F4**：Store 方法贯穿 `ctx`；多写方法包事务（与 F1 同根，一并做） | 致命 | 中 |
| P1 | **O1**：枚举收敛到"真枚举 A"或"无类型常量 B"，清掉 `string()` 与 83 处裸串、24 个废常量 | 过度设计/死码 | 低 |
| P1 | **F5/F6**：data-scope 条件合并 + org 变更失效；create/update 成功审计补齐 | 致命 | 中 |
| P2 | 删死代码（jobs/PathParam/nextEmployeeNo/employeeNoSequence/filterLeaveRequestsByEmployee） | 死码 | 低 |
| P2 | 优化项：SQL 分页下沉、审计去重、泛型排序、`pg_trgm` | 优化 | 低 |
| P2 | **测试网**：补真实 Postgres 集成测试（F1/F3/F4/F5 的回归前置，至今仍缺） | 工程 | 低 |

---

## 6. 一句话总结

整改到第四轮，代码**能跑、干净、有测试**，没有"架构级"返工需求。但还藏着 **2 个已确认的真实 Bug**——事务不是 panic-safe（叠加 `must()` 会泄漏连接并崩 goroutine）和 **XLSX 主管列静默丢数据**——这两个必须先修；**最大的"过度设计"是枚举只连了一半线**（类型有了、`string()` 转回、字段仍是 string、24 个常量空置、83 处裸串残留），要么做成真枚举、要么退回无类型常量，别停在中间；**最该删的无效代码是整个未接入的 `jobs` 包**。把 P0/P1 清掉后，剩下的分页下沉、审计去重、排序泛型都是锦上添花。**核心建议：先补一张真实 Postgres 集成测试网，再动 F1/F3/F4 这组同根的事务/ctx 修复——它同时关掉本轮一半的致命项。**
