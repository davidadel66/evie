"""Git and deterministic-command boundaries for a story candidate."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path
from typing import Any


class RepositoryError(RuntimeError):
    """Raised when a candidate does not match repository state."""


def _run(
    arguments: list[str],
    *,
    cwd: Path,
    check: bool = True,
    timeout: int | None = None,
) -> subprocess.CompletedProcess[str]:
    try:
        result = subprocess.run(
            arguments,
            cwd=cwd,
            check=False,
            capture_output=True,
            text=True,
            timeout=timeout,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise RepositoryError(
            f"command failed to run: {' '.join(arguments)}: {exc}"
        ) from exc
    if check and result.returncode != 0:
        detail = (result.stderr or result.stdout).strip()
        raise RepositoryError(
            f"command exited {result.returncode}: {' '.join(arguments)}"
            + (f": {detail}" if detail else "")
        )
    return result


def repository_root(path: Path) -> Path:
    result = _run(["git", "rev-parse", "--show-toplevel"], cwd=path)
    return Path(result.stdout.strip()).resolve()


def primary_worktree(path: Path) -> Path:
    root = repository_root(path)
    result = _run(["git", "worktree", "list", "--porcelain"], cwd=root)
    for line in result.stdout.splitlines():
        if line.startswith("worktree "):
            return Path(line.removeprefix("worktree ")).resolve()
    raise RepositoryError(f"Git did not report a primary worktree for {root}")


def git_common_dir(path: Path) -> Path:
    result = _run(
        ["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
        cwd=path,
    )
    return Path(result.stdout.strip()).resolve()


def git_text(path: Path, *arguments: str) -> str:
    return _run(["git", *arguments], cwd=path).stdout.strip()


def prepare_story_worktree(
    repository: Path,
    *,
    worktree: Path,
    branch: str,
    base_commit: str,
) -> None:
    """Create or reconcile the controller-owned isolated worktree."""
    repository = primary_worktree(repository)
    worktree = worktree.resolve()
    expected_parent = (repository / ".worktrees").resolve()
    if worktree.parent != expected_parent:
        raise RepositoryError(
            f"story worktree must be directly under {expected_parent}"
        )
    _run(["git", "check-ref-format", "--branch", branch], cwd=repository)
    actual_base = git_text(
        repository, "rev-parse", "--verify", f"{base_commit}^{{commit}}"
    )
    if actual_base != base_commit:
        raise RepositoryError(
            f"base commit changed identity: {base_commit} -> {actual_base}"
        )

    registered: dict[Path, str] = {}
    current_path: Path | None = None
    result = _run(["git", "worktree", "list", "--porcelain"], cwd=repository)
    for line in result.stdout.splitlines():
        if line.startswith("worktree "):
            current_path = Path(line.removeprefix("worktree ")).resolve()
            registered[current_path] = ""
        elif line.startswith("branch ") and current_path is not None:
            registered[current_path] = line.removeprefix("branch refs/heads/")

    if worktree in registered:
        if registered[worktree] != branch:
            raise RepositoryError(
                f"worktree {worktree} is on {registered[worktree]}, expected {branch}"
            )
        snapshot = inspect_worktree(repository, worktree=worktree, branch=branch)
        if snapshot["head_sha"] != base_commit or not snapshot["clean"]:
            raise RepositoryError(
                "partially prepared worktree does not match its base; inspect it before retrying"
            )
        return
    if worktree.exists():
        raise RepositoryError(
            f"unregistered story worktree path already exists: {worktree}"
        )
    branch_exists = _run(
        ["git", "show-ref", "--verify", "--quiet", f"refs/heads/{branch}"],
        cwd=repository,
        check=False,
    )
    if branch_exists.returncode == 0:
        raise RepositoryError(
            f"story branch exists without its expected worktree: {branch}"
        )

    worktree.parent.mkdir(parents=True, exist_ok=True)
    _run(
        ["git", "worktree", "add", "-b", branch, str(worktree), base_commit],
        cwd=repository,
    )


def inspect_worktree(
    repository: Path,
    *,
    worktree: Path,
    branch: str,
) -> dict[str, Any]:
    worktree = worktree.expanduser().resolve()
    if not worktree.is_dir():
        raise RepositoryError(f"candidate worktree does not exist: {worktree}")
    if git_common_dir(repository) != git_common_dir(worktree):
        raise RepositoryError(
            f"candidate worktree belongs to a different repository: {worktree}"
        )
    actual_branch = git_text(worktree, "branch", "--show-current")
    if not actual_branch:
        raise RepositoryError("candidate worktree is detached")
    if actual_branch != branch:
        raise RepositoryError(
            f"candidate branch mismatch: expected {branch}, worktree is {actual_branch}"
        )
    status = git_text(worktree, "status", "--porcelain")
    return {
        "worktree": str(worktree),
        "branch": actual_branch,
        "head_sha": git_text(worktree, "rev-parse", "HEAD"),
        "clean": not bool(status),
        "status": status,
    }


def inspect_candidate(repository: Path, payload: dict[str, Any]) -> dict[str, str]:
    worktree = Path(payload["worktree"]).expanduser().resolve()
    snapshot = inspect_worktree(repository, worktree=worktree, branch=payload["branch"])
    actual_sha = snapshot["head_sha"]
    if actual_sha != payload["head_sha"]:
        raise RepositoryError(
            f"stale candidate: reported {payload['head_sha']}, worktree is {actual_sha}"
        )
    if not snapshot["clean"]:
        raise RepositoryError(
            f"candidate worktree has uncommitted changes:\n{snapshot['status']}"
        )
    return {
        "worktree": str(worktree),
        "branch": snapshot["branch"],
        "head_sha": actual_sha,
    }


def validate_candidate_unchanged(repository: Path, candidate: dict[str, Any]) -> None:
    inspect_candidate(repository, candidate)


class ValidationRunner:
    """Runs only operator-supplied commands, never commands returned by a model."""

    def __init__(self, *, output_limit: int = 20_000) -> None:
        self.output_limit = output_limit

    def run(
        self,
        commands: list[str],
        *,
        cwd: Path,
        timeout_seconds: int,
    ) -> dict[str, Any]:
        results: list[dict[str, Any]] = []
        for command in commands:
            try:
                completed = subprocess.run(
                    ["/bin/sh", "-lc", command],
                    cwd=cwd,
                    check=False,
                    capture_output=True,
                    text=True,
                    timeout=timeout_seconds,
                    env=os.environ.copy(),
                )
                combined = "\n".join(
                    part for part in (completed.stdout, completed.stderr) if part
                )
                results.append(
                    {
                        "command": command,
                        "status": "PASSED" if completed.returncode == 0 else "FAILED",
                        "exit_code": completed.returncode,
                        "output": combined[-self.output_limit :],
                    }
                )
            except subprocess.TimeoutExpired as exc:
                output = "\n".join(
                    str(part) for part in (exc.stdout, exc.stderr) if part is not None
                )
                results.append(
                    {
                        "command": command,
                        "status": "TIMED_OUT",
                        "exit_code": -1,
                        "output": output[-self.output_limit :],
                    }
                )
        return {
            "overall_passed": all(result["status"] == "PASSED" for result in results),
            "results": results,
        }
