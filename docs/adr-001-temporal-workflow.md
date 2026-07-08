# ADR-001：Temporal 长流程审批接入方案

- 状态：Implemented（第一批：表单审批）
- 日期：2026-07-07
- 决策人：架构评审

## 背景

架构设计文档（技术架构 / 技术选型对比）将 Temporal 定为审批、入职、转正、异动、离职等长流程的工作流引擎。当前实现为同步状态表流转（`form_instances` + `workflow_runs` + `workflow_service.go`），没有定时器、催办、超时升级、补偿逻辑。Compose 环境已包含 Temporal 服务。

## 决策

第一批接入对象为表单审批。Temporal 是表单审批默认且唯一的执行引擎，也是 API 启动硬依赖。

表单提交仍先写入现有查询投影，随后必须启动 `{tenant_id}:{form_instance_id}` workflow。approve / reject / return / withdraw 只能通过 signal 驱动 workflow，由 activity 复用 service 与 tenant context 写回 `form_instances` / `workflow_runs` 投影、审计和通知。若 workflow 不存在，API 返回明确错误，提示先执行 backfill；禁止回退到旧同步状态机。

## 触发条件（满足任意一条即启动实施）

1. 出现第一个需要**超时自动升级**或**定时催办**的审批场景（例如 N 天未审批自动转上级）。
2. 出现需要**长时间挂起/恢复**的流程（例如入职流程等待背调回执、合同续签等待窗口期）。
3. 出现多级串并联会签且需要**失败补偿**（例如审批通过后下游步骤失败需回滚已生效动作）。

## 接入方案

### 架构位置

- Go 模块化单体内新增 `internal/platform/temporal`（client 封装）与 `internal/workflows`（workflow/activity 定义）。
- Worker 以同进程 goroutine 启动（`cmd/api` bootstrap 中连接 Temporal 并注册 worker），连接 Temporal 失败则 API 启动失败。流量增长后可平移为独立部署单元，不改代码结构。

### 执行权威与读模型

- Temporal 成为流程**执行权威**；现有 `workflow_runs` / `form_instances` 表降级为**查询投影**（读模型），由 activity 在每个状态迁移时更新，保证现有查询 API 与前端不变。
- approve / reject / return / withdraw API 只向 workflow 发 signal；同步等待查询投影更新后保持接口响应语义。
- `ActOnWorkflowStage` 保留为 activity 的投影写入实现，不再由 API 层直接调用。

### 多租户与隔离

- Workflow ID 规范：`{tenant_id}:{form_instance_id}`，防跨租户冲突。
- 单一 task queue 起步；出现大租户噪音后按租户分级拆 queue。
- Activity 内访问数据库必须走现有 `tenantctx` 注入路径，RLS 语义不变。

### 版本化与迁移

- Workflow 代码使用 Temporal `GetVersion` API 做版本分叉，禁止直接修改运行中 workflow 的历史决定性逻辑。
- 存量进行中的审批：使用 `tenantctl temporal-backfill-form-workflows --tenant-id <tenant-id> --dry-run` 先检查候选单据，再执行 `tenantctl temporal-backfill-form-workflows --tenant-id <tenant-id>` 补齐 workflow execution。API 层不提供回退兼容。

### 审计与观测

- 每个 signal / 状态迁移在 activity 中写现有 `audit_logs`（沿用 `workflow.form.*` action 风格）。
- Activity 内使用现有全局 tracer provider 手动创建 span。

### 首个迁移对象

请假审批（最简单、量最大）先行验证，再迁转正/异动/离职。

## 实施说明（第一批：表单审批）

- 新增 `internal/platform/temporal` 封装 client、workflow start/signal adapter 和 worker 注册；worker 随 API 进程启动，连接失败时 API 启动失败。
- 新增 `internal/workflows` 表单审批 workflow，入口使用 `workflow.GetVersion` 预留版本点；workflow 内只使用 signal、timer 和 activity，不直接访问 DB、时钟或随机数。
- `stage_definitions_json` 支持可选 `remind_after_hours`，未配置时默认 72 小时；超时未处理时 activity 写通知并记录 `workflow.form.reminder` 审计。
- approve / reject / return / withdraw signal 发送成功时同步等待查询投影更新后返回；workflow 不存在时报 `workflow_not_found`，由 backfill 工具补齐存量执行。

## 后果

- 收益：超时催办、挂起恢复、补偿、完整执行历史均由引擎原生提供，替代自建 cron + 幂等逻辑。
- 代价：Temporal server、namespace、task queue 和版本治理成为必需运维面；团队需掌握 workflow 决定性约束。
- 风险控制：迁移前必须 dry-run 并执行 backfill，避免存量在审单据审批时报 workflow missing。
