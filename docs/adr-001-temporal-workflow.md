# ADR-001：Temporal 长流程审批接入方案（设计先行，暂不实现）

- 状态：Accepted（设计归档，等待触发条件）
- 日期：2026-07-07
- 决策人：架构评审

## 背景

架构设计文档（技术架构 / 技术选型对比）将 Temporal 定为审批、入职、转正、异动、离职等长流程的工作流引擎。当前实现为同步状态表流转（`form_instances` + `workflow_runs` + `workflow_service.go`），没有定时器、催办、超时升级、补偿逻辑。Compose 环境已包含 Temporal 服务，但 Go 代码未引入 SDK。

## 决策

第一阶段**不接入** Temporal，维持纯状态机审批。本 ADR 归档接入方案与触发条件，待条件满足后按方案实施。

## 触发条件（满足任意一条即启动实施）

1. 出现第一个需要**超时自动升级**或**定时催办**的审批场景（例如 N 天未审批自动转上级）。
2. 出现需要**长时间挂起/恢复**的流程（例如入职流程等待背调回执、合同续签等待窗口期）。
3. 出现多级串并联会签且需要**失败补偿**（例如审批通过后下游步骤失败需回滚已生效动作）。

在触发前，禁止以「架构完整性」为由提前接入。

## 接入方案（触发后执行）

### 架构位置

- Go 模块化单体内新增 `internal/platform/temporal`（client 封装）与 `internal/workflows`（workflow/activity 定义）。
- Worker 以同进程 goroutine 启动（`cmd/api` bootstrap 中按 `TEMPORAL_ENABLED` 开关），流量增长后可平移为独立部署单元，不改代码结构。

### 执行权威与读模型

- Temporal 成为流程**执行权威**；现有 `workflow_runs` / `form_instances` 表降级为**查询投影**（读模型），由 activity 在每个状态迁移时更新，保证现有查询 API 与前端不变。
- approve / reject / withdraw API 从直接改表变为向 workflow 发 signal；同步等待用 update-with-start 或 query 保持接口响应语义。

### 多租户与隔离

- Workflow ID 规范：`{tenant_id}:{form_instance_id}`，防跨租户冲突。
- 单一 task queue 起步；出现大租户噪音后按租户分级拆 queue。
- Activity 内访问数据库必须走现有 `tenantctx` 注入路径，RLS 语义不变。

### 版本化与迁移

- Workflow 代码使用 Temporal `GetVersion` API 做版本分叉，禁止直接修改运行中 workflow 的历史决定性逻辑。
- 存量进行中的审批：旧状态机路径保留双轨运行，新提交走 Temporal，存量单据自然消化后下线旧路径。

### 审计与观测

- 每个 signal / 状态迁移在 activity 中写现有 `audit_logs`（沿用 `workflow.form.*` action 风格）。
- Temporal SDK 的 OTel interceptor 接入现有 tracing，保证一次审批可串起 API → workflow → activity → DB。

### 首个迁移对象

请假审批（最简单、量最大）先行验证，再迁转正/异动/离职。

## 后果

- 收益（触发后）：超时升级、催办、挂起恢复、补偿、完整执行历史均由引擎原生提供，替代自建 cron + 幂等逻辑。
- 代价：新增一套运维面（Temporal server、namespace、版本治理）；团队需掌握 workflow 决定性约束。
