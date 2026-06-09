#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root/platform-ui"

port="${1:-4173}"
echo "Serving platform-ui at http://127.0.0.1:${port}"
python3 -m http.server "$port"
