# nexus-pro-fe / nexus-pro-be 接口联调审计

## 当前结论

- 前端项目：`/Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-fe`，Next.js App Router，页面主要消费 `/api/platform/*` hooks，本地登录注册仍走 `/api/auth/*` mock。
- 后端项目：`/Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be`，Go 模块化单体，当前分支 `draft/2026-07-fe-backend-sync`。
- 本轮已把前端开发代理改为代理 `/api/platform/*`、`/api/workflows/*`、`/api/attendance/*` 到后端 `/v1/*`，保留 `/api/auth/*` 本地 mock，避免误伤当前登录页。
- 本轮已补齐后端 `/v1/platform/workspace/{overview,employees,organization,attendance,turnover}`，前端 workspace hooks 可以直接通过代理访问。
- 当前已调通平台读模型、workspace 读模型、组织架构上级调整、管理员权限设置、假勤制度基础编辑保存、表单设计新增/编辑/启停/软删除、操作纪录后端筛选、通知审批 review queue、批量审核、单笔核准/不通过/退回、任务/待办 CRUD、打卡写入，以及表单草稿、提交、下载、复制、删除、取消申请闭环。
- Agent/AI 对话按本轮边界先不做，后续应由独立 Python 服务承接，Go 后端只保留平台投影或转发边界。

## 前端页面与入口功能

| 路由 | 页面文件 | 主要按钮 / 交互 | 当前数据源 | 后端承接状态 |
| --- | --- | --- | --- | --- |
| `/` | `app/(platform)/page.tsx` | 侧边栏导航、AI 面板、通知、查看助理、常用表单、打卡、出勤记录、地点选择 | `/api/platform/home` | 已接 `/v1/platform/home` |
| `/assistants` | `app/(platform)/assistants/page.tsx` | 助理筛选、助理卡片、返回列表、快捷 prompt、上传、发送 | `/api/platform/assistants` | 列表已接；聊天写动作缺真实 agent API |
| `/forms` | `app/(platform)/forms/page.tsx` | 表单分类、申请记录 tab、打开表单、打印、下载、存草稿、提交、确认提交、取消、复制、删除草稿 | `/api/platform/forms`、`/api/workflows/forms/*` | 已接后端并完成 smoke |
| `/tasks` | `app/(platform)/tasks/page.tsx` | 新增工时、月切换、任务编辑/删除、新增任务、待办完成/转工时/删除、打卡入口 | `/api/platform/tasks`、`/api/attendance/clock-records` | 读投影、任务/待办 CRUD、转工时、打卡已接 |
| `/workspace` | `app/(platform)/workspace/page.tsx` | 工作区 10 个子视图导航：概览、员工、组织、在职分析、工时统计、打卡时间、假勤制度、表单设计、管理员、操作纪录 | `/api/platform/workspace*`、`/api/attendance/*` | 主聚合、新增 5 个子接口、组织上级调整、管理员权限、假勤制度基础编辑保存、表单设计基础写动作和操作纪录后端筛选已接；假余额重算、规则版本化和表单发布/版本化仍待生产化 |
| `/notifications` | `app/(platform)/notifications/page.tsx` | 待处理/已处理/已知会 tab、日期/类型/状态筛选、勾选、全选、单笔不通过、单笔退回、单笔核准、批量不通过、批量退回、批量核准、分页、行详情 | `/api/workflows/reviews`、`/api/workflows/reviews/bulk-action`、`/api/workflows/forms/{id}/{action}` | 已接 `/v1/workflows/reviews`、批量审核和单笔审批接口 |
| `/insights` | `app/(platform)/insights/page.tsx` | 报表 tab、月份前后切换、成员详情弹窗、AI 面板 | `/api/platform/insights` | 已接 `/v1/platform/insights`，销售/财务仍是投影/占位 |
| `/login` | `app/(auth)/login/page.tsx` | 邮箱密码登录、Google/Microsoft SSO stub、注册链接、隐私/条款 | `/api/auth/login` mock | 后端保持 OIDC bound-account；不新增公共密码登录 |
| `/register` | `app/(auth)/register/page.tsx` | 注册表单、提交、返回登录、隐私/条款 | `/api/auth/register` mock | 按产品约束不允许开放注册，暂不接后端 |
| `/privacy` | `app/(auth)/privacy/page.tsx` | 返回登录、关闭、隐私/条款互链、mailto | 静态 | 不需要后端 |
| `/terms` | `app/(auth)/terms/page.tsx` | 返回登录、关闭、隐私/条款互链、mailto | 静态 | 不需要后端 |
| `/design-system` | `app/design-system/page.tsx` | 组件展示、抽屉、消息、弹窗、确认、popover 等示例按钮 | 静态/示例 | 开发文档页，不接业务后端 |
| `/insight-example` | `app/insight-example/page.tsx` | 示例月份切换、示例图表 tab | 静态/示例 | 示例页，不接业务后端 |

