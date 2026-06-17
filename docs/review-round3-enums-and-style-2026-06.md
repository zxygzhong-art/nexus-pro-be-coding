# 第三轮 Review：残留问题 · 魔法字符串枚举化 · API/Service 写法

> 日期：2026-06-16（第三轮）
> 范围：全量代码复审 + 两个专题——(A) 字符串字面量 vs 枚举/常量；(B) API 层与 Service 层写法是否得当、有无更优写法
> 约束：**不改代码，仅给方案与示例**

---

## 0. 先确认进展（第二轮方案已落地的部分）

| 第二轮建议 | 状态 | 证据 |
|---|---|---|
| T1.1 抽授权 guard | ✅ 已做 | `c.Authorize(ctx, CheckRequest, AuditTarget) (Account, CheckResult, error)`，`hr_service.go:151/183/241` 等已迁移 |
| T1.6 `service.go` 瘦身 | ✅ 已做 | 461→297 行；新增 `agent_knowledge.go`/`me_menu.go`/`timeutil.go` |
| T1.4 抽 utils | ✅ 已做 | 新增 `internal/utils` |
| T1.2 泛型排序 | ◑ 部分 | `list_sort.go` 139→115，仍是每类型一函数 |
| CSV 文件名头注入 | ✅ 已修 | `hr.go:146` 改用 `mime.FormatMediaType` |
| 响应信封统一 | ✅ 基本统一 | `writeJSON` 统一 `{data}`，`writeError` 统一 `{error}` |

进展明显。本轮聚焦你新提的两个专题，并补充复审中发现的残留问题。

---

## 1. 残留问题清单（按优先级，附修复方案）

### 🔴 R1. 授权成功路径"双重记账"——guard 已抽，但审计仍重复手写
`Authorize` 内部（拒绝时）已用 `AuditTarget` 记审计；但**成功**后业务方法又**重复**调用 `auditAuthzDecision(...)`，把同一组字符串再写一遍：
```go
// hr_service.go:151-178  ExportEmployees
account, decision, err := c.Authorize(ctx,
    CheckRequest{ApplicationCode: "hr", ResourceType: "employee", Action: "export"},
    AuditTarget{Action: "hr.employee.export", Resource: "employee_collection"})   // ← 审计目标传了一次
...
c.auditAuthzDecision(ctx, "hr.employee.export", "employee_collection", "", decision)  // ← 同样的串又写一次
```
**问题**：`"hr.employee.export"`/`"employee_collection"` 在一个方法里出现两遍，8 处方法都如此（`auditAuthzDecision` 共 8 个调用点），改一处易漏另一处。
**修复**：让 guard 返回一个"审计收尾器"或承担成功审计：
```go
func (c *core) Authorize(...) (Account, CheckResult, AuthzAudit, error)
// AuthzAudit.Commit(ctx) 在业务成功后调用，或用 defer 模式
defer audit.Commit(ctx)   // 成功/失败都从同一 AuditTarget 派生，零重复
```

### 🟠 R2. 字段命名碰撞：`CheckRequest.Action` 与 `AuditTarget.Action` 同名异义
- `CheckRequest.Action` = 鉴权动词，**裸词**："export" / "delete" / "update_status"
- `AuditTarget.Action` = 审计标签，**点分全名**："hr.employee.export"

两个结构体都叫 `Action`，但格式和语义不同，读代码时极易混淆，也是 R1 重复的根源（两套命名各写一遍）。
**修复**：审计标签字段改名为 `AuditTarget.Event`（或 `Label`），并由 `ApplicationCode + ResourceType + Action` **派生**点分名，而非手写：
```go
func (r CheckRequest) AuditEvent() string { // "hr"+"employee"+"export" → "hr.employee.export"
    return r.ApplicationCode + "." + r.ResourceType + "." + r.Action
}
```
这样 R1 的重复与 R2 的碰撞一起消失。

### 🟠 R3. 列表信封不一致：`exportEmployees` 手搓 `{items}`，其余用类型化 `PageResponse`
```go
// hr.go:167  导出：手搓 map → {"data":{"items":[...]}}
writeJSON(w, http.StatusOK, map[string]any{"items": items})
// hr.go:49 / 243  列表：类型化 → {"data":{"items":[...],"total":N,"page":1,...}}
writeJSON(w, http.StatusOK, response)        // PageResponse[Employee]
```
同样是"员工列表"，两个端点返回**结构不同**的 body。
**修复**：导出也走 `domain.ListResponse[Employee]`（或统一 `PageResponse`），删除手搓 `map[string]any`。配合 §2 让"列表信封"只有一种形状。

