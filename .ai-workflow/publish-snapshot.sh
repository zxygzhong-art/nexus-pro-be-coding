#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
if [ -z "$version" ]; then
  echo "Usage: .ai-workflow/publish-snapshot.sh vX.Y.Z"
  exit 2
fi

coding_root="$(git rev-parse --show-toplevel)"
release_root="${RELEASE_ROOT:-$(cd "$coding_root/.." && pwd)/nexus-pro-be}"

if [ ! -d "$release_root/.git" ]; then
  echo "Release repo not found: $release_root"
  echo "Set RELEASE_ROOT=/path/to/release-repo if you want another target."
  exit 1
fi

if [ -n "$(git -C "$release_root" status --porcelain)" ]; then
  echo "Release repo has uncommitted changes. Please clean or commit them first:"
  git -C "$release_root" status --short
  exit 1
fi

rsync -a --delete \
  --exclude='.git' \
  --exclude='.DS_Store' \
  --exclude='.env' \
  --exclude='.env.*' \
  --exclude='node_modules' \
  --exclude='.venv' \
  --exclude='venv' \
  --exclude='dist' \
  --exclude='build' \
  --exclude='.next' \
  --exclude='.nuxt' \
  --exclude='coverage' \
  --exclude='.ai-workflow' \
  --exclude='AGENTS.md' \
  "$coding_root/" "$release_root/"

find "$release_root" -name .DS_Store -delete

git -C "$release_root" add -A
if git -C "$release_root" diff --cached --quiet; then
  echo "No release changes to commit."
  exit 0
fi

git -C "$release_root" commit -m "Release $version"
if git -C "$release_root" rev-parse --verify --quiet "$version" >/dev/null; then
  echo "Tag $version already exists; skipped tag creation."
else
  git -C "$release_root" tag -a "$version" -m "Release $version"
fi

echo "Release snapshot committed in $release_root"
echo "Next: cd \"$release_root\" && git push origin main --tags"
