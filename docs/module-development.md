# 模块开发规范

本文档约束当前 modular monolith 的逻辑模块化方式。目标是在不提前搬迁到 `internal/modules/...` 的前提下，让后续财务、产品、销售、研发等模块按一致边界落地。

## 适用原则

- 新模块先按现有目录分层落地，不提前创建空接口、空目录或占位路由；service 逻辑优先按模块集中到一个 `<module>_service.go`，小能力并入现有模块文件。
- 不跨过 service facade 直接调用其他模块的 repository 写路径。
- 对外 HTTP、OpenAPI、route policy、service facade、repository 子接口、memory/postgres 实现和测试必须同步演进。
- 模块稳定前优先保持小步提交和小范围验证；业务闭环稳定后再评估是否迁移到物理模块化目录。

## 新模块文件布局模板

以 `billing` 为例，新模块应按下面的逻辑位置新增文件：

```text
internal/domain/
  billing.go
  billing_inputs.go

internal/service/
  billing_service.go
  facades.go             # 扩展 BillingFacade；不要为少量方法拆小 facade 文件

internal/repository/
  billing_store.go

internal/repository/memory/
  store.go                # 新增 BillingStore 方法实现，或拆出 billing.go

internal/repository/postgres/
  store.go                # 新增 BillingStore 方法实现，或拆出 billing.go

internal/api/v1/
  billing.go

db/queries/
  billing.sql

db/migrations/
  <next>_billing.sql

tests/unit/
  api/v1/
  service/
  repository/memory/
  repository/postgres/
```

如果模块需要后台作业或外部平台适配，再按现有模式放到 `internal/jobs` 或 `internal/platform/<provider>`，不要把平台细节混入 service。

## 新模块接入 Checklist

- 定义 domain entity、input DTO 和必要 enum；DTO 类型名和 JSON tag 一开始就按 OpenAPI 契约定准。
- 新增或扩展 service facade，例如 `BillingFacade`；facade 定义集中维护，模块内 service 逻辑优先留在 `<module>_service.go`，避免为几个方法拆出过多小文件。
- 新增 repository 子接口，例如 `BillingStore`，并把它加入 `repository.Store` 聚合接口。
- 补齐 `memory.Store` 和 `postgres.Store` 实现，保留 `var _ repository.Store = (*Store)(nil)` 编译期保障。
- 注册 API v1 controller route，controller 只依赖对应 facade，不直接依赖具体 service。
- 更新 `docs/openapi.yaml`，保持 route、request、response envelope 与真实 handler 对齐。
- 更新 `internal/domain/authz.go` route policy，并确保 route parity 测试覆盖。
- 更新 seed permission，至少包含 admin 可操作权限；高风险按钮要显式标注高风险。
- 写入审计事件名和资源名，写操作和高风险读操作需要可追踪。
- 增加 unit tests，覆盖 service 直接调用、API route、repository memory/postgres 关键路径。

## 跨模块调用规则

- 业务模块不能直接写其他模块 repository。
- 跨域读优先通过对方 service facade；跨域写优先通过对方 facade 或后续事件机制。
- 跨模块引用只保存稳定 ID、快照或必要冗余字段，避免运行时强耦合。
- 需要一致性的跨模块写入必须在调用方明确事务边界；没有事务保障时使用 outbox/event 语义。
- 共享基础能力，例如 authz、audit、tenant、object store，按现有平台边界复用，不复制到业务模块。

## 权限和审计规则

- 所有非公开 route 必须有 route-level authz。
- service 写路径和高风险读路径必须有 service-level authz，不能只依赖 HTTP middleware。
- 财务、销售等高风险模块必须同时满足 route-level authz、service-level authz 和 audit。
- 高风险操作必须走 approval confirmation 或 approval instance；测试 fixture 要显式体现这一点。
- route policy 的 `resource/action` 必须和 service 写路径、seed permission、audit event 一起核对。

## DB 和 Repository 规则

- 所有业务表默认带 `tenant_id`，遵循当前 tenant/RLS 隔离模式。
- 新模块使用独立 migration 和 query 文件，不把新业务 SQL 混进已有模块 query 文件。
- 写路径优先使用现有 transaction helper，保证错误和 panic 都能回滚。
- repository/store 方法必须显式传递 `context.Context`，请求链路不要退回 `context.Background()`。
- 新增 `db/queries/*.sql` 后执行 `make sqlc`；新增 migration 后执行 `make migrate-validate`。

## 后续四模块默认边界

### 产品模块

负责产品、SKU、价格、套餐、版本、发布。

产品模块提供价格和产品快照给销售使用。销售可以引用产品快照，但不能直接修改产品定价或发布状态。

### 销售模块

负责线索、客户、商机、合同、订单。

销售合同或订单可以触发财务应收或开票请求，但销售不能直接写财务账、发票状态或收款记录。

### 财务模块

负责发票、收款、付款、费用、预算、账务流水。

财务模块自己生成和维护财务记录。其他模块只能提交请求、引用结果或读取授权范围内的财务状态。

### 研发模块

负责项目、需求、迭代、任务、缺陷、发布交付。

研发交付结果可以同步到产品版本，但研发不能直接修改产品商业定价、套餐规则或销售合同。

### HR 作为基础引用

HR 提供人员、组织、负责人信息给销售、研发、财务引用。引用方保存必要快照和 ID，不能直接写 HR employee/org repository。

## 推荐落地顺序

如果未来先实现四个模块中的一个，建议优先从产品或销售开始。产品能定义价格和 SKU 边界，销售能定义合同/订单边界，这两者会自然约束财务和研发的上游输入。