### 🟠 R4. `updateEmployeeStatus` 用匿名内联 struct，破坏 DTO 一致性
```go
// hr.go:219-222 —— 唯一一个不用 domain.XInput 的 handler
var input struct {
    Status string `json:"status"`
}
```
其余 handler 全部用 `domain.CreateEmployeeInput` 等具名输入类型。
**修复**：定义 `domain.UpdateEmployeeStatusInput{ Status string }`，与其它 handler 对齐；顺带让 `Status` 走 §2 的 `EmployeeStatus` 类型。

### 🟡 R5. 路径参数名 `"id"` 作为魔法串重复出现
路由声明 `TargetEmployeeID("id")` 与 handler 内 `r.PathValue("id")` 各写一遍字面量 `"id"`，跨 12 条 `/:id` 路由。改路由参数名会静默断链。
**修复**：定义 `const PathParamID = "id"`，或让 `RouteOption` 把解析好的 ID 注入 `RequestContext`，handler 直接取 `ctx.ResourceID`，不再各自 `PathValue`。

### 🟡 R6. `list_sort.go` 仍是 10 个雷同函数（第二轮 T1.2 未收尾）
每个 `sortXxx` 都是 copy→switch→`sortSlice`，仅比较器不同。建议按第二轮方案用**比较器表 + 泛型** `sortBy[T]` 收敛（见 §4.4）。

### 🟡 R7. 业务硬编码字符串散落（与 §2 同源）
`next.Status = "deleted"`、`account.Status = "disabled"`、`appendEmployeeEvent(ctx, "employee.offboarded", ...)`、历史原因 `"刪除"`/`"狀態更新"` 等直接写在业务流程里（`hr_service.go:207-220`）。统一并入 §2 的枚举化。

---

## 2. 专题 A：字符串字面量 → 类型化枚举/常量

### 2.1 现状量化（复审实测）

- 直接相等比较 `== "x"` / `!= "x"`：**38** 处
- `switch { case "x": }`：**64** 处
- 目前**仅** `RiskLevel`（`domain/authz/types.go`）和分页常量是类型化的；**其余领域取值全是裸串**。

按语义归类（均为当前散落的裸串）：

| 枚举域 | 取值 | 出现量级 | 当前定义处 |
|---|---|---|---|
| **Effect** | allow / deny | 8+ | 无（裸串）|
| **Severity**（审计）| low / medium / high / critical | 23+ | 无 |
| **PrincipalType** | account / user_group / assumable_role | 14 | 无 |
| **Scope** | self / all / department_subtree / direct_reports | 50 | 无 |
| **ApplicationCode** | hr / iam / attendance / agent / workflow / audit / platform | 18+ | 无 |
| **ResourceType** | employee / leave / org_unit / assumable_role / tool … | 18+ | 无 |
| **Action** | read / create / update / delete / export / import / assume / invite / submit / call / update_status / status_transition | 50+ | 无 |
| **EmployeeStatus** | active / probation / leave_suspended / onboarding / resigned / deleted（+ 在職/離職/留停…）| 多 | 无 |
| **EmployeeCategory** | full_time / part_time / intern / contractor / other（+ 全職/兼職…）| 多 | 无 |
| **FieldPolicyEffect** | mask / hide / readonly / deny | 8+ | 无 |
| **AccountStatus** | active / disabled | 几处 | 无 |
| **EventType** | employee.offboarded / employee.status_changed … | 几处 | 无 |

### 2.2 为什么必须枚举化（风险）

1. **拼写错误静默通过编译**：`case "leave_suspeded"`（少个 n）不会报错，只会默默走 `default` 返回 false——一个**永不触发的权限分支**，排查极难。
2. **无 IDE 补全、无"查找引用"**：重命名一个取值要全局 grep，漏改即 bug。
3. **无穷举检查**：新增一个 `EmployeeStatus`，编译器不会提醒你哪些 switch 没覆盖。
4. **双语重复散落**（最危险）：`normalizeEmployeeStatus` 已把中文归一为英文规范值，但 `employee.go` 的 `EmployeeStats` 等处**仍同时比对** `"active"`/`"在職"`——归一后中文分支其实是**死代码**，说明"何时已归一"在脑中无单一模型，极易在新代码里再写错。

