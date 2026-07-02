# 当前项目全功能测试计划

编写基准: 2026-07-01，依据当前源码目录 `app/`、`hooks/api/`、`app/api/`、`store/` 与现有 Vitest 测试整理。

本计划用于手工测试、补齐 MSW/API mock、以及后续自动化用例拆分。页面文字以当前前端 UI 为准，保留繁中标签。

## 1. 测试范围总览

### 1.1 当前页面

| 页面 | 路径 | 类型 | 主要测试重点 |
|---|---|---|---|
| 登入 | `/login` | Auth | 表单验证、demo 帐号、cookie、跳转、SSO stub |
| 注册 | `/register` | Auth | 注册字段验证、密码强度、条款勾选、跳转 |
| 隐私权政策 | `/privacy` | Legal | 内容渲染、返回登入、页脚链接、mailto |
| 使用条款 | `/terms` | Legal | 内容渲染、返回登入、页脚链接、内部隐私链接 |
| 平台首页 | `/` | Platform | Hero、导航、AI 入口、常用助理、常用表单、打卡卡片 |
| AI 助理 | `/assistants` | Platform | 助理筛选/搜索、列表空态、进入聊天、聊天侧栏 |
| 工作任务 | `/tasks` | Platform | 月份切换、任务新增/编辑/删除、待办、打卡卡片 |
| 表单申请 | `/forms` | Platform | 表单搜索、已申请/草稿筛选、表单 modal、草稿/送出动作 |
| 待办审核 | `/notifications` | Platform | 审核队列、筛选、分页、单笔/批次审核 |
| 洞察报表 | `/insights` | Platform | 报表 tab、月份切换、图表、表格排序/分页、成员明细 |
| 工作区设置 | `/workspace` | Platform | 概览、员工、组织、在职分析、工时、打卡、假勤、管理员、审计、表单设计 |
| 元件对照页 | `/design-system` | Showcase | Anchor、收合、AntD 元件状态、反馈组件 |
| 洞察示例页 | `/insight-example` | Showcase | 静态报表 tab、月份切换、表格/图表 |

### 1.2 当前本地 Next mock API

这些接口在 `app/api/` 中有本地 route，可以直接用 `pnpm dev` 测试展示流:

| 方法 | API | 覆盖页面 |
|---|---|---|
| POST | `/api/auth/login` | `/login` |
| POST | `/api/auth/register` | `/register` |
| GET | `/api/platform/home` | `/` |
| GET | `/api/platform/assistants?tag=&search=` | `/assistants` |
| GET | `/api/platform/forms` | `/forms` |
| GET | `/api/platform/insights?month=YYYY-MM` | `/insights` |
| GET | `/api/platform/tasks` | `/tasks` |
| GET | `/api/platform/workspace/overview` | `/workspace` 概览 |
| GET | `/api/platform/workspace/employees` | `/workspace` 员工管理 |
| GET | `/api/platform/workspace/organization` | `/workspace` 组织架构 |
| GET | `/api/platform/workspace/attendance?year=&month=` | `/workspace` 工时/打卡 |
| GET | `/api/platform/workspace/turnover?year=&month=&annualYear=` | `/workspace` 在职分析 |
| GET | `/api/platform/workspace` | `/workspace` 假勤、管理员、审计、表单设计基础数据 |

### 1.3 当前需要真实后端或补 mock 的 mutation/API

以下 hooks 已存在，但当前 `app/api/` 没有对应本地 route。若只跑本地 Next mock，预期会 404 并触发全局错误提示，除非测试环境另有 backend 或 MSW handler。

| API | 涉及功能 |
|---|---|
| `/api/attendance/clock-records` | 首页/任务页打卡 |
| `/api/platform/tasks/items*` | 任务新增、编辑、删除 |
| `/api/platform/tasks/todos*` | 待办新增、勾选、删除、转任务 |
| `/api/workflows/forms*` | 表单送出、草稿、取消、复制、下载、审核动作 |
| `/api/workflows/reviews*` | 待办审核队列与批次审核 |
| `/api/attendance/policies/current` | 假勤制度保存 |
| `/api/platform/workspace/admins*` | 管理员新增、编辑权限、删除 |
| `/api/platform/workspace/forms*` | 工作区表单设计新增、编辑、删除、启停 |
| `/api/platform/workspace/audit-logs*` | 操作纪录服务端筛选 |
| `/api/platform/workspace/organization/employees/*/manager` | 组织上级更新 |

## 2. 前置检查

1. 使用指定版本:

```bash
nvm use
corepack enable
pnpm install
```

2. 启动本地站点:

```bash
pnpm dev
```

默认访问 `http://localhost:3000`。若 `NEXT_PUBLIC_API_BASE_URL` 未设置，axios 使用相对路径，命中当前 Next mock routes。

3. 基础质量门槛:

```bash
pnpm lint
pnpm test:run
pnpm build
```

说明: 本文档是测试计划，不代表上述命令已在本次编写时通过。实际执行时如失败，先记录失败命令、错误文件、是否阻断页面启动。

4. 浏览器视口:

| 视口 | 用途 |
|---|---|
| 1440 x 900 | 桌面主路径 |
| 1100 x 800 | 平板/右侧面板折叠临界 |
| 720 x 900 | 移动导航、单栏布局临界 |
| 360 x 780 | 小屏文字溢出、按钮换行 |

5. 通用检查:

- 控制台无红色 error。
- 404/500 请求只出现在预期的“缺本地 mock mutation”场景。
- 页面切换后滚动回顶部。
- Modal 打开/关闭后焦点、遮罩、Esc/点击关闭行为正常。
- 文本不重叠，表格可横向滚动，按钮文案不被截断。
- 所有下载动作生成文件名正确，并包含 BOM 时可用 Excel 打开。

## 3. Auth 与 Legal

### 3.1 `/login` 登入

测试数据:

| 场景 | Email | Password | 预期 |
|---|---|---|---|
| 管理员 demo | `admin@demo.local` | `Password123!` | 成功，写入 `_t` cookie，跳转 `/` |
| 员工 demo | `employee@demo.local` | `Password123!` | 成功，写入 `_t` cookie，跳转 `/` |
| 审计 demo | `audit@demo.local` | `Password123!` | 成功，写入 `_t` cookie，跳转 `/` |
| 邮箱大小写/空白 | ` Admin@Demo.Local ` | `Password123!` | API 会 trim/lowercase，预期成功 |
| 错误密码 | `admin@demo.local` | `wrong` | 401，全局错误提示 |
| 未登记邮箱 | `nobody@demo.local` | `Password123!` | 401，全局错误提示 |
| 邮箱格式错 | `abc` | 任意 | 前端提示 `請輸入正確的 Email 格式` |
| 空密码 | 合法邮箱 | 空 | 前端提示 `請輸入密碼` |

步骤:

1. 打开 `/login`。
2. 空表单直接点 `登入`，确认必填提示。
3. 输入非法 email，确认格式提示。
4. 输入 demo 成功账号，分别测试 `記住我` 勾选与不勾选。
5. 成功后确认跳转 `/`，cookie `_t` 存在。勾选 `記住我` 时 cookie 有 30 天 expires；未勾选时为 session cookie。
6. 点击 `忘記密碼？`，预期显示 `重設密碼流程待後端合約`。
7. 点击 `使用 Google 登入`、`使用 Microsoft 登入`，预期为 SSO stub info。
8. 点击 `立即註冊`、页脚 `隱私權政策`、`使用條款`，确认路由正确。

边界:

- 重复快速点击 `登入` 时按钮应 loading，避免重复提交。
- 401 即使 silentError 为 false，也应执行全局错误提示。
- 登出后再次访问 `/login`，不应残留 Authorization header。

### 3.2 `/register` 注册

测试数据:

| 字段 | 有效值 | 边界值 |
|---|---|---|
| 姓名 | `王小明` | 空值应提示 `請輸入姓名` |
| Email | `new.user@example.com` | `abc` 应提示格式错误 |
| 密码 | `Password123!` | `1234567` 应提示至少 8 位 |
| 确认密码 | 与密码相同 | 不同应提示 `兩次輸入的密碼不一致` |
| 条款 | 勾选 | 未勾选应提示 `請先同意使用條款與隱私權政策` |

步骤:

1. 打开 `/register`。
2. 空表单点 `建立帳號`，确认必填提示。
3. 输入弱密码，确认强度条与文案变化。
4. 输入不一致确认密码，确认错误提示。
5. 不勾选条款提交，确认阻断。
6. 输入有效数据并勾选条款，预期 POST `/api/auth/register` 返回 201，写入 `_t`，跳转 `/`。
7. 点击 Google/Microsoft 注册按钮，确认 SSO stub。
8. 点击 `登入`、条款/隐私链接，确认路由。

边界:

- 当前本地注册只验证 email 格式，不限制公司域名。若产品要求仅企业邮箱，需要后端或前端补充。
- 注册成功 message 文案为 `帳號已建立`，API message 为 `登入成功`，需确认是否接受。

### 3.3 `/privacy` 与 `/terms`

步骤:

1. 分别打开 `/privacy`、`/terms`。
2. 确认 logo、标题、meta 日期、章节列表、页脚链接渲染。
3. 点击左上 logo、关闭按钮、`返回登入`，均应到 `/login`。
4. 点击页脚 `/privacy`、`/terms` 互跳。
5. 点击 mailto 链接: privacy 使用 `privacy@ikala.com`，terms 使用 `legal@ikala.com`。
6. 在 360px 宽度检查 header sticky、长段落换行、列表缩进。

边界:

- 页面无 API，离线也应可打开。
- Legal 文案含链接和 strong 节点，检查不会丢失样式。

## 4. 全局平台外框

覆盖页面: `/`、`/assistants`、`/tasks`、`/forms`、`/notifications`、`/insights`，不包含 `/workspace` 内部左侧导航，也不包含展示页。

步骤:

1. 桌面打开 `/`，逐项点击侧栏: `主頁`、`洞察報表`、`AI 助理`、`工作任務`、`表單申請`、`待辦審核`、`工作區設定`。
2. 点击侧栏 collapse，确认只显示 icon，hover tooltip 可读，重新展开正常。
3. 720px 以下点击 header 菜单，确认移动侧栏出现；点击遮罩和导航项后关闭。
4. 点击 `歷史紀錄`，打开历史 modal。
5. 历史 modal 搜索:
   - `週報`: 应匹配 `幫我潤飾週報開頭` 或相关项。
   - `不存在`: 显示 `找不到符合的對話`。
   - 空搜索: 只显示最新 30 笔，并显示 `僅顯示最新 30 筆`。
