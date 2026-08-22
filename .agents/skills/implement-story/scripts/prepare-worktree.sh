#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: prepare-worktree.sh --story <id-or-name> [--base <ref>] [--dry-run]

Prepare codex/<story-slug> in the current linked worktree, or create it under
<repository>/.worktrees/<story-slug> when invoked from the primary checkout.
The base defaults to the current committed HEAD.
EOF
}

story=""
base_ref="HEAD"
dry_run="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --story)
      [[ $# -ge 2 ]] || { echo "error: --story requires a value" >&2; exit 2; }
      story="$2"
      shift 2
      ;;
    --base)
      [[ $# -ge 2 ]] || { echo "error: --base requires a value" >&2; exit 2; }
      base_ref="$2"
      shift 2
      ;;
    --dry-run)
      dry_run="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[[ -n "$story" ]] || { echo "error: --story is required" >&2; exit 2; }

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "error: current directory is not inside a Git worktree" >&2
  exit 1
}
repo_root=$(cd "$repo_root" && pwd -P)

slug=$(printf '%s' "$story" \
  | tr '[:upper:]' '[:lower:]' \
  | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-+/-/g' \
  | cut -c1-64)
[[ -n "$slug" ]] || { echo "error: story does not produce a safe slug" >&2; exit 2; }

branch="codex/$slug"
git check-ref-format --branch "$branch" >/dev/null 2>&1 || {
  echo "error: generated branch name is invalid: $branch" >&2
  exit 1
}

base_commit=$(git rev-parse --verify "${base_ref}^{commit}" 2>/dev/null) || {
  echo "error: base ref does not resolve to a commit: $base_ref" >&2
  exit 1
}

current_branch=$(git branch --show-current)
primary_root=$(git worktree list --porcelain \
  | awk '/^worktree / { print substr($0, 10); exit }')
primary_root=$(cd "$primary_root" && pwd -P)

pr_base=""
case "$base_ref" in
  HEAD)
    pr_base="$current_branch"
    ;;
  refs/heads/*)
    pr_base=${base_ref#refs/heads/}
    ;;
  refs/remotes/origin/*)
    pr_base=${base_ref#refs/remotes/origin/}
    ;;
  origin/*)
    pr_base=${base_ref#origin/}
    ;;
  *)
    if git show-ref --verify --quiet "refs/heads/$base_ref"; then
      pr_base="$base_ref"
    fi
    ;;
esac

if [[ "$base_ref" == "HEAD" && -z "$pr_base" ]]; then
  primary_branch=$(git -C "$primary_root" branch --show-current)
  if [[ -n "$primary_branch" ]]; then
    primary_commit=$(git -C "$primary_root" rev-parse HEAD)
    if [[ "$primary_commit" == "$base_commit" ]]; then
      pr_base="$primary_branch"
    fi
  fi
fi

print_result() {
  local status_value="$1"
  local worktree_value="$2"
  printf 'status=%s\n' "$status_value"
  printf 'repository=%s\n' "$primary_root"
  printf 'worktree=%s\n' "$worktree_value"
  printf 'branch=%s\n' "$branch"
  printf 'base_ref=%s\n' "$base_ref"
  printf 'base_commit=%s\n' "$base_commit"
  printf 'pr_base=%s\n' "$pr_base"
}

if [[ "$repo_root" != "$primary_root" ]]; then
  if [[ "$current_branch" == "$branch" ]]; then
    print_result "resumed-current" "$repo_root"
    exit 0
  fi

  if [[ -n "$current_branch" ]]; then
    echo "error: linked worktree is already on branch $current_branch, expected $branch" >&2
    exit 1
  fi

  current_commit=$(git rev-parse HEAD)
  if [[ "$current_commit" != "$base_commit" ]]; then
    echo "error: linked worktree HEAD does not match requested base $base_ref" >&2
    exit 1
  fi

  if git show-ref --verify --quiet "refs/heads/$branch"; then
    echo "error: branch already exists and is not checked out here: $branch" >&2
    exit 1
  fi

  if [[ "$dry_run" == "true" ]]; then
    print_result "would-prepare-current" "$repo_root"
    exit 0
  fi

  git switch -c "$branch" >&2
  print_result "prepared-current" "$repo_root"
  exit 0
fi

worktrees_root="$primary_root/.worktrees"
worktree_path="$worktrees_root/$slug"
registered_path=$(git worktree list --porcelain \
  | awk -v ref="refs/heads/$branch" '
      /^worktree / { path = substr($0, 10) }
      /^branch / && $2 == ref { print path; exit }
    ')

if [[ -n "$registered_path" ]]; then
  registered_path=$(cd "$registered_path" && pwd -P)
  if [[ "$registered_path" == "$worktree_path" ]]; then
    print_result "resumed" "$worktree_path"
    exit 0
  fi
  echo "error: branch is already registered to another worktree: $registered_path" >&2
  exit 1
fi

if [[ -e "$worktree_path" ]]; then
  echo "error: target path already exists but is not the expected registered worktree: $worktree_path" >&2
  exit 1
fi

if git show-ref --verify --quiet "refs/heads/$branch"; then
  echo "error: branch already exists without the expected worktree: $branch" >&2
  exit 1
fi

if [[ "$dry_run" == "true" ]]; then
  print_result "would-create" "$worktree_path"
  exit 0
fi

common_dir=$(git rev-parse --path-format=absolute --git-common-dir)
exclude_file="$common_dir/info/exclude"
mkdir -p "$worktrees_root" "$(dirname "$exclude_file")"
touch "$exclude_file"
if ! grep -Fqx '/.worktrees/' "$exclude_file"; then
  printf '\n/.worktrees/\n' >> "$exclude_file"
fi

git worktree add -b "$branch" "$worktree_path" "$base_commit" >&2
print_result "created" "$worktree_path"