### 2.3 设计方案：领域内"具名字符串类型 + 常量 + 校验"

Go 惯例用**具名字符串类型**（named string type）承载枚举，兼顾 JSON 兼容与类型安全：

```go
// internal/domain/enum.go （或按域分文件）
package domain

type EmployeeStatus string

const (
    EmployeeStatusActive         EmployeeStatus = "active"
    EmployeeStatusProbation      EmployeeStatus = "probation"
    EmployeeStatusLeaveSuspended EmployeeStatus = "leave_suspended"
    EmployeeStatusOnboarding     EmployeeStatus = "onboarding"
    EmployeeStatusResigned       EmployeeStatus = "resigned"
    EmployeeStatusDeleted        EmployeeStatus = "deleted"
)

// 集中校验 + 双语归一（唯一真源）
func ParseEmployeeStatus(raw string) (EmployeeStatus, bool) {
    switch strings.TrimSpace(raw) {
    case "active", "在職":          return EmployeeStatusActive, true
    case "resigned", "離職":        return EmployeeStatusResigned, true
    case "leave_suspended", "留停": return EmployeeStatusLeaveSuspended, true
    // …
    default:                        return "", false
    }
}
func (s EmployeeStatus) Valid() bool { /* switch 已知值 */ }
```

**收益对比（前后）**：

```go
// 前：裸串，拼错静默失败，双语在业务层重复
if emp.Status == "active" || emp.Status == "在職" { ... }   // 散落多处
next.Status = "deleted"

// 后：编译期约束，归一只在边界发生一次，业务层只认规范常量
if emp.Status == domain.EmployeeStatusActive { ... }
next.Status = domain.EmployeeStatusDeleted
```

同法定义 `Effect`、`Severity`、`Scope`、`PrincipalType`、`FieldPolicyEffect`、`AccountStatus`。

对 **Application / Resource / Action** 这类"鉴权三元组"，建议进一步收敛为**集中目录**，让路由策略、`CheckRequest`、审计标签**同源**：
```go
type App string;  type Resource string;  type Action string
const ( AppHR App = "hr"; ResEmployee Resource = "employee"; ActExport Action = "export" )
// CheckRequest 字段改为强类型；AuditEvent() 由三者派生（呼应 R2）
```

### 2.4 落地"何时归一"的纪律（修死代码）

规则：**只在系统边界归一一次，内部一律只用规范常量比较。**
- 入口（API 解析、Import 解析）：调用 `ParseEmployeeStatus` 把中文/英文/别名归一为 `EmployeeStatus` 常量；非法值立刻返回 400。
- 内部所有 service/repo：**只比较常量**，删除所有 `"active" || "在職"` 这类双语分支（消除 §2.2-4 的死代码）。
- 出口：需要中文展示时由**展示层**做 `Status→Label` 映射，不在业务流里塞中文。

### 2.5 迁移策略（低风险、分域推进）

1. **先建类型与常量**（`domain/enum.go`），不改调用点——零风险，先让常量可用。
2. **按域替换**：一次一个枚举域（先 `Effect`/`Severity` 最简单，再 `EmployeeStatus`/`Scope`），用 `grep` 定位所有裸串逐个替换为常量。
3. **校验集中化**：把分散的 `normalizeEmployeeStatus`/`validEmployeeStatus` 合并进 `ParseEmployeeStatus`/`(s).Valid()`，删旧函数。
4. **加 linter 防回潮**：启用 `golangci-lint` 的 `goconst`（检测重复裸串）+ 自定义 `forbidigo` 规则（禁止在 service 包内出现这些已枚举化的中文/英文裸串）。
5. **JSON 兼容**：具名字符串类型序列化结果与原字符串一致，**前端无感**，无需改契约。

> 注意：DB 侧 sqlc 仍是 `text`，无需改 schema；只是 Go 层用具名类型承载，存取时隐式转换即可。

---

## 3. 专题 B-1：API 层写法评估

### 3.1 现状（总体优秀）
当前 handler 采用自定义签名 `func(w, r, ctx) error`，配合 `ginHandle` 适配器 + `routeBinder`，共享 `readJSON`/`readOptionalJSON`/`writeJSON`/`writeError`/`pageRequestFromRequest`。**样板极少、信封统一、加路由仅一行**——这是好设计，值得保留。

