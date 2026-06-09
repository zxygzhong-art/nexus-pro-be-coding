#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "Usage: .ai-workflow/auto-codex-cycle.sh \"task prompt\""
  exit 2
fi

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

task="$*"
review_base_ref="${CODEX_REVIEW_BASE_REF:-origin/main}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

echo "== Codex implement =="
.ai-workflow/codex-fix.sh "$task"

echo "== Codex review =="
set +e
.ai-workflow/codex-review.sh "$review_base_ref" > "$tmp_dir/review.md" 2>&1
review_status=$?
set -e

cat "$tmp_dir/review.md"

echo "== Codex review-fix pass =="
codex exec --sandbox workspace-write "$(cat <<PROMPT
下面是本轮自动 review 输出。请只修复其中的 P0/P1 或明确会导致真实 bug 的问题。
如果 review 没有发现 P0/P1，或者只有风格建议，请不要修改文件。

原始任务：
$task

Review base:
$review_base_ref

Review 输出：
$(cat "$tmp_dir/review.md")

要求：
- 只做最小必要改动。
- 不提交、不 push。
- 完成后运行最小必要验证；如无法验证，说明原因。
PROMPT
)"

if [ "$review_status" -ne 0 ]; then
  echo "Review command exited with status $review_status; continuing because Codex may still have produced actionable output."
fi