## workspace 子功能拆解

| 子视图 | 主要按钮 / 交互 | 已有后端接口 | 缺口 |
| --- | --- | --- | --- |
| 概览 | 跳转工时统计、待办详情 | `/v1/platform/workspace/overview` | 写动作无 |
| 员工管理 | 员工预览、搜索/筛选、CSV 下载 | `/v1/platform/workspace/employees`，HR CRUD 在 `/v1/hr/*` | 前端页面目前只读，若要编辑需切 HR API |
| 组织架构 | 放大、缩小、下载 PNG、上级选择 popover、主管状态展示 | `/v1/platform/workspace/organization`、`PATCH /v1/platform/workspace/organization/employees/{id}/manager` | 上级调整已持久化；主管状态由下属上级关系自动计算 |
| 在职分析 | 月份/年份切换、CSV 下载 | `/v1/platform/workspace/turnover` | 目前是 HR 投影，长期可做月度 projection |
| 工时统计 | 月份切换、CSV 导出、异常详情 | `/v1/platform/workspace/attendance` | 只读统计；异常处理需接考勤补卡 workflow |
| 打卡时间 | 月份切换、导出打卡/异常 CSV | `/v1/platform/workspace/attendance` | 同上 |
| 假勤制度 | 假别/配额/工时编辑保存 | `/v1/platform/workspace` 聚合 `leavePolicy`；`PATCH /v1/attendance/policies/current` | 基础政策保存已接；假余额生效、年假生成、历史规则版本和规则生效日期仍待生产化 |
| 表单设计 | 新增、预览、更多操作、编辑基础信息、启用/停用、流程节点编辑 | `/v1/platform/workspace` 聚合 `formDesign`；`POST /v1/platform/workspace/forms`；`PATCH /v1/platform/workspace/forms/{id}`；`DELETE /v1/platform/workspace/forms/{id}` | 基础新增/编辑/启停/软删除已接 `form_templates.schema.workspace_design`；发布、版本化和多节点 workflow step 表仍待生产化 |
| 管理员设置 | 搜索候选人、权限矩阵勾选、新增/编辑/批量删除管理员 | `/v1/platform/workspace` 聚合 `adminSettings`；`POST /v1/platform/workspace/admins`；`PATCH /v1/platform/workspace/admins/{id}/permissions`；`DELETE /v1/platform/workspace/admins/{id}` | 已接 IAM permission set/assignment；旧系统直授管理员仍建议回 IAM 专页治理 |
| 操作纪录 | 类型/操作者/日期筛选、搜索 | `/v1/platform/workspace` 聚合 `auditLogs`；`GET /v1/platform/workspace/audit-logs` | 后端筛选已接，支持 operator/type/from/to/keyword/page |

## 已补齐 / 已调通的接口

前端开发环境现在按以下映射代理：