### 3.2 两种主流写法对比（你问的"有没有更好的写法"）

| 维度 | **A. 当前：自定义 HandlerFunc + 手解析** | **B. Gin 原生：`gin.Context` + `ShouldBindJSON` + binding tag** |
|---|---|---|
| 与框架耦合 | 低（handler 不依赖 gin，易迁移/易测） | 高（handler 绑定 `*gin.Context`）|
| 字段校验 | **手写**（service 层兜底）| 框架自带 `binding:"required,email"` 声明式校验 |
| 样板量 | 已很少 | 更少（绑定+校验一行）|
| 错误一致性 | 强（单点 `writeError`）| 需手动统一 validator 错误格式 |
| 可测试性 | 强（纯 `httptest`，无需 gin engine）| 一般（要构造 gin.Context）|
| 学习成本 | 低 | 低 |

**结论**：当前 A 风格**总体更优**（解耦+可测），**唯一短板是缺少声明式字段校验**——不必整体倒向 B，而是**在 A 上补一层校验**即可（见 3.3-②）。

### 3.3 具体改进点（在保留 A 的前提下）

**① 列表信封统一**（呼应 R3）：导出端点弃用手搓 `map{"items"}`，统一 `ListResponse[T]`。让"集合响应"只有一种形状，前端解析逻辑唯一。

**② 引入轻量校验，替代"裸解析 + service 兜底"**
现状 `createEmployee` 只 `readJSON` 进 `domain.CreateEmployeeInput`，所有 required/格式校验都压到 service。可在解码后加一步**显式 Validate**，把"输入合法性"留在 API 边界、"业务规则"留在 service：
```go
// 输入类型自带校验，边界即拦截
func (in CreateEmployeeInput) Validate() error { /* 返回 *AppError(ValidationFailed) */ }

// handler：
if err := readJSON(w, r, &input); err != nil { return err }
if err := input.Validate(); err != nil { return err }   // ← 新增一行，错误走同一 writeError
```
- 优点：职责清晰（边界校验 vs 业务校验）、错误格式统一、不引入 gin 绑定耦合。
- 缺点：Validate 需手写（可用 `go-playground/validator` 在结构体上加 tag + 一个通用 `validate.Struct` 封装，兼顾声明式与解耦）。

**③ 匿名 struct → 具名 DTO**（呼应 R4）：`updateEmployeeStatus` 的内联 struct 提为 `domain.UpdateEmployeeStatusInput`。

**④ 路径参数常量化 / 注入**（呼应 R5）：消除 `"id"` 魔法串重复，最好由中间件把已解析的资源 ID 放进 `RequestContext`。

**⑤ `writeJSON` 的编码错误可观测**：当前 `_ = json.NewEncoder(w).Encode(payload)` 吞错。可在编码失败时记一条 error 日志（header 已发无法改状态码，但至少可观测）。

### 3.4 一个"更好写法"的小例子（handler 进一步收敛）
大量 handler 是"解析 body → 调 service → 写 201/200"。可用一个泛型助手把三步收敛：
```go
func handleJSON[I any, O any](status int, fn func(domain.RequestContext, I) (O, error)) HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
        var in I
        if err := readJSON(w, r, &in); err != nil { return err }
        if v, ok := any(in).(interface{ Validate() error }); ok {
            if err := v.Validate(); err != nil { return err }
        }
        out, err := fn(ctx, in)
        if err != nil { return err }
        writeJSON(w, status, out)
        return nil
    }
}
// 注册：c.routes.Handle("hr.employee","create", handleJSON(201, c.svc.CreateEmployee))
```
- 优点：create/update 类 handler 从 ~10 行降到 0 行（直接复用）；校验自动接入。
- 缺点：路径参数/多入参场景仍需手写 handler（不强求全覆盖，覆盖 70% 的 CRUD 即可）；泛型让调用签名稍抽象。
- 取舍：建议**仅对标准 CRUD 用**，特殊 handler（导入多形态、导出 CSV）保留显式写法。

---

## 4. 专题 B-2：Service 层写法评估

### 4.1 `Authorize` guard——已抽得好，再补两刀
guard 抽取是本轮最大改进。残留两点（已在 R1/R2 详述）：
- **成功审计仍重复**→ 让 guard 返回审计收尾器或 `defer audit.Commit()`。
- **`Action` 命名碰撞**→ 审计标签字段改名 `Event` 并由三元组派生。

