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


def inspect_candidate(repository: Path, payload: dict[str, Any]) -> dict[str, str]:
    worktree = Path(payload["worktree"]).expanduser().resolve()
    if not worktree.is_dir():
        raise RepositoryError(f"candidate worktree does not exist: {worktree}")
    if git_common_dir(repository) != git_common_dir(worktree):
        raise RepositoryError(
            f"candidate worktree belongs to a different repository: {worktree}"
        )

    actual_sha = git_text(worktree, "rev-parse", "HEAD")
    if actual_sha != payload["head_sha"]:
        raise RepositoryError(
            f"stale candidate: reported {payload['head_sha']}, worktree is {actual_sha}"
        )
    actual_branch = git_text(worktree, "branch", "--show-current")
    if not actual_branch:
        raise RepositoryError("candidate worktree is detached")
    if actual_branch != payload["branch"]:
        raise RepositoryError(
            f"candidate branch mismatch: reported {payload['branch']}, worktree is {actual_branch}"
        )
    status = git_text(worktree, "status", "--porcelain")
    if status:
        raise RepositoryError(f"candidate worktree has uncommitted changes:\n{status}")
    return {
        "worktree": str(worktree),
        "branch": actual_branch,
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