6. 点击历史行，预期进入 `/assistants` full chat。
7. 点击 header `系統通知`，确认 popover 四条通知、`全部標為已讀` 与 `查看全部通知` 当前为静态按钮。
8. 点击头像菜单，确认 `個人資料`、`偏好設定`、`登出`。点击 `登出` 应清 `_t` 并跳 `/login`。
9. 点击 header `AI 問答`:
   - 首页 `/`: 预期跳 `/assistants` 并进入 full chat。
   - 非首页平台页: 打开右侧 `AI 問答` panel。
   - `/workspace`: AI 按钮隐藏。
   - `/assistants` full chat: 全局 header/AI panel 隐藏。

边界:

- 右侧 AI panel 在 1100px 以下应覆盖全宽，不压缩主内容。
- AI panel 空输入时 `送出` disabled；输入空格仍 disabled。
- AI panel Enter 发送，Shift+Enter 换行。
- AI panel 快捷提示会追加用户消息和前端示范回复。
- 上传按钮只是 UI 入口，当前无上传逻辑。

## 5. `/` 平台首页

### 5.1 Hero 与导航

步骤:

1. 打开 `/`，确认 hero 问候、日期天气、输入框渲染。
2. 点击 hero 输入框 `送出`，预期跳 `/assistants` full chat。
3. 在 hero textarea 输入 `請幫我請假{Enter}`，预期只是换行，不发送。
4. 滚动首页，确认浮动 header 背景从透明变实色，文字颜色切换。
5. 小屏确认问候文案换行，不遮挡输入框。

边界:

- 问候语按浏览器小时变化，挂载前固定 server 安全值，检查 hydration 无 mismatch。
- 空输入仍可点击 hero 送出进入聊天，这是当前实现行为。

### 5.2 常用 AI 助理

步骤:

1. 首次加载显示 Skeleton，然后渲染 12 张助理卡。
2. 横向滚动，检查左右箭头显示/隐藏逻辑。
3. 点击 `查看全部`，预期到 `/assistants`。

边界:

- 当前首页助理卡本身没有进入聊天的 onClick 逻辑，只是 button 视觉。
- 卡片长描述应两行截断。

### 5.3 常用申请表单

步骤:

1. 确认两列分类: `人事考勤類`、`財會相關`。
2. 点击表单项，当前应无实际 modal 或跳转。
3. 640px 以下确认变单列。

边界:

- 首页常用表单是静态入口展示；真正表单申请在 `/forms`。

### 5.4 每日打卡卡片

测试数据:

| 状态 | summary 输入 | 预期按钮 |
|---|---|---|
| 尚未上班 | `checkedInAt=null, checkedOutAt=null` | `上班打卡` enabled，`下班打卡` disabled |
| 已上班 | `checkedInAt=09:02, checkedOutAt=null` | 上班 disabled，`下班打卡` enabled |
| 已下班 | `checkedInAt=09:02, checkedOutAt=18:10` | 两个按钮 disabled |

步骤:

1. 确认当前时间每秒更新。
2. 点击 `出席紀錄`，打开记录 modal。
3. 在记录 modal 点击上/下月，确认 2026 年 3 月和 2026 年 4 月切换。
4. 点击 `填寫` 补卡入口，显示 `補卡表單待後端串接`。
5. 点击 `下班打卡`，打开 `打卡定位` modal。
6. 地点边界:
   - `純白咖啡`、`板橋車站`、`林本源園邸`、`河濱公園` 在 300m 内，可点。
   - `遠百大樓` 328m，按钮 disabled。
7. 有真实 `/api/attendance/clock-records` 或 MSW 时，点击可打卡地点应显示成功并更新本地状态。
8. 只有本地 Next mock 时，预期此 mutation 404，显示全局错误提示。

边界:

- 下班前没有上班时直接调用应提示 `請先完成上班打卡`。
- 后端返回 rejected 时，需映射 `duplicate`、`invalid_sequence`、`low_location_accuracy`、`outside_geofence`、`outside_time_window` 等原因。

## 6. `/assistants` AI 助理

### 6.1 助理列表

测试数据:

| 操作 | 数据 | 预期 |
|---|---|---|
| 分类全部 | `all` | 最多显示 9 笔，total 为过滤后总数 |
| 工作流 | `workflow` | 员工疑难、招聘、项目、采购、CRM 等 |
| 文件 | `doc` | 产品目录、培训、法务、行销等 |
| 分析 | `analytics` | 业绩、ESG |
| IT | `it` | 资安风控官 |
| 搜索命中 | `CRM`、`法務`、`資安` | 显示匹配卡片 |
| 搜索无结果 | `不存在的助理` | 显示 `找不到符合的助理` |
| 前后空格 | `  crm  ` | API trim 后应匹配 |

步骤:

1. 打开 `/assistants`。
2. 逐一切换 tab，确认请求参数 `tag` 与卡片数量。
3. 输入搜索关键字，确认 URL 请求 `search` 和列表刷新。
4. 点击卡片，进入 full chat。
5. 用键盘 Tab 聚焦卡片，Enter/Space 也应进入 full chat。

边界:

- API 对 title、desc、tag 做 lowercase includes，中文大小写无差异。
- route 返回 `slice(0, 9)`，当过滤总数超过 9 时 UI 目前没有分页。

### 6.2 Full chat

步骤:

1. 点击 `返回助理列表`，确认回到列表。
2. 点击 `新對話`，当前只展示按钮，无清空逻辑。
3. 点击左侧历史项，当前只关闭移动历史，不改变主对话内容。
4. 移动端点击 `對話紀錄`，侧栏出现；点遮罩关闭。
5. 点击 `編輯表單`，当前无 onClick。
6. 输入 composer，点击送出，当前 ChatComposer 无本地状态和发送逻辑，预期无新增消息。

边界:

- full chat 是静态骨架，和全局 AI panel 的“可追加消息”行为不同。
- header 内通知/头像是静态视觉按钮，不联动全局 header 菜单。

## 7. `/tasks` 工作任务

### 7.1 任务记录

测试数据:

当前任务种子在 2026 年 4 月:

| 日期 | 总时数 | 项目 |
|---|---:|---|
| 2026/04/16 | 7h | 3 项 |
| 2026/04/15 | 3h | 2 项 |
| 2026/04/14 | 4.5h | 2 项 |

步骤:

1. 打开 `/tasks`。页面用浏览器当前月份初始化。按当前日期 2026-07-01，初始会显示 2026 年 7 月与最近 7 天空记录。
2. 连续点击 `上個月` 到 2026 年 4 月，确认种子任务出现。
3. 检查 7h 显示满进度，超过 7h 时应为 over tone。
4. 点击空日期 `新增工作項目` 或页面 `新增任務`，打开 modal。
5. 新增 modal 验证:
   - `工作日期` 必填。
   - `工作內容` 必填，最多 80 字。
   - `工時` 必填，min 0.5，max 24，step 0.5。
   - `產品` 默认 Nexus。
   - `分類` 默认 一般。
   - `備註` maxLength 200。
6. 点击已有任务编辑按钮，确认 modal 回填。
7. 点击删除按钮，确认出现删除确认 modal。

边界:

- 新增/编辑/删除使用 `/api/platform/tasks/items*`，当前本地 Next mock 未实现。只有本地 mock 时预期 404。
- `buildMonthRecords` 会把当前月份最近 7 天补为空记录，非当前月份只显示 API 数据。
- 月份可无限前后切换，需要确认未来月份空态和按钮不会异常。

### 7.2 右侧打卡与待办

测试数据:

| 待办 | 状态 |
|---|---|
| 完成 Nexus OA 表單流程設計 | done |
| 準備週五 sprint demo | todo |
| 回覆 Sarah 的設計反饋 | todo |
| 更新 API 文件 | todo |
| 整理用戶訪談筆記 | done |

步骤:

1. 桌面右侧确认 `每日打卡` 和 `待辦事項`。
2. 720px 以下切换 `打卡 · 待辦` 与 `任務紀錄` mobile tabs。
3. 点击待办 checkbox，预期调用更新 API。
4. hover 未完成待办，点击 `轉成任務` 与删除按钮。
5. 在新增待办输入框:
   - 空白 Enter，无动作。
   - 输入 `補測任務頁` 后 Enter，预期新增。

边界:

- 待办 mutation 使用 `/api/platform/tasks/todos*`，当前本地 Next mock 未实现。
- 已完成待办的 `轉成任務` disabled。
- busy 状态时按钮 disabled，避免重复提交。

## 8. `/forms` 表单申请

### 8.1 表单列表

步骤:

1. 打开 `/forms`，确认 `表單列表` 默认 active。
2. 搜索:
   - `請假`: 应只显示请假相关表单。
   - `Cloud`: 应匹配销售相关 Cloud 表单。
   - `不存在`: 显示 `找不到符合的表單`。
   - 前后空格和大小写应被 trim/lowercase。
3. 点击任一表单打开 FormModal。
4. 检查所有分类都有卡片: 人事考勤、人资、财会、采购、销售、合约、行政、资产、MIS、授信、NS、其他。

边界:

- `行政相關` 中 `出差採購單` 当前有重复 id `travel-purchase`，测试时要确认 React key/打开 modal 是否异常。
- 只有 `leave-request`、`field-leave`、`leave-cancel` 走请假专用表单，其余走通用表单。

### 8.2 请假类 FormModal

有效测试数据:

| 字段 | 值 |
|---|---|
| 代理人 | `王思怡 · Siyi Wang (產品開發部)` |
| 代理人员工编号 | 自动带 `IKL011` |
| 假勤名称 | `特休假` |
| 开始时间 | `2026/07/06 09:00` |
| 结束时间 | `2026/07/06 18:00` |
| 请假原因 | `家庭事务处理` |

步骤:

1. 打开 `請假申請單`。
2. 不填直接点 `送出申請`，确认代理人、假勤名称、开始、结束、原因错误提示。
3. 选择代理人，确认员工编号自动带入。
4. 只选开始时间，确认结束时间自动补开始 + 8 小时。
5. 设置结束时间早于开始，确认 `結束時間必須晚於開始時間`。
6. 设置同日 4 小时，确认 `請假時數` 为 4。
7. 设置跨两天，确认按自然日数乘 8，例如 2026/07/06 09:00 到 2026/07/07 09:00 为 16。
8. 原因输入 101 字，确认最多 100 字错误。
9. 上传附件:
   - 选择 1 到 5 个文件，预期只留本地列表，不上传。
   - 第 6 个文件受 `maxCount=5` 限制。
   - UI 写 `單檔上限 20MB`，但当前没有 size 校验，需要作为缺口记录。
