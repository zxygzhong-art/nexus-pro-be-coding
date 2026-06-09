# AGENTS.md

本仓库是 `nexus-pro-be` 的 AI coding 工作仓库。这里允许 Codex 进行频繁试错、review 修复和阶段性提交；正式仓库只接收人工确认后的大版本快照。

## 工作原则

- 默认先判断哪些信息收集能并行，优先走最短关键路径。
- 修改前先看现有结构和 Git 状态，不要覆盖用户未提交内容。
- 不提交 `.DS_Store`、本地环境变量、依赖目录、构建产物。
- 代码改动保持小步、可 review、可回滚；避免无关重构。
- 每次较大改动后至少运行最小必要验证，再决定是否扩大验证。

## 本地 Codex 流程

- 写代码：`.ai-workflow/codex-fix.sh "你的任务描述"`
- 本地审查：`.ai-workflow/codex-review.sh`
- 静态预览：`.ai-workflow/serve-platform-ui.sh`
- 发布快照：`.ai-workflow/publish-snapshot.sh vX.Y.Z`

## Review Guidelines

- 优先指出 P0/P1：会导致错误结果、数据丢失、权限绕过、明显运行失败的问题。
- 前端变更要关注响应式布局、文字溢出、按钮/输入框可访问性、交互状态是否完整。
- 不要把纯风格偏好当成阻塞问题，除非它会影响可用性或品牌一致性。
- 发现问题后优先给出最小修复方案。

## Release Sync

默认正式仓库目录为：

```text
/Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be
```

从 coding 仓库发布时，只把产品文件同步过去；`.ai-workflow/`、`AGENTS.md`、`.git/`、依赖目录和本地噪音文件不会进入正式仓库。
