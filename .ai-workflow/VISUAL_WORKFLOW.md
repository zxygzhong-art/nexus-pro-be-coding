# Local Codex Visual Workflow

## 总流程

```mermaid
flowchart TD
  A["GitHub Actions 手动触发"] --> B{"单任务还是队列?"}
  B -->|"Local Codex Automation"| C["填写 branch + task"]
  B -->|"Local Codex Task Queue"| Q["读取 queue JSON"]
  Q --> Q1["按顺序选择 enabled task"]
  Q1 --> C
  C --> D["self-hosted runner on local Mac"]
  D --> E["切换/创建 feature branch"]
  E --> F["Codex implement"]
  F --> G["Codex review"]
  G --> H["Codex fix review findings"]
  H --> I{"有文件变更?"}
  I -->|"是"| J["commit + push feature branch"]
  I -->|"否"| K["记录 no changes"]
  J --> L["人工查看 diff / PR / 合并"]
  K --> L
  L --> M["大版本快照到正式仓库 nexus-pro-be"]
```

## 员工管理队列

```mermaid
flowchart LR
  M1["M1 list filters<br/>feature/employee-list-filters<br/>done"] --> M2["M2 CRUD local state<br/>feature/employee-crud-local-state"]
  M2 --> M3["M3 bulk delete + export<br/>feature/employee-bulk-delete-export"]
  M3 --> M4["M4 CSV import preview<br/>feature/employee-csv-import-preview"]
  M4 --> M5["M5 detail tabs<br/>feature/employee-detail-tabs"]
  M5 --> M6["M6 org linkage<br/>feature/employee-org-linkage"]
  M6 --> M7["M7 regression<br/>feature/employee-management-regression"]
```

## 可视化位置

- GitHub Actions run 页面：实时看当前 job、耗时、成功/失败。
- `Local Codex Task Queue` 的 run summary：显示队列流程图、每个任务的 branch、结果和 commit。
- 本文件：看整体流程和任务依赖。

## 队列运行方式

打开：

```text
https://github.com/zxygzhong-art/nexus-pro-be-coding/actions/workflows/local-codex-queue.yml
```

常用输入：

```text
queue_file: .ai-workflow/queues/employee-management.json
start_from: M2
max_tasks: 1
dry_run: false
```

说明：

- `max_tasks=1`：一次只跑一个模块，推荐日常使用。
- `max_tasks=0`：从 `start_from` 开始一路跑完所有 enabled 任务。
- `dry_run=true`：只显示队列计划，不执行 Codex。
