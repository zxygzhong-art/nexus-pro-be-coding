# ADR-002：NATS JetStream 事件总线（Outbox 发布 + durable consumer）

- 状态：Implemented（发布端 + OpenFGA consumer）
- 日期：2026-07-07
- 决策人：架构评审

## 背景

架构设计文档将 NATS JetStream 定为领域事件、通知、Outbox 和异步 Agent 任务的事件总线。任务 11 之前实现为事务性发件箱模式：`outbox_events` 表 + `internal/jobs/outbox_dispatcher.go` DB 轮询 worker，且仅注册了 OpenFGA 关系 tuple 同步一类 handler。Compose 环境已包含 NATS 服务，任务 11 起应用按开关接入 JetStream。

## 决策

历史第一阶段先维持 Outbox 表 + DB 轮询；任务 11 触发后，按本 ADR 的接入方案落地 NATS JetStream。Outbox 表和写入路径仍保持不变，事件总线只接入 dispatcher 消费端和第一个 durable consumer。

`NATS_ENABLED=false` 是默认值，dispatcher 继续直接调用 OpenFGA tuple writer，保持既有行为。`NATS_ENABLED=true` 时，dispatcher 发布有 subject 映射的 outbox 事件到 JetStream，publish ack 成功后标记 outbox 事件 `succeeded`；无 subject 映射的事件继续保持 `pending`。

## 触发条件（满足即启动实施）

出现**第二个真实异步消费者**。可能的候选：

1. 通知中心需要消费领域事件（员工异动 → 站内信/邮件）。
2. Agent 异步任务队列上线（Agent Run 后台执行）。
3. 报表/工作台投影需要事件驱动更新（替代现有同步写投影）。

## 接入方案（触发后执行）

- Outbox 表和写入路径**保持不变**；改造点集中在 dispatcher 消费端：从「直接路由到 handler」改为「发布到 JetStream subject」。
- Subject 命名规范：`events.{domain}.{resource}.{action}`，租户 ID 放消息 header（不进 subject，避免 subject 爆炸），例如 `events.iam.relationship.write`。
- 每个消费者使用 durable consumer + 显式 ack；消费者自行保证幂等（事件带全局唯一 event_id）。
- OpenFGA tuple 同步作为第一个迁移到 JetStream 的消费者，验证链路后再接新消费者。
- 事件 schema 在 `internal/domain` 中集中定义并版本化，禁止各模块自定义裸 JSON。

## 实施说明

- 平台层封装：`internal/platform/natsbus` 负责 NATS connect、`NEXUS_EVENTS` stream ensure、publish ack、durable consumer subscription。stream subjects 为 `events.>`。
- 消息信封：`internal/domain.DomainEventEnvelope` 是标准 JSON body，字段包括 `event_id`、`event_type`、`tenant_id`、`occurred_at`、`schema_version`、`payload`，当前版本 `schema_version=1`。
- Headers：`Nexus-Tenant-Id`、`Nexus-Event-Id`、`Nexus-Event-Type`。租户 ID 不进入 subject。
- 已映射事件：`openfga.relationship.write -> events.iam.relationship.write`，`openfga.relationship.delete -> events.iam.relationship.delete`。未映射事件保持 outbox `pending`。
- OpenFGA consumer：durable 名称默认 `nexus-openfga`，filter subject 为 `events.iam.relationship.*`，显式 ack，`MaxDeliver=5`。
- 幂等策略：consumer 用 `Nexus-Event-Id` 做进程内去重；OpenFGA tuple writer 对重复写入和删除缺失 tuple 的重放冲突做容错。
- 失败策略：处理失败执行 `nak` 触发 JetStream 重投；达到 `MaxDeliver` 的失败记录结构化 error 日志，并写入审计事件 `platform.event.dead_letter`。当前不引入独立 dead-letter stream，后续可用 JetStream advisory 或独立 DLQ stream 扩展。
- 可观测：publish / consume 路径接入 OpenTelemetry span，并记录 `tenant_id`、`event_id`、`event_type`、subject/stream 等属性；日志沿用 `slog` 结构化字段。

## 后果

- 收益：Outbox 继续保证事务一致性；发布端和消费者解耦，后续新增通知、投影、Agent 任务等消费者不需要继续扩展 dispatcher 硬编码 handler。
- 代价：启用 NATS 后增加 JetStream 运行依赖；发布成功但消费失败时需要同时查看 outbox 状态、JetStream consumer 状态、OpenFGA consumer 日志和 `platform.event.dead_letter` 审计。