```text
/api/platform/:path*    ->  ${NEXUS_API_PROXY_TARGET:-http://localhost:8080}/v1/platform/:path*
/api/workflows/:path*  ->  ${NEXUS_API_PROXY_TARGET:-http://localhost:8080}/v1/workflows/:path*
/api/attendance/:path* ->  ${NEXUS_API_PROXY_TARGET:-http://localhost:8080}/v1/attendance/:path*
```

后端本轮新增或确认可用：

- `GET /v1/platform/home`
- `GET /v1/platform/assistants`
- `GET /v1/platform/forms`
- `GET /v1/platform/tasks`
- `GET /v1/platform/workspace`
- `GET /v1/platform/workspace/overview`
- `GET /v1/platform/workspace/employees`
- `GET /v1/platform/workspace/organization`
- `PATCH /v1/platform/workspace/organization/employees/{id}/manager`
- `POST /v1/platform/workspace/admins`
- `PATCH /v1/platform/workspace/admins/{id}/permissions`
- `DELETE /v1/platform/workspace/admins/{id}`
- `POST /v1/platform/workspace/forms`
- `PATCH /v1/platform/workspace/forms/{id}`
- `DELETE /v1/platform/workspace/forms/{id}`
- `GET /v1/platform/workspace/audit-logs?operator_id=&type=&from=&to=&keyword=`
- `GET /v1/platform/workspace/attendance?year=&month=`
- `GET /v1/platform/workspace/turnover?year=&month=`
- `GET /v1/platform/insights?month=`
- `PATCH /v1/attendance/policies/current`
- `POST /v1/platform/tasks/items`
- `PATCH /v1/platform/tasks/items/{id}`
- `DELETE /v1/platform/tasks/items/{id}`
- `POST /v1/platform/tasks/todos`
- `PATCH /v1/platform/tasks/todos/{id}`
- `DELETE /v1/platform/tasks/todos/{id}`
- `POST /v1/platform/tasks/todos/{id}/convert`
- `GET /v1/workflows/reviews`
- `POST /v1/workflows/reviews/bulk-action`
- `POST /v1/workflows/forms/{id}/approve`
- `POST /v1/workflows/forms/{id}/reject`
- `POST /v1/workflows/forms/{id}/return`
- `POST /v1/workflows/forms/drafts`
- `PATCH /v1/workflows/forms/{id}`
- `DELETE /v1/workflows/forms/{id}`
- `GET /v1/workflows/forms/{id}/export`
- `POST /v1/workflows/forms/{id}/submit`
- `POST /v1/workflows/forms/{template_key}/submit`
- `POST /v1/workflows/forms/{id}/cancel`
- `POST /v1/workflows/forms/{id}/duplicate`
- `POST /v1/attendance/clock-records`

## 后端仍缺的功能

1. 通知审批页
   - Review queue、批量核准/退回/不通过、单笔核准/不通过/退回已经接到后端。
   - 下一步仍需补单笔详情抽屉、评论/附件、已读/通知 read 状态，以及真正的多节点审批任务表。

2. 表单设计生产化
   - 基础新增、编辑、启停和软删除已经接到 `form_templates.schema.workspace_design`。
   - 下一步仍需 template version、published/draft 状态和 workflow step 定义，才能支持正式发布、回滚、灰度和多节点审批。

3. 任务与待办深水区
   - `/tasks` 的工时记录、待办 CRUD 和转工时已接后端。
   - 后续仍需明确它与正式工时、项目、成本中心、审批补卡之间的边界，避免长期停留在轻量个人记录模型。

4. AI 助理聊天
   - 助理列表和初始消息已可投影。
   - 上传、发送、会话历史、消息状态先不放入 Go 单体；按当前边界应由独立 Python agent 服务提供，Go 后端再按需要做账号、租户、审计和转发。

5. 工作区后台写动作
   - 组织上级调整、管理员权限分配/撤销/权限变更、假勤制度基础编辑保存、表单设计基础写动作和审计筛选已经接到后端；假勤制度余额生效/规则版本化与表单发布/版本化仍未完整接入。

