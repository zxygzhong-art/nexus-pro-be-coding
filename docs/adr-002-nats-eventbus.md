# ADR-002：NATS JetStream 事件总线（第一阶段维持 Outbox + DB 轮询）

- 状态：Accepted
- 日期：2026-07-07
- 决策人：架构评审

## 背景

架构设计文档将 NATS JetStream 定为领域事件、通知、Outbox 和异步 Agent 任务的事件总线。当前实现为事务性发件箱模式：`outbox_events` 表 + `internal/jobs/outbox_dispatcher.go` DB 轮询 worker，且仅注册了 OpenFGA 关系 tuple 同步一类 handler。Compose 环境已包含 NATS 服务，但应用未接入。

## 决策

第一阶段**不接入** NATS JetStream，维持 Outbox 表 + DB 轮询。理由：当前只有一类事件、一个消费者，DB 轮询链路最短、排查最简单、无消息中间件运维成本；Outbox 表本身是正确且必要的模式（事务一致性），未来接入 JetStream 时它仍是发布源，不构成技术债。

## 触发条件（满足即启动实施）

出现**第二个真实异步消费者**。可能的候选：

1. 通知中心需要消费领域事件（员工异动 → 站内信/邮件）。
2. Agent 异步任务队列上线（Agent Run 后台执行）。
3. 报表/工作台投影需要事件驱动更新（替代现有同步写投影）。

## 接入方案（触发后执行）

- Outbox 表和写入路径**保持不变**；改造点集中在 dispatcher 消费端：从「直接路由到 handler」改为「发布到 JetStream subject」。
- Subject 命名规范：`{domain}.{resource}.{action}`，租户 ID 放消息 header（不进 subject，避免 subject 爆炸），例如 `hr.employee.updated`。
- 每个消费者使用 durable consumer + 显式 ack；消费者自行保证幂等（事件带全局唯一 event_id）。
- OpenFGA tuple 同步作为第一个迁移到 JetStream 的消费者，验证链路后再接新消费者。
- 事件 schema 在 `internal/domain` 中集中定义并版本化，禁止各模块自定义裸 JSON。

## 后果

- 现状收益：零额外运维，事务一致性已保证，故障排查只看一张表和一个 worker 日志。
- 现状代价：多消费者场景下 dispatcher 会退化为串行硬编码路由——这正是触发条件要防止的点，在第二个消费者出现前不构成实际问题。