改进后业务方法形态（对比）：
```go
// 前：guard + 末尾再手写一遍审计串
account, decision, err := c.Authorize(ctx, CheckRequest{...,"export"}, AuditTarget{"hr.employee.export","employee_collection"})
...
c.auditAuthzDecision(ctx, "hr.employee.export", "employee_collection", "", decision)  // 重复

// 后：审计目标只声明一次，成功/失败都由它派生
account, decision, done, err := c.Authorize(ctx, CheckRequest{App:AppHR, Res:ResEmployee, Act:ActExport})
if err != nil { return nil, err }
defer done(ctx)   // 成功收尾审计；事件名 = req.AuditEvent()
```

### 4.2 事务内"读-改-写"散落字符串
`DeleteEmployee` 事务体里 `"deleted"`/`"disabled"`/`"employee.offboarded"`/`"刪除"` 全裸串（`hr_service.go:207-220`）。并入 §2 枚举化后：
```go
next.Status = domain.EmployeeStatusDeleted
account.Status = domain.AccountStatusDisabled
tx.appendEmployeeEvent(ctx, domain.EventEmployeeOffboarded, next.ID, ...)
```

### 4.3 子服务仍内嵌 `*Service`（第二轮路线 B 尚未做）
本轮不强求，但提醒：当前 `HRService struct{ *Service }` 仍可摸到全量 store 与他域 helper，模块边界仍是"组织级"而非"依赖级"。待枚举化与 guard 收尾后，再按第二轮路线 B 切窄接口注入。

### 4.4 `list_sort.go` 泛型收敛（呼应 R6）
```go
// 前：10 个雷同函数
func sortUserGroups(items []UserGroup, sort string) []UserGroup { copy→switch→sortSlice }
// …×10

// 后：一个泛型 + 每类型一张比较器表
type cmpTable[T any] struct{ def string; by map[string]func(a,b T) bool }
func sortBy[T any](items []T, key string, t cmpTable[T]) []T { ... }
var userGroupCmp = cmpTable[UserGroup]{def:"created_at_desc", by: map[string]func(a,b UserGroup)bool{
    "name_asc":       func(a,b UserGroup)bool{return a.Name<b.Name},
    "created_at_asc": func(a,b UserGroup)bool{return a.CreatedAt.Before(b.CreatedAt)},
}}
```
- 优点：10 函数 115 行 → 1 泛型 + N 张小表，新增排序键只加一行 map。
- 缺点：泛型可读性略低于平铺 switch（可接受）。

---

## 5. 执行优先级（建议顺序）

| 顺序 | 任务 | 准则命中 | 风险 | 价值 |
|---|---|:---:|:---:|:---:|
| 1 | **§2 枚举化**：先建 `domain/enum.go`，按域替换裸串；归一只在边界 | 可读/正确性 | 低 | ⭐⭐⭐ |
| 2 | **R1+R2**：guard 收尾审计 + `AuditTarget.Action`→`Event` 派生 | 简洁 | 低 | ⭐⭐⭐ |
| 3 | **R3+R4**：列表信封统一 + status 匿名 struct 提 DTO | 一致性 | 低 | ⭐⭐ |
| 4 | **§3.3-②**：API 边界加 `Validate()` 一层 | 模块化 | 低 | ⭐⭐ |
| 5 | **R6/§4.4**：泛型排序收尾 | 简洁 | 低 | ⭐⭐ |
| 6 | **R5**：路径参数注入 `RequestContext` | 简洁 | 中 | ⭐ |
| 7 | linter 防回潮：`goconst`+`forbidigo`+`gocyclo` | 长效 | 低 | ⭐⭐ |

> 一句话：第三轮你已经把"重复的控制流"（guard）压平了，接下来最高价值的是把"**重复的取值**"（魔法字符串）也压成**编译器能管的类型**——它同时解决可读性、防拼写错、消双语死代码三件事；API/Service 写法整体已经得当，只需在边界补一层校验、把成功审计与列表信封的"最后一点不一致"抹平即可。

---

## 附：本轮发现一并speed-check的结论
- ✅ CSV 头注入已修；响应信封已统一；service.go 已瘦身；guard 已抽——第二轮主要项达标。
- ⚠️ 真实 Postgres 集成测试（第一/二轮 E2/T2.2 前置）仍建议尽快补，否则 §2 替换与未来 Store 签名统一缺回归网。
