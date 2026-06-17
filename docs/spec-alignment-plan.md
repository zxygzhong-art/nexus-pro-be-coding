# Notion 需求补齐计划

本计划覆盖三份 Notion 需求文档：系统架构设计、系统权限设计、Feature 员工管理。目标不是一次性做完整 AI/权限/HR 平台，而是把当前 Go 后端补到“需求可追踪、边界不误导、关键行为不违约”的状态。

## 当前判断

| 需求域 | 当前状态 | 本轮补齐口径 |
| --- | --- | --- |
| 系统架构设计 | Go 模块化单体、PostgreSQL、RLS、审计和基础 Agent run 已存在；AI Agent 文档里的 company/user/workspace/agent/knowledge/file/plan/license 数据模型缺失 | 先补 schema、domain placeholder 和 schema guard test，运行时接入留到后续阶段 |
| 系统权限设计 | 已有 PermissionSet、UserGroup、DataScope、FieldPolicy、AssumableRole、relationship tuple、outbox、RLS 和审计；Keycloak/OpenFGA 仍是可替换/待同步边界 | 保持权限中心在同一个 Go 服务内，先不拆独立服务；后续补审批流和 OpenFGA tuple 同步闭环 |
| Feature 员工管理 | 员工 CRUD、导入、编号、状态流转、字段策略、导出和基础 API 已具备；创建校验和导入确认仍偏宽松 | 本轮优先把导入确认改成默认 all-or-nothing，并保留六段资料的服务端校验入口 |

## P0 本轮补齐

1. 架构追踪文档：新增本文件，明确哪些需求已满足、哪些只是 schema 层占位、哪些进入后续阶段。
2. AI Agent 架构 schema：新增 `companies`、`users`、`roles`、`workspaces`、`workspace_users`、`agents`、`knowledges`、`files`、`file_process_tasks`、`pricing_plans`、`company_plans`、`licenses` 等表，保持与现有 HR tenant 模型并存。
3. Domain placeholder：补充 AI Agent 架构相关领域结构体，避免后续实现继续散落匿名 map。
4. Guard test：增加 schema 对齐测试，防止架构表在后续重构中被误删。
5. 员工导入确认：确认阶段先全量复验；只要有任意行错误，就返回 `import_validation_failed`，不写入任何员工，不产生部分成功。

## P1 后续补齐

1. 权限审批闭环：把高风险权限变更、AssumeRole、员工删除/批量导入等动作接入统一审批实例，而不是只依赖 `ApprovalConfirmed` header。
2. OpenFGA 同步闭环：将 relationship tuple / outbox 的落库、投递、重试、幂等和状态回写做成可观测链路。
3. 员工创建强校验：基于前端六段资料表单确定必填矩阵，再把本地/外籍、任用类别、保险、教育兵役、内部经历等规则落到统一 validator。
4. API contract：补齐 OpenAPI 的必填字段、错误码和导入确认原子性说明。

## P2 后续补齐

1. AI runtime：接入 Python/LangGraph、任务队列、文件处理、向量库和 Agent knowledge 权限检查。
2. 套餐/license 生效：把 pricing plan、company plan、license 与调用额度、存储额度、用户数限制挂钩。
3. Company/tenant 统一：明确当前 HR `tenant_id` 与 AI Agent `company_id` 的映射关系，再收敛认证上下文和 RLS session 变量。

## 验证策略

本轮先跑最小闭环：

```bash
GOCACHE=$PWD/.gocache go test ./tests/unit/...
```

如 schema/migration 变更影响迁移解析，再补跑：

```bash
make migrate-validate
```
