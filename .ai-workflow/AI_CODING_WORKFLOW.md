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

## 日常开发闭环

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
git@gitlab.corp.ikala.tv:nexus-pro/nexus-pro-be-coding.git
```

如果仓库路径或权限还没准备好，本地提交不受影响。等远端可用后运行：

```bash
git push -u origin main
```

## 安全边界

- 不要让陌生人提交的 MR/PR 或评论直接触发你本机 Codex。
- coding 仓库可以频繁提交，但正式仓库只发布人工确认后的快照。
- 任何包含密钥、账号、内部配置的文件都不要提交；用 `.env.example` 描述配置格式。
