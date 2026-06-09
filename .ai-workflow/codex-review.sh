#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

base_ref="${1:-origin/main}"
if ! git rev-parse --verify --quiet "$base_ref" >/dev/null; then
  base_ref="HEAD"
fi

codex exec --sandbox read-only "请以代码审查模式检查当前工作区相对 ${base_ref} 的变更。

审查范围：
- 已跟踪和未跟踪文件。
- 重点找会导致真实 bug、回归、数据/权限问题、明显 UX 破坏的问题。

输出要求：
- Findings first，按严重程度排序。
- 每个问题给出文件和具体位置。
- 如果没有高优先级问题，明确说未发现 P0/P1。
- 不要修改文件。"
