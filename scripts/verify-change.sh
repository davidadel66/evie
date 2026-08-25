#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ui_dir="$repo_root/internal/web/ui"

cd "$repo_root"

if [[ ! -x "$ui_dir/node_modules/.bin/oxlint" ]] || \
   [[ ! -x "$ui_dir/node_modules/.bin/vite" ]]; then
  printf '%s\n' \
    'UI dependencies are missing. Run: npm --prefix internal/web/ui ci' >&2
  exit 1
fi

npm --prefix internal/web/ui run lint
npm --prefix internal/web/ui run build
go test ./...
go vet ./...

# Check both unstaged and staged changes. CI separately checks the complete PR
# diff because a clean checkout has neither.
git diff --check
git diff --cached --check