10. 点击 `儲存草稿`，有后端时保存并刷新 `/api/platform/forms`。只有本地 mock 时预期 404。
11. 点击 `送出申請`，进入确认态，字段 disabled。
12. 点击 `返回修改`，回到编辑态。
13. 点击 `確定送出`，有后端时显示成功 overlay 并 1.6 秒后关闭；只有本地 mock 时预期 404。

边界:

- `下載` 在 create 模式点击，应提示 `請先儲存或送出表單後再下載`。
- `列印` 调用 `window.print()`。
- 关闭 modal 后重新打开，应重置 step 和 field errors。

### 8.3 通用 FormModal

有效测试数据:

| 字段 | 值 |
|---|---|
| 申请主旨 | `采购笔电配件` |
| 需求日期 | `2026/07/15` |
| 申请说明 | `需要采购 USB-C hub 两个，供项目会议使用。` |

步骤:

1. 打开非请假表单，例如 `採購單`。
2. 空提交，确认主旨、需求日期、说明必填。
3. 说明输入 201 字，确认最多 200 字错误。
4. 保存草稿、送出申请、确认送出流程同请假类。

### 8.4 已申请

测试数据:

`formApplications` 共 24 笔，日期 2026/06/01 到 2026/06/24，状态循环 `reviewing`、`approved`、`rejected`、`cancelled`。

步骤:

1. 切换 `已申請`，确认 badge 数量为 24。
2. 日期筛选:
   - 全部: 24。
   - 最近 7 天: 以最新表单日 2026/06/24 为参考，应包含 2026/06/18 到 2026/06/24。
   - 最近 30 天: 包含全部 24。
   - 本月: 包含全部 6 月数据。
   - 上个月: 当前种子应为空。
3. 状态筛选 `審核中`、`已核准`、`已退回`、`已取消`。
4. 点击行打开 view modal，确认摘要、编号、时间、payload 回填。
5. view modal 点击 `取消申請`，出现确认框；确认后需要后端。
6. 点击 `複製表單`，需要后端。
7. 点击 `下載`，需要 `/api/workflows/forms/{id}/export`。

### 8.5 草稿区

测试数据:

当前只有 `draft-1`，标题 `請假申請單`，更新时间 `2026/06/24 11:42`。

步骤:

1. 切换 `草稿區`，确认 badge 为 1。
2. 日期筛选同已申请。
3. 点击草稿行打开 draft modal，确认 payload 可回填，若 payload 为空则按默认值。
4. 行 selection checkbox 不应触发行打开。
5. 选中草稿后出现批次栏。
6. 点击 `取消` 清空选择。
7. 点击 `批次刪除`，需要后端。
8. 打开草稿 modal 后点击 `刪除`，需要后端。

边界:

- 批次删除多个草稿时使用 Promise.all，若部分失败当前只走 catch，全局错误提示。

## 9. `/notifications` 待办审核

前置: 当前 `useGetWorkflowReviewQueue` 请求 `/api/workflows/reviews`，本地 Next mock 未实现。要完整测此页，需要接真实后端或补 MSW/route mock。否则页面会显示错误通知并可能空数据。

测试数据建议:

| 分组 | 数量 | 示例 |
|---|---:|---|
| 待处理 | 5 | 请假、费用、加班、出差、通用签呈 |
| 已处理 | 6 | 已核准、已退回、已取消 |
| 已知会 | 4 | 出差、费用、请假、采购 |

步骤:

1. 打开 `/notifications`。
2. 确认 tabs: `待處理`、`已處理`、`已知會`，badge 数量正确。
3. 待处理:
   - 日期筛选: 全部、最近 7 天、最近 30 天、本月、上个月。
   - 类型筛选: 全部、各表单标题。
   - 点击行，选中行高亮。
   - 单笔点击 `不通過`、`退回`、`核准`。
   - `不通過` 与 `退回` 需确认框和原因 textarea，maxLength 120。
   - `核准` 需确认框但无需原因。
4. 批次:
   - 勾选单行。
   - 点击表头全选当前页。
   - 再点表头取消当前页。
   - 有选择时显示批次栏。
   - 批次不通过/退回/核准提交后清空选择并 mutate。
5. 已处理:
   - 日期筛选。
   - 状态筛选: 全部、已核准、已退回/已取消。
   - 分页每页 10。
6. 已知会:
   - 日期筛选。
   - 分页每页 10。

边界:

- `filterByDateRange` 只解析 `YYYY/M/D` 开头时间。`今天`、`昨天` 这种相对时间无法解析，会保留在任何日期筛选结果中。
- 批次接口可能返回部分失败，UI 应显示 `已處理 X 件，Y 件失敗`。
- 当前页面没有右侧详情 pane，点击行只做 row highlight。

## 10. `/insights` 洞察报表

测试数据:

默认 month 为 `2026-04`。GET `/api/platform/insights?month=2026-04` 会生成三类报表:

- `deptTasks`: 部门工时。
- `sales`: 业务绩效。
- `finance`: 财务统计。

步骤:

1. 打开 `/insights`，默认 `部門工時`。
2. 点击月切换:
   - 上个月: 2026 年 3 月。
   - 下个月: 回 2026 年 4 月，再到 2026 年 5 月。
   - 确认图表 active 月份和表格数据随 month 变化。
