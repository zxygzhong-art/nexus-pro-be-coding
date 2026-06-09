# Local Codex Runner Migration

这份文档用于把 `nexus-pro-be-coding` 的全自动 AI coding 流程迁移到另一台 Mac。

## 当前已跑通的链路

```text
GitHub task
  -> self-hosted runner on local Mac
  -> codex exec implement
  -> codex exec review
  -> codex exec fix review findings
  -> commit and push to nexus-pro-be-coding
```

已验证成功的 GitHub Actions run：

```text
https://github.com/zxygzhong-art/nexus-pro-be-coding/actions/runs/27180996285
```

## 新机器前置条件

- 已安装 Git。
- 已安装 Codex CLI，并且当前 macOS 用户能运行：

  ```bash
  codex --version
  codex exec --sandbox read-only "请确认 Codex 可以运行，不要修改文件。"
  ```

- 新机器可以访问 GitHub。
- 新机器用于跑 runner 的 macOS 用户有自己的 `~/.codex`。

不要把 runner 安装到带空格的路径里，例如 `Application Support`。推荐路径：

```text
/Users/<mac-user>/github-actions-runners/nexus-pro-be-coding
```

## 迁移步骤

1. 克隆 coding 仓库：

   ```bash
   mkdir -p /Users/<mac-user>/Desktop/ai-coding
   cd /Users/<mac-user>/Desktop/ai-coding
   git clone git@github-zxygzhong-art:zxygzhong-art/nexus-pro-be-coding.git
   ```

2. 如果新机器没有 `github-zxygzhong-art` SSH host，生成新 key 并添加到 GitHub：

   ```bash
   ssh-keygen -t ed25519 -C "zxygzhong-art" -f ~/.ssh/id_ed25519_zxygzhong_art
   ```

   `~/.ssh/config` 增加：

   ```ssh-config
   Host github-zxygzhong-art
     HostName github.com
     User git
     IdentityFile ~/.ssh/id_ed25519_zxygzhong_art
     IdentitiesOnly yes
   ```

   然后把 `~/.ssh/id_ed25519_zxygzhong_art.pub` 添加到 GitHub SSH keys，并测试：

   ```bash
   ssh -T git@github-zxygzhong-art
   ```

3. 在 GitHub 仓库页面创建新的 self-hosted runner：

   ```text
   Settings -> Actions -> Runners -> New self-hosted runner
   ```

   选择 `macOS` 和 `ARM64`，按页面给出的下载命令下载 runner。安装目录用：

   ```bash
   mkdir -p /Users/<mac-user>/github-actions-runners/nexus-pro-be-coding
   cd /Users/<mac-user>/github-actions-runners/nexus-pro-be-coding
   ```

4. 配置 runner。GitHub 页面里的 token 每次都不同，使用页面给你的新 token：

   ```bash
   ./config.sh \
     --url https://github.com/zxygzhong-art/nexus-pro-be-coding \
     --token <github-runner-token> \
     --name <machine-name>-nexus-pro-be-coding \
     --labels nexus-pro-be-coding,local-codex
   ```

   GitHub 会自动给 macOS ARM64 runner 加上 `self-hosted`、`macOS`、`ARM64` 标签。workflow 依赖的完整标签是：

   ```text
   self-hosted, macOS, ARM64, nexus-pro-be-coding, local-codex
   ```

5. 安装并启动 runner 服务：

   ```bash
   ./svc.sh install
   ./svc.sh start
   ./svc.sh status
   ```

6. 触发一次自检：

   ```text
   GitHub Actions -> Local Codex Automation -> Run workflow
   ```

   task 填：

   ```text
   自动化环境自检：不要修改任何文件。请检查当前仓库结构、确认本机 Codex 可运行，并在最终回复中说明检查结果。
   ```

   成功后应看到 workflow 状态为 `Success`，并且 coding 仓库没有多余提交。

## 多机器注意事项

- 如果多台机器同时保留同一组 labels，GitHub 会把任务派给任意空闲 runner。
- 如果只想指定某一台机器，给它加独立 label，例如 `macbook-air-local-codex`，并同步修改 `.github/workflows/local-codex.yml` 的 `runs-on`。
- 换机器后建议在 GitHub `Settings -> Actions -> Runners` 删除旧机器 runner，避免离线 runner 留在列表里。
- Codex 登录态属于本机用户环境。优先在新机器重新登录 Codex，不建议随意复制 `~/.codex` 里的认证文件。

## 日常触发

手动触发：

```text
GitHub Actions -> Local Codex Automation -> Run workflow
```

issue 或 PR 评论触发：

```text
/codex 你的开发任务
```

只有 `zxygzhong-art` 的 `/codex ...` 评论会触发本机 runner。
