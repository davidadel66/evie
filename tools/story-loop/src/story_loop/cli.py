"""Command-line interface for starting, resuming, inspecting, and smoking a loop."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from collections.abc import Sequence
from pathlib import Path
from typing import Any

from .backend import CodexBackend
from .controller import Controller, new_state
from .repository import RepositoryError, git_common_dir, git_text, primary_worktree
from .storage import RunStore, StateError, validate_run_id


def _run_id_from_story(story: str) -> str:
    issue = re.search(r"(?:^|/)issues/(\d+)(?:$|[/?#])", story)
    if issue:
        return f"issue-{issue.group(1)}"
    slug = re.sub(r"[^a-z0-9]+", "-", story.lower()).strip("-")[:64]
    if not slug:
        raise StateError(
            "story reference does not produce a safe run ID; pass --run-id"
        )
    return slug


def _positive(value: str) -> int:
    parsed = int(value)
    if parsed < 1:
        raise argparse.ArgumentTypeError("must be at least 1")
    return parsed


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="evie-story-loop",
        description="Run one implementation task through fresh review and deterministic validation.",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    start = subparsers.add_parser("start", help="create and execute a new run")
    start.add_argument(
        "--story", required=True, help="approved story issue URL or stable reference"
    )
    start.add_argument(
        "--story-contract-file",
        help="approved contract text; otherwise load the GitHub issue with gh",
    )
    start.add_argument("--repo", default=".", help="path inside the target repository")
    start.add_argument("--base", default="master", help="pull-request base branch")
    start.add_argument(
        "--check",
        action="append",
        required=True,
        help="trusted validation command; repeat for multiple commands",
    )
    start.add_argument(
        "--check-timeout",
        type=_positive,
        default=1200,
        help="seconds allowed for each validation command",
    )
    start.add_argument("--max-review-passes", type=_positive, default=3)
    start.add_argument(
        "--run-id", help="durable run name; defaults from the story reference"
    )
    start.add_argument(
        "--draft-pr",
        action="store_true",
        help="push and open a draft PR only after the loop passes",
    )
    start.add_argument(
        "--model",
        default="gpt-5.6-terra",
        help="Codex model (default: gpt-5.6-terra)",
    )
    start.add_argument(
        "--review-skill",
        required=True,
        help="explicit path to the external review-story/SKILL.md contract",
    )

    resume = subparsers.add_parser("resume", help="continue a durable interrupted run")
    resume.add_argument("--repo", default=".", help="path inside the target repository")
    resume.add_argument("--run-id", required=True)

    status = subparsers.add_parser(
        "status", help="print durable state without running agents"
    )
    status.add_argument("--repo", default=".", help="path inside the target repository")
    status.add_argument("--run-id", required=True)

    smoke = subparsers.add_parser(
        "smoke", help="check SDK startup, auth, and a read-only structured turn"
    )
    smoke.add_argument("--repo", default=".", help="path inside the target repository")
    smoke.add_argument(
        "--model",
        default="gpt-5.6-terra",
        help="Codex model (default: gpt-5.6-terra)",
    )
    return parser


def _store(repository: Path, run_id: str) -> RunStore:
    return RunStore(git_common_dir(repository) / "story-loop", validate_run_id(run_id))


def _skill_path(argument: str, name: str) -> Path:
    path = Path(argument).expanduser()
    path = path.resolve()
    if not path.is_file():
        raise StateError(f"required skill is missing: {path}")
    return path


def _backend(config: dict[str, Any]) -> CodexBackend:
    return CodexBackend(
        repository=Path(config["repository"]),
        review_skill=Path(config["review_skill"]),
        model=config["model"] or None,
    )


def _story_contract(
    story: str, contract_file: str | None, repository: Path
) -> tuple[str, str]:
    if contract_file:
        path = Path(contract_file).expanduser().resolve()
        try:
            body = path.read_text(encoding="utf-8")
        except OSError as exc:
            raise StateError(f"cannot read story contract {path}: {exc}") from exc
        if not body.strip():
            raise StateError(f"story contract is empty: {path}")
        first_line = next(line.strip() for line in body.splitlines() if line.strip())
        return first_line.lstrip("# "), body

    result = subprocess.run(
        ["gh", "issue", "view", story, "--json", "title,body,url"],
        cwd=repository,
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        detail = (result.stderr or result.stdout).strip()
        raise StateError(
            "cannot load the approved story contract with gh; pass --story-contract-file"
            + (f": {detail}" if detail else "")
        )
    try:
        issue = json.loads(result.stdout)
        title = issue["title"]
        body = issue["body"]
        url = issue["url"]
    except (KeyError, TypeError, json.JSONDecodeError) as exc:
        raise StateError("gh returned an invalid story contract") from exc
    return title, f"# {title}\n\nSource: {url}\n\n{body}"


def _summary(state: dict[str, Any], store: RunStore) -> dict[str, Any]:
    return {
        "run_id": state["run_id"],
        "phase": state["phase"],
        "outcome": state["outcome"],
        "message": state["message"],
        "passes_completed": state["passes_completed"],
        "candidate_sha": state["candidate"].get("head_sha", ""),
        "worktree": state["candidate"].get("worktree", ""),
        "pr_url": state["pr_url"],
        "state_path": str(store.state_path),
        "iterations_path": str(store.iterations_directory),
    }


def _execute(store: RunStore, state: dict[str, Any]) -> dict[str, Any]:
    backend = _backend(state["config"])
    try:
        return Controller(store=store, backend=backend).run(state)
    finally:
        backend.close()


def _start(arguments: argparse.Namespace) -> tuple[dict[str, Any], RunStore]:
    repository = primary_worktree(Path(arguments.repo).expanduser().resolve())
    base_commit = git_text(
        repository, "rev-parse", "--verify", f"{arguments.base}^{{commit}}"
    )
    run_id = validate_run_id(arguments.run_id or _run_id_from_story(arguments.story))
    review_skill = _skill_path(arguments.review_skill, "review-story")
    story_title, story_contract = _story_contract(
        arguments.story, arguments.story_contract_file, repository
    )
    branch = f"codex/{run_id}"
    worktree = repository / ".worktrees" / run_id
    config = {
        "run_id": run_id,
        "story": arguments.story,
        "repository": str(repository),
        "base_branch": arguments.base,
        "base_commit": base_commit,
        "branch": branch,
        "worktree": str(worktree),
        "story_title": story_title,
        "story_contract": story_contract,
        "checks": list(arguments.check),
        "check_timeout_seconds": arguments.check_timeout,
        "max_review_passes": arguments.max_review_passes,
        "draft_pr": bool(arguments.draft_pr),
        "review_skill": str(review_skill),
        "model": arguments.model,
    }
    store = _store(repository, run_id)
    with store.lock():
        if store.exists():
            raise StateError(f"run already exists; use resume: {run_id}")
        state = new_state(config)
        store.save(state)
        return _execute(store, state), store


def _resume(arguments: argparse.Namespace) -> tuple[dict[str, Any], RunStore]:
    repository = primary_worktree(Path(arguments.repo).expanduser().resolve())
    store = _store(repository, arguments.run_id)
    with store.lock():
        state = store.load()
        configured_repository = Path(
            state.get("config", {}).get("repository", "")
        ).resolve()
        if configured_repository != repository:
            raise StateError(
                f"run belongs to {configured_repository}, not requested repository {repository}"
            )
        if state.get("phase") == "TERMINAL":
            return state, store
        return _execute(store, state), store


def _status(arguments: argparse.Namespace) -> tuple[dict[str, Any], RunStore]:
    repository = primary_worktree(Path(arguments.repo).expanduser().resolve())
    store = _store(repository, arguments.run_id)
    return store.load(), store


def _smoke(arguments: argparse.Namespace) -> dict[str, Any]:
    repository = primary_worktree(Path(arguments.repo).expanduser().resolve())
    backend = CodexBackend(
        repository=repository,
        review_skill=repository / ".agents" / "skills" / "review-story" / "SKILL.md",
        model=arguments.model,
    )
    try:
        return backend.smoke()
    finally:
        backend.close()


def main(argv: Sequence[str] | None = None) -> int:
    parser = build_parser()
    arguments = parser.parse_args(argv)
    try:
        if arguments.command == "start":
            state, store = _start(arguments)
            result = _summary(state, store)
            exit_code = 0 if state["outcome"] == "READY_FOR_HUMAN_REVIEW" else 2
        elif arguments.command == "resume":
            state, store = _resume(arguments)
            result = _summary(state, store)
            exit_code = 0 if state["outcome"] == "READY_FOR_HUMAN_REVIEW" else 2
        elif arguments.command == "status":
            state, store = _status(arguments)
            result = {"summary": _summary(state, store), "state": state}
            exit_code = 0
        else:
            result = _smoke(arguments)
            exit_code = 0
        print(json.dumps(result, indent=2, sort_keys=True))
        return exit_code
    except KeyboardInterrupt:
        print("story loop interrupted; rerun the resume command", file=sys.stderr)
        return 130
    except (StateError, RepositoryError, RuntimeError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