3. 切换 `業務績效`，确认 KPI、近 6 个月趋势、洽谈中厂商表格。
4. 切换 `財務統計`，确认收入/支出双柱图、部门收支明细。
5. 部门工时:
   - 表格点击 `本月工時` 排序。
   - 点击 `請假` 排序。
   - 点击成员 `檢視`，打开任务明细 modal。
   - Modal 中确认总工时、任务数、参与产品、请假四个 stats。
6. 表格分页:
   - 部门成员明细默认 pageSize 10，确认 `共 N 人`。
   - 其他报表确认横向滚动。

边界:

- `getMonthLabel` 对非法 month 会回显原字符串；正常 UI 只会用 `YYYY-MM`。
- 图表为 CSS/DOM 图，需检查 bar 高度不为 0、legend 和 label 不重叠。
- API schema drift 时 dev 环境会弹 schema notification，但不会阻断数据渲染。

## 11. `/workspace` 工作区设置

### 11.1 导航与概览

步骤:

1. 打开 `/workspace`，默认 `概覽`。
2. 点击左侧所有 nav:
   - 概览、员工管理、组织架构、在职分析、工时统计、打卡时间、假勤制度、表单设计、管理员设置、操作纪录。
3. 点击 `返回主頁` 回 `/`。
4. 720px 以下确认 workspace nav 与平台移动侧栏事件不会互相卡住。
5. 概览:
   - 确认人力 stats。
   - `今日出勤狀況` 点击 `查看詳細`，应切换到 `工時統計`。
   - 点击每个待办类别，打开人员 modal。
   - Modal 表头日期标题根据类别变更，如 `預計到職日`、`試用期屆滿日`。

边界:

- 概览按系统当前日期生成月数据。当前日期 2026-07-01 时，若 mock 用 new Date，会显示 2026 年 7 月。
- 待办人数为 people.length，空数组应显示 0 且 modal 空表。

### 11.2 员工管理

步骤:

1. 点击 `員工管理`。
2. 确认 stats: 在职人数、本月入职、本月离职、试用期人数。
3. 表格检查员工编号、姓名、部门/职称、类别、电话、状态、到职时间。
4. 点击行或预览 icon，打开员工资料 modal。
5. 点击 `下載 csv`，确认文件名 `員工列表_YYYY-MM-DD.csv`，包含 BOM 与表头。
6. 测试筛选控件:
   - 部门: `產品開發部`。
   - 类别: `全職`。
   - 状态: `在職`。
   - 搜索: 姓名、Email、员工编号。

当前实现限制:

- 部门/类别/状态/search 目前只更新控件状态，没有应用到表格数据过滤。测试时应记录为当前行为或缺陷。
- `STATUS_OPTIONS` 包含 `試用期`、`留職停薪`、`離職`、`已退休`，但 schema 状态只有 `在職`、`待加入`、`已停用`，需确认后端合同。

边界:

- 到职日期晚于今天时 tenure 显示 `尚未到職`。
- CSV 字段中的双引号要被转义。

### 11.3 组织架构

步骤:

1. 点击 `組織架構`。
2. `組織列表`:
   - 排序层级、员工编号、部门、上级、主管。
   - 不完整行应有 `is-incomplete` 样式。
   - 点击上级按钮打开 picker。
   - 搜索姓名/员工编号/部门。
   - 选择 `無上級` 或某位候选人。
3. `樹狀圖預覽`:
   - 点击 `+`、`-`，缩放范围 50% 到 200%。
   - 鼠标拖拽平移。
   - Ctrl/Meta + wheel 缩放。
   - 点击 `下載 PNG`，确认文件名 `組織架構_YYYY-MM-DD.png`。

边界:

- 上级候选人必须排除自己和所有下属，避免循环。
- 若 parentId 找不到，显示 `上級資料不存在`。
- 更新上级使用 `/api/platform/workspace/organization/employees/{id}/manager`，当前本地 Next mock 未实现。

### 11.4 在职分析

步骤:

1. 点击 `在職分析`。
2. `月檢視` 默认 2026 年 5 月:
   - 检查 KPI stats、部门新进比较、部门离职率比较、明细表。
   - 点击前后月，数据随 `year/month` 变化。
   - 点击 `匯出 CSV`，确认当月非未来时 enabled。
3. `年檢視` 默认 2026:
   - 检查 KPI、每月在职人数、每月离职率、BU 饼图、部门离职率比较、年度表。
   - 点击前后年，数据随 `annualYear` 变化。
   - 未来年份应显示 `尚無資料` 且 CSV disabled。

边界:

- 月份跨年切换要正确，例如 2026/01 上个月为 2025/12。
- 当前 API readNumber 对非数字使用 fallback，直接请求 `month=abc` 应回 fallback 5。

### 11.5 工时统计

步骤:

1. 点击 `工時統計`，默认 period 2026 年 6 月。
2. 检查 stats: 本月应出勤日、国定假日、平均请假时数、全勤人数。
3. 检查图例: 出勤、各假别、国定假日、周末。
4. 表格:
   - 左侧员工编号/部门/员工 sticky。
   - 每日列标日期和星期。
   - leave cell 显示假别 code 与小时。
   - 汇总列包含应出勤、出勤、各假别、请假、扣薪、生日。
5. 点击上/下月，确认 period 与数据刷新。
6. 点击 `匯出 CSV`，确认文件名 `出勤統計_YYYY-MM.csv`，每行列数等于 header。

边界:

- API 对 `month=0` clamp 到 1，`month=13` clamp 到 12，非整数 fallback 6。
- 未来月份若 summary 为 null，应显示 `—` 而非崩溃。

