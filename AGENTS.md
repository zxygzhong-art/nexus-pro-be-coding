# AGENTS.md

## 协作偏好

- 执行任务时，默认先区分哪些步骤可以并行、哪些必须串行；不要把本可并行的工作保守地全按顺序执行。
- 信息收集尽量并行化，例如同时搜索相关文件、测试、调用链、配置和日志；只有存在明确依赖关系的步骤才串行推进。
- 优先走最短关键路径，先完成能推进判断和交付的最小闭环，减少过度探索和不必要的上下文铺陈。
- 验证采用分层策略：先做最小必要验证，再根据结果决定是否扩大验证范围；除非明确要求，否则不要默认执行耗时的全量检查。
- 在没有明显高风险时，可以自行做合理假设并继续执行，减少频繁确认。默认偏速度优先，但保证基本正确性。
- 用户给出精确路径、分支、工作区或“不修改代码/只给方案”等边界时，以最新明确边界为准。
- 不回滚或覆盖用户已有改动；遇到无关脏工作区时忽略，遇到相关改动时先读懂并在其基础上继续。
- 最终反馈优先说明改了什么、如何验证、剩余风险；避免泛泛解释。

## 项目画像

- 这是 Go 后端项目，当前主体是模块化单体，核心边界包括 `internal/api/v1`、`internal/service`、`internal/repository`、`internal/domain`、`internal/platform/postgres` 和 `tests/unit`。
- 当前 people-domain / employee / IAM / agent 相关工作仍以“保持既有 API 行为，逐步补齐契约”为主，避免无关大重构。
- 需要回答启动、验证或配置问题时，先看真实仓库文件，例如 `Makefile`、`.env.example`、`internal/config/config.go`、`docs/openapi.yaml`，不要按通用 Go 项目习惯猜。
- 涉及员工管理前后端契约时，如果用户指向 `~/Desktop/platform-ui`，把该目录视为 UI/交互契约来源之一，并与 OpenAPI、领域模型、测试一起核对。
- 需求补齐优先按阶段推进：需求矩阵 -> schema 对齐决策 -> employee 校验/导入硬化 -> 权限闭环 -> PostgreSQL/RLS 集成 -> Agent runtime。

## 代码规范

- 测试优先放在 `tests/unit/...`，按模块镜像目录组织；不要把新的单元测试随意散落到 `internal/...`，除非现有模式或 Go 包可见性确实要求。
- 请求相关的 repository/store 路径应显式传递 `context.Context`；避免在请求链路里用 `context.Background()` 或 panic 型 helper 掩盖错误。
- 鉴权边界必须 token-first：token 派生的 tenant/account 身份优先于可伪造请求头；临时角色/assumed role 只能来自已验证的会话状态。
- 路由策略、authz resource/action 字符串、service 写路径和 authz snapshot 要一起核对；缺失 `data_scope_id` 等关键约束时应 fail closed。
- IAM permission-set assignment 相关路由和服务应使用专用 `permission_set_assignment` resource，不要混用普通 permission-set 资源。
- 涉及 tenant 数据写入时，优先走现有 transaction helper，保证错误和 panic 都能回滚，不留下部分写入。
- 员工可见范围、部门选项、列表结果等必须来自当前 authz 决策下的可见数据，不要退回全租户列表。
- XLSX 员工导入保持 10 列契约，尤其不要丢失第 J 列 `主管員工ID`。
- 修改 `db/queries/*.sql` 后运行 `make sqlc`；修改迁移后运行 `make migrate-validate`。

## 项目验证

- 优先使用仓库已有命令：`make unit-test`、`make test`、`make sqlc`、`make migrate-validate`。
- 在本环境跑 Go 测试时优先加 `GOCACHE=$PWD/.gocache`，避免默认缓存路径或并发清理导致的失败。
- 修改 Go 代码后先跑最小相关验证，例如：
  - API v1：`GOCACHE=$PWD/.gocache go test ./internal/api/v1 ./tests/unit/api/v1`
  - service：`GOCACHE=$PWD/.gocache go test ./internal/service ./tests/unit/service`
  - unit baseline：`GOCACHE=$PWD/.gocache go test ./tests/unit/...`
- 需要扩大验证时再运行 `GOCACHE=$PWD/.gocache go test ./...` 或 `make test`；除非任务明确需要，不默认做耗时全量检查。
- 本项目不会自动加载 `.env`；本地启动前需要手动导出环境，常用方式是 `set -a; source .env; set +a`。
- 最快的无外部依赖 smoke 可以不设置 `DATABASE_URL` / `REDIS_ADDR`，启动后检查 `/healthz`、`/v1/me`、`/swagger/index.html`、`/openapi.yaml`。
- 不要默认启动依赖 Docker 的本地服务，除非任务明确需要集成验证。

## Git 与工作区

- 开始涉及代码或历史操作前先看 `git status --short`，确认当前工作区和分支。
- `nexus-pro-be`、相邻 sibling repo、以及 `.codex/worktrees/.../nexus-pro-be` 可能不是同一个工作区；路径比名称更权威。
- 历史改写、分支整理、删除 worktree 或本地分支前，要先证明目标路径和 diff/HEAD 关系；不要用宽泛假设操作相似目录。
- `docs/people-domain-employee-iam-test-plan.md` 这类临时/计划文档曾被当作 opt-in 上下文处理；不要自动纳入提交，除非用户要求。
