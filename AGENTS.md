# AGENTS.md

## 協作偏好

- 執行任務時，預設先區分哪些步驟可以並行、哪些必須串行；不要把本可並行的工作保守地全按順序執行。
- 資訊蒐集儘量並行化，例如同時搜尋相關檔案、測試、呼叫鏈、設定和日誌；只有存在明確依賴關係的步驟才串行推進。
- 優先走最短關鍵路徑，先完成能推進判斷和交付的最小閉環，減少過度探索和不必要的上下文鋪陳。
- 驗證採用分層策略：先做最小必要驗證，再根據結果決定是否擴大驗證範圍；除非明確要求，否則不要預設執行耗時的全量檢查。
- 在沒有明顯高風險時，可以自行做合理假設並繼續執行，減少頻繁確認。預設偏速度優先，但保證基本正確性。
- 使用者給出精確路徑、分支、工作區或「不修改程式碼/只給方案」等邊界時，以最新明確邊界為準。
- 不回滾或覆蓋使用者已有改動；遇到無關髒工作區時忽略，遇到相關改動時先讀懂並在其基礎上繼續。
- 最終回饋優先說明改了什麼、如何驗證、剩餘風險；避免泛泛解釋。

## 專案畫像

- 這是 Go 後端專案，當前主體是模組化單體，核心邊界包括 `internal/api/v1`、`internal/service`、`internal/repository`、`internal/domain`、`internal/platform/postgres` 和 `tests/unit`。
- 當前 people-domain / employee / IAM / agent 相關工作仍以「保持既有 API 行為，逐步補齊契約」為主，避免無關大重構。
- `docs/openapi.yaml` 是 API 契約源頭；後續做程式碼生成時優先走 OpenAPI spec-first（例如由 OpenAPI 生成 Go types/server interface），不要把 ctrl 註解生成作為主契約來源。
- 需要回答啟動、驗證或設定問題時，先看真實倉庫檔案，例如 `Makefile`、`.env.example`、`internal/config/config.go`、`docs/openapi.yaml`，不要按通用 Go 專案習慣猜。
- 涉及員工管理前後端契約時，如果使用者指向 `~/Desktop/platform-ui`，把該目錄視為 UI/互動契約來源之一，並與 OpenAPI、領域模型、測試一起核對。
- 需求補齊優先按階段推進：需求矩陣 -> schema 對齊決策 -> employee 校驗/匯入硬化 -> 權限閉環 -> PostgreSQL/RLS 整合 -> Agent runtime。

## 程式碼規範

- 測試優先放在 `tests/unit/...`，按模組鏡像目錄組織；不要把新的單元測試隨意散落到 `internal/...`，除非現有模式或 Go 套件可見性確實要求。
- 請求相關的 repository/store 路徑應顯式傳遞 `context.Context`；避免在請求鏈路裡用 `context.Background()` 或 panic 型 helper 掩蓋錯誤。
- 鑑權邊界必須 token-first：token 派生的 tenant/account 身分優先於可偽造請求頭；臨時角色/assumed role 只能來自已驗證的會話狀態。
- 路由策略、authz resource/action 字串、service 寫路徑和 authz snapshot 要一起核對；缺失 `data_scope_id` 等關鍵約束時應 fail closed。
- 修改 `internal/api/v1` 的 ctrl/handler/路由契約時，必須同步檢查並更新 `docs/openapi.yaml`；如確認無 OpenAPI 契約變化，最終回饋必須明確說明原因。
- IAM permission-set assignment 相關路由和服務應使用專用 `permission_set_assignment` resource，不要混用普通 permission-set 資源。
- 涉及 tenant 資料寫入時，優先走現有 transaction helper，保證錯誤和 panic 都能回滾，不留下部分寫入。
- 員工可見範圍、部門選項、列表結果等必須來自當前 authz 決策下的可見資料，不要退回全租戶列表。
- XLSX 員工匯入保持 10 列契約，尤其不要遺失第 J 列 `主管員工ID`。
- 修改 `db/queries/*.sql` 後執行 `make sqlc`；修改遷移後執行 `make migrate-validate`。

## Platform 與 Workspace 邊界

- **Platform**（`/v1/platform/*`）：授權對象為 `me` 的個人門戶聚合（home、assistants、我的表單填寫、tasks/todos）。
- **Workspace**（`/v1/workspace/*`）：授權對象為租戶級資源（`hr.employee`、`iam.permission_set_assignment`、`workflow.form_template`、`audit.log`、`attendance.clock`）的管理後台能力。
- **Insights** 歸 Workspace（`GET /v1/workspace/insights`），資料來自 HR 概覽，不是個人 `me` 域。
- service 層 **Platform 可依賴 WorkspaceFacade，Workspace 不可依賴 Platform**。
- 新增 API 時依授權判据落位；禁止在 `/v1/platform/workspace/*` 下新增路由。

## 專案驗證

- 優先使用倉庫已有命令：`make unit-test`、`make test`、`make sqlc`、`make migrate-validate`。
- 在本環境跑 Go 測試時優先加 `GOCACHE=$PWD/.gocache`，避免預設快取路徑或並發清理導致的失敗。
- 修改 Go 程式碼後先跑最小相關驗證，例如：
  - API v1：`GOCACHE=$PWD/.gocache go test ./internal/api/v1 ./tests/unit/api/v1`
  - service：`GOCACHE=$PWD/.gocache go test ./internal/service ./tests/unit/service`
  - unit baseline：`GOCACHE=$PWD/.gocache go test ./tests/unit/...`
- 需要擴大驗證時再執行 `GOCACHE=$PWD/.gocache go test ./...` 或 `make test`；除非任務明確需要，不預設做耗時全量檢查。
- 本專案不會自動載入 `.env`；本地啟動前需要手動匯出環境，常用方式是 `set -a; source .env; set +a`。
- 最快的無外部依賴 smoke 可以不設定 `DATABASE_URL` / `REDIS_ADDR`，啟動後檢查 `/healthz`、`/v1/me`、`/swagger/index.html`、`/openapi.yaml`。
- 不要預設啟動依賴 Docker 的本地服務，除非任務明確需要整合驗證。

## Git 與工作區

- 開始涉及程式碼或歷史操作前先看 `git status --short`，確認當前工作區和分支。
- `nexus-pro-be`、相鄰 sibling repo、以及 `.codex/worktrees/.../nexus-pro-be` 可能不是同一個工作區；路徑比名稱更權威。
- 歷史改寫、分支整理、刪除 worktree 或本地分支前，要先證明目標路徑和 diff/HEAD 關係；不要用寬泛假設操作相似目錄。
- `docs/people-domain-employee-iam-test-plan.md` 這類臨時/計畫文件曾被當作 opt-in 上下文處理；不要自動納入提交，除非使用者要求。


<claude-mem-context>
# Memory Context

# [nexus-pro-be] recent context, 2026-07-06 2:35pm GMT+9

No previous sessions found.
</claude-mem-context>