### 11.6 打卡时间

步骤:

1. 点击 `打卡時間`。
2. 检查 stats: 打卡异常人数、异常天数、正常打卡天数。
3. 点击异常人数卡片 `查看`，打开 `打卡異常明細` modal。
4. 在异常 modal 点击 `匯出 CSV`，确认文件名 `打卡異常明細_YYYY-MM.csv`。
5. 点击页面 `打卡定位`，打开 fallback map modal。
6. 点击 `匯出 CSV`，确认文件名 `打卡時間_YYYY-MM.csv`。
7. 表格检查:
   - 正常上下班显示时间和地点。
   - 半天请假显示打卡和 `半天` tag。
   - 异常显示 `需補卡` 与 title 原因。

边界:

- 空打卡 cell 应显示空白，不出现 `undefined`。
- 假日/周末样式和工时统计一致。

### 11.7 假勤制度

步骤:

1. 点击 `假勤制度`。
2. 初始 `還原`、`儲存` disabled。
3. 修改标准上班/下班时间，dirty 后按钮 enabled。
4. 点击 `還原`，确认恢复原始值并 disabled。
5. 修改假别名称、年度配额、累计规则、需附证明。
6. 点击 `儲存`。

边界:

- 当前无前端校验阻止下班早于上班、休息结束早于开始、空假别名称。需要后端或后续前端校验。
- 保存使用 `/api/attendance/policies/current` 且带 `X-Approval-Confirmed: true`，当前本地 Next mock 未实现。

### 11.8 管理员设置

步骤:

1. 点击 `管理員設定`。
2. 表格检查管理员、部门/职位、权限摘要、指派信息。
3. 勾选行，出现批次删除栏。
4. 点击行操作 `編輯 {姓名} 權限`:
   - 修改预览/编辑 checkbox。
   - 编辑勾选后预览 checkbox disabled 且保持 checked。
   - 取消编辑应回到 view。
5. 点击行操作 `刪除管理員`。
6. 点击 `新增管理員`:
   - 未选择候选人时 `送出` disabled。
   - 搜索姓名、员工编号、部门。
   - 已是管理员的人不应出现在候选人。
   - 选择候选人后可调整权限并送出。

边界:

- 添加、编辑、删除使用 `/api/platform/workspace/admins*`，当前本地 Next mock 未实现。
- 搜索只显示前 6 笔候选人。
- 批次删除逐个调用 delete，需测试部分失败时状态是否一致。

### 11.9 操作纪录

步骤:

1. 点击 `操作紀錄`。
2. 检查表格时间、操作者、类型 tag、操作内容。
3. 筛选:
   - 操作者。
   - 操作类型。
   - 日期范围: 全部、近 7 天、近 30 天、近 90 天。
   - 搜索关键字。
4. 未筛选时分页 pageSize 10。
5. 有筛选时分页关闭，并显示 `符合 N 筆`。

当前实现限制:

- 过滤主要依赖 `/api/platform/workspace/audit-logs*` 服务端接口，且使用 silentError。
- 当前本地 Next mock 未实现该接口时，页面会 fallback 使用初始 logs，筛选控件可能不会实际改变表格行。
- 日期范围 from 是基于当前 logs 的 latestDate，不是浏览器今天。

### 11.10 表单设计

步骤:

1. 点击 `表單設計`。
2. 检查 stats: 表单总数、已启用、未启用、本月新增。
3. 筛选:
   - 类别。
   - 启用状态。
   - 搜索表单名称、类别、流程。
4. 点击启用 Switch，预期调用 update API。
5. 点击预览 icon，打开表单预览 modal。
6. 点击更多 `編輯`，进入 builder。
7. 点击更多 `刪除`，预期调用 delete API。
8. 点击 `新增表單`:
   - 表单名称为空时 `下一步` disabled。
   - 填写名称、类别、Icon、描述后进入 builder。
9. Builder:
   - 点击分栏布局，预览新增 layout field。
   - 点击各种字段类型，预览新增字段。
   - 点击字段卡片，在右侧属性修改 label、placeholder、必填。
   - 删除字段后若删除当前选中字段，属性面板应回到未选中状态。
   - 点击 `預覽`，确认字段和审查流程渲染。
   - 点击 `編輯審核流程`，添加审查/条件/会签/知会节点，移除节点。
   - 点击 `儲存`。

边界:

- 保存、启停、删除使用 `/api/platform/workspace/forms*`，当前本地 Next mock 未实现。
- 新建表单保存时默认 `enabled=false`。
- Field id 与 stage id 在当前组件内自增，反复进入 builder 后需确认无 key 冲突。

## 12. `/design-system` 元件对照页

步骤:

1. 打开 `/design-system`。
2. 桌面左侧 Anchor 点击每个 section:
   - Foundations、Buttons、Forms、Data Display、Feedback、Navigation。
3. 点击 `收合目錄`，确认侧栏宽度收为 0；点击固定 `目錄` 按钮展开。
4. 移动端确认侧栏隐藏，内容全宽。
5. Feedback section 中 message、notification、modal、popover/popconfirm 都能触发。
6. Forms section 中 Input、Select、Checkbox、Radio、Switch、Slider、InputNumber、DatePicker、Form 验证状态正常。
7. Data Display 中 Table、Progress、Skeleton、Spin、Empty、Tooltip 显示正确。

边界:

- 此页使用 AntD `App` 包裹，反馈 API 应拿到主题 context。
- Anchor offsetTop/targetOffset 为 24，点击后标题不应被遮挡。

## 13. `/insight-example` 静态洞察示例

步骤:

1. 打开 `/insight-example`。
2. 默认 `部門工時`。
3. 切换 `業務績效`、`財務統計`。
4. 每个 tab 点击上/下月按钮，确认月份文案和静态数据切换。
5. 检查表格无分页、图表不溢出。

边界:

- 该页是静态 showcase，不走 `/api/platform/insights`。
- Month offset 可以持续前后切换，需确认 label 不错乱。

## 14. API contract 测试数据

### 14.1 Auth API

```json
POST /api/auth/login
{
  "email": "admin@demo.local",
  "password": "Password123!",
  "remember_me": true
}
```

预期: 200，字段为 snake_case，axios 后在前端变成 `expiresAt`、`nextPath`。

```json
POST /api/auth/register
{
  "name": "王小明",
  "email": "new.user@example.com",
  "password": "Password123!"
}
```

预期: 201，返回 session。

### 14.2 Assistants API

| URL | 预期 |
|---|---|
| `/api/platform/assistants?tag=all` | 最多 9 笔 data，total 为全部匹配数 |
| `/api/platform/assistants?tag=analytics` | 只含 analytics |
| `/api/platform/assistants?search=CRM` | 匹配 CRM |
| `/api/platform/assistants?search=zzzz` | data 空，total 0 |

### 14.3 Attendance period API

| URL | 预期 |
|---|---|
| `/api/platform/workspace/attendance?year=2026&month=6` | 2026 年 6 月 |
| `/api/platform/workspace/attendance?year=2026&month=0` | month clamp 为 1 |
| `/api/platform/workspace/attendance?year=2026&month=13` | month clamp 为 12 |
| `/api/platform/workspace/attendance?year=abc&month=abc` | year fallback 2026，month fallback 6 |

### 14.4 Turnover API

| URL | 预期 |
|---|---|
| `/api/platform/workspace/turnover?year=2026&month=5&annualYear=2026` | 月/年数据都有 |
| `/api/platform/workspace/turnover?year=2035&month=1&annualYear=2035` | 未来数据应为 empty/future 状态 |
| `/api/platform/workspace/turnover?year=abc&month=abc&annualYear=abc` | fallback 到 year 2026、month 5、annualYear 2026 |

## 15. 已有自动化测试覆盖

| 文件 | 覆盖 |
|---|---|
| `utils/case.test.ts` | camel/snake 转换、hyphen key、FormData |
| `libs/axios.contract.test.ts` | request/response case 转换、blob、silentError、401 logout、schema fallback |
| `app/api/platform/_mockData.test.ts` | insights 月份数据重算、workspace overview deterministic |
| `app/(platform)/_components/WelcomeHero.test.tsx` | hero send 进入 assistants、Enter 不发送 |
| `app/(platform)/_components/ClockCard.test.tsx` | 打卡按钮 gate、出席纪录 modal、定位打卡成功路径 |
| `app/(platform)/forms/_components/forms.helpers.test.ts` | 日期筛选、selection click、请假小时计算 |
| `app/(platform)/insights/_components/ReportDataTable.test.tsx` | 可访问表格、总数显示 |
| `app/(platform)/workspace/_components/attendanceCsv.test.ts` | CSV header/row 对齐 |

建议补充的自动化用例:

- AuthPage 登录/注册表单验证和 cookie。
- AssistantsListView 分类/搜索/空态。
- FormsLeftPane 已申请/草稿日期与状态筛选。
- FormModal leave/generic schema、confirm/success step。
- NotificationsPage selection、batch action、relative date 过滤。
- Workspace employee filters 当前缺失过滤行为的回归用例。
- OrganizationParentPicker 排除 self/descendant。
- FormDesign builder 新增/删除 field/stage。

## 16. 测试执行记录模板

```md
## 测试记录

- 日期:
- 分支/代码快照:
- Node/pnpm:
- 环境变量:
- 后端/API mock:
- 浏览器:

### 命令结果

- pnpm lint:
- pnpm test:run:
- pnpm build:

### 页面结果

| 页面 | 桌面 | 移动 | API | 备注 |
|---|---|---|---|---|
| /login | PASS/FAIL | PASS/FAIL | PASS/FAIL | |
| /register | PASS/FAIL | PASS/FAIL | PASS/FAIL | |
| / | PASS/FAIL | PASS/FAIL | PASS/FAIL | |
| /assistants | PASS/FAIL | PASS/FAIL | PASS/FAIL | |
| /tasks | PASS/FAIL | PASS/FAIL | PASS/FAIL | |
| /forms | PASS/FAIL | PASS/FAIL | PASS/FAIL | |
| /notifications | PASS/FAIL | PASS/FAIL | PASS/FAIL | |
| /insights | PASS/FAIL | PASS/FAIL | PASS/FAIL | |
| /workspace | PASS/FAIL | PASS/FAIL | PASS/FAIL | |
| /design-system | PASS/FAIL | PASS/FAIL | N/A | |
| /insight-example | PASS/FAIL | PASS/FAIL | N/A | |

### 缺陷记录

| ID | 严重度 | 页面 | 步骤 | 实际 | 预期 | 截图/日志 |
|---|---|---|---|---|---|---|
| BUG-001 | P1/P2/P3 | | | | | |
```