6. insights 财务/销售数据
   - 当前后端能返回 HR/attendance/agent 投影，但销售与财务没有真实业务库表。
   - 需要单独业务域或外部数据同步表后，报表才不只是演示投影。

7. 登录/注册
   - 前端仍有 email/password login/register mock。
   - 后端产品约束是 OIDC + bound account，不应直接新增公共注册；前端应改成 OIDC 登录入口或保留 demo-only mock。

## 建议库表设计

### 1. 审批流

```sql
workflow_template_versions(
  id, tenant_id, template_id, version, status,
  schema_json, published_at, published_by, created_at
)

workflow_steps(
  id, tenant_id, template_version_id, step_key, step_type,
  name, order_index, assignee_rule_json, condition_json, created_at
)

workflow_tasks(
  id, tenant_id, instance_id, step_id, assignee_account_id,
  status, due_at, claimed_at, completed_at, created_at
)

workflow_task_actions(
  id, tenant_id, task_id, actor_account_id, action,
  comment, payload_json, created_at
)

workflow_notifications(
  id, tenant_id, instance_id, recipient_account_id,
  notification_type, read_at, created_at
)

workflow_events(
  id, tenant_id, instance_id, event_type,
  actor_account_id, payload_json, created_at
)
```

### 2. 任务 / 工时

```sql
task_records(
  id, tenant_id, account_id, employee_id, work_date,
  total_hours, source, created_at, updated_at
)

task_items(
  id, tenant_id, task_record_id, title, category,
  product, hours, note, created_at, updated_at
)

task_todos(
  id, tenant_id, account_id, text, due_date,
  status, converted_task_item_id, created_at, updated_at
)
```

### 3. AI 助理对话

```sql
assistant_conversations(
  id, tenant_id, assistant_id, account_id,
  title, created_at, updated_at
)

assistant_messages(
  id, tenant_id, conversation_id, role,
  content, status, created_at
)

assistant_attachments(
  id, tenant_id, conversation_id, message_id,
  object_key, file_name, mime_type, size_bytes, created_at
)
```

### 4. 员工外部同步

```sql
employee_source_mappings(
  id, tenant_id, source, external_id, employee_id,
  source_hash, last_payload, last_synced_at, created_at, updated_at
)
```

### 5. 报表投影

```sql
attendance_monthly_summaries(
  tenant_id, year, month, employee_id,
  work_days, leave_hours, abnormal_count, updated_at
)

workflow_pending_counters(
  tenant_id, account_id, pending_count,
  reviewed_count, notified_count, updated_at
)
```

销售/财务报表如要生产化，应独立建业务事实表，例如 `sales_opportunities`、`sales_orders`、`finance_entries`，不要长期塞进 platform mock payload。

## 建议接口设计

已接的表单、任务基础写动作、组织上级调整、管理员设置、表单设计基础写动作和操作纪录筛选不再列为缺口。短期优先补前端还存在但没有生产级 source-of-truth 的动作：

- `POST /v1/workflows/templates/{id}:publish`
- `POST /v1/workflows/templates/{id}:version`
- Python agent service: `POST /agent/conversations`、`POST /agent/conversations/{id}/messages`、`POST /agent/conversations/{id}/attachments`

## 推荐推进顺序

1. 保持当前已打通的 platform/workflows/attendance 代理和页面 smoke，把本轮变更拆成可 review 的本地提交。
2. 补 `/notifications` 的详情抽屉、评论/附件、已读和真正 `workflow_tasks/events/notifications` 表。
3. 补 `/workspace` 生产化深水区：假勤制度余额生效/规则版本化、表单设计发布/版本化。
4. 按独立 Python 服务设计 Agent/AI 对话，不混进本轮 Go 单体。
5. 最后生产化 sales/finance insights，接真实业务域或外部数仓同步。
