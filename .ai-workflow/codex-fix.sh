#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "Usage: .ai-workflow/codex-fix.sh \"task prompt\""
  exit 2
fi

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

prompt="$*"

codex exec --sandbox workspace-write "$prompt

请在本机工作区完成修改，遵循 AGENTS.md。
要求：
- 先理解现有结构，再做最小必要改动。
- 不提交、不 push。
- 完成后运行最小必要验证；如果无法验证，请说明原因。
- 最后用简短中文总结改动、验证和残余风险。"
