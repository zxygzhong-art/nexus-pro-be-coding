# Nexus Pro BE AI Coding Workflow

## 目录角色

```text
/Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be-coding
```

AI coding 仓库。Codex 在这里写代码、反复 review、修 bug、做阶段性提交。提交历史可以密集一些，方便追踪 AI 修改过程。

```text
/Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be
```

正式/发布仓库。只接收稳定大版本快照，历史保持干净。

## 全自动开发闭环

GitHub Actions 已配置 `.github/workflows/local-codex.yml`，会把任务派发到你本机的 self-hosted runner：

```text
kuzhu-mac-nexus-pro-be-coding
```

runner 本机安装目录：

```text
/Users/kuzhiluoya/github-actions-runners/nexus-pro-be-coding
```

注意不要把 runner 放在带空格的路径里，例如 `Application Support`。GitHub runner 执行 bash step 时会把临时脚本路径传给 shell，带空格的安装路径会导致 `/Users/.../Library/Application: No such file or directory`。

迁移到其他机器时按 `.ai-workflow/MIGRATION.md` 操作。

触发方式有两种：

1. 在 GitHub Actions 页面手动运行 `Local Codex Automation`，填写 `task`。
2. 在 issue 或 PR 评论里发送：

   ```text
   /codex 你的开发任务
   ```

安全限制：

- 只有 `zxygzhong-art` 的 `/codex ...` 评论会触发本机 runner。
- workflow 会执行 `AI coding -> AI review -> AI fix review findings -> commit -> push branch`。
- 自动提交只进入 `nexus-pro-be-coding` 的任务分支，正式仓库仍然只接收你确认后的大版本快照。

如果在 Actions 页面触发，建议填写 `branch` 输入框，例如：

```text
feature/employee-list-filters
```

如果用 issue 或 PR 评论触发，可以写：

```text
/codex branch=feature/employee-list-filters 你的开发任务
```

如果要按队列自动执行多个任务，使用 `Local Codex Task Queue` workflow。队列文件默认是：

```text
.ai-workflow/queues/employee-management.json
```

流程图和队列说明在 `.ai-workflow/VISUAL_WORKFLOW.md`。

## 手动开发闭环

1. 进入 coding 仓库：

   ```bash
   cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be-coding
   ```

2. 让 Codex 在本机写代码：

   ```bash
   .ai-workflow/codex-fix.sh "实现 xxx 功能，并运行最小验证"
   ```

3. 本机 review：

   ```bash
   .ai-workflow/codex-review.sh
   ```

4. 人工看 diff：

   ```bash
   git status --short
   git diff
   ```

5. 提交到 coding 仓库：

   ```bash
   git add -A
   git commit -m "feat: ..."
   git push origin main
   ```

## 静态 UI 预览

当前项目里 `platform-ui/` 是静态 HTML/CSS/JS，可以直接启动本地服务：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be-coding
.ai-workflow/serve-platform-ui.sh
```

默认地址：

```text
http://127.0.0.1:4173
```

## 发布大版本到正式仓库

确认 coding 仓库已稳定后：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be-coding
.ai-workflow/publish-snapshot.sh v0.1.0
```

脚本会把 coding 仓库快照同步到：

```text
/Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be
```

并在正式仓库里创建一个干净提交：

```text
Release v0.1.0
```

然后你再执行：

```bash
cd /Users/kuzhiluoya/Desktop/ai-coding/nexus-pro-be
git push origin main --tags
```

## Git remote

coding 仓库 remote 预设为：

```text
git@github-zxygzhong-art:zxygzhong-art/nexus-pro-be-coding.git
```

本机 SSH host `github-zxygzhong-art` 会使用专门给 `zxygzhong-art` 配置的 SSH key。
如果以后重新 clone 或切 remote，可以运行：

```bash
git remote set-url origin git@github-zxygzhong-art:zxygzhong-art/nexus-pro-be-coding.git
git push -u origin main
```

## 安全边界

- 不要让陌生人提交的 MR/PR 或评论直接触发你本机 Codex。
- coding 仓库可以频繁提交，但正式仓库只发布人工确认后的快照。
- 任何包含密钥、账号、内部配置的文件都不要提交；用 `.env.example` 描述配置格式。
