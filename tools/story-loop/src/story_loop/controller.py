"""Deterministic state machine for one reviewed story delivery run."""

from __future__ import annotations

import hashlib
import json
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Protocol

from .contracts import ContractError, validate_implementation, validate_review
from .repository import (
    RepositoryError,
    ValidationRunner,
    inspect_candidate,
    inspect_worktree,
    prepare_story_worktree,
    validate_candidate_unchanged,
)
from .storage import RunStore, StateError


class Backend(Protocol):
    def start_implementation_thread(self, config: dict[str, Any]) -> str: ...

    def run_implementation(
        self, config: dict[str, Any], thread_id: str
    ) -> dict[str, Any]: ...

    def repair(
        self,
        config: dict[str, Any],
        thread_id: str,
        candidate: dict[str, Any],
        delta: list[dict[str, Any]],
    ) -> dict[str, Any]: ...

    def review(
        self,
        config: dict[str, Any],
        candidate: dict[str, Any],
    ) -> tuple[str, dict[str, Any]]: ...


PHASES = {
    "PREPARE",
    "START_IMPLEMENTATION",
    "IMPLEMENT",
    "REVIEW",
    "VALIDATE",
    "FINALIZE",
    "REPAIR",
    "DELIVER",
    "TERMINAL",
}


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def new_state(config: dict[str, Any]) -> dict[str, Any]:
    now = utc_now()
    return {
        "version": 1,
        "run_id": config["run_id"],
        "phase": "PREPARE",
        "config": config,
        "implementation_thread_id": "",
        "candidate": {},
        "pending_review_thread_id": "",
        "pending_review": {},
        "pending_validation": {},
        "pending_delta": [],
        "previous_delta_fingerprint": "",
        "passes_completed": 0,
        "last_repair": {},
        "outcome": "",
        "message": "",
        "pr_url": "",
        "created_at": now,
        "updated_at": now,
    }


def validate_state(state: dict[str, Any]) -> None:
    required = {
        "version",
        "run_id",
        "phase",
        "config",
        "implementation_thread_id",
        "candidate",
        "pending_review_thread_id",
        "pending_review",
        "pending_validation",
        "pending_delta",
        "previous_delta_fingerprint",
        "passes_completed",
        "last_repair",
        "outcome",
        "message",
        "pr_url",
        "created_at",
        "updated_at",
    }
    if set(state) != required:
        raise StateError(
            f"state keys mismatch; missing={sorted(required - set(state))}, "
            f"extra={sorted(set(state) - required)}"
        )
    if state["version"] != 1:
        raise StateError(f"unsupported state version: {state['version']}")
    if state["phase"] not in PHASES:
        raise StateError(f"unknown state phase: {state['phase']}")
    if not isinstance(state["config"], dict):
        raise StateError("state config must be an object")
    if state["run_id"] != state["config"].get("run_id"):
        raise StateError("state run ID differs from its immutable config")
    if isinstance(state["passes_completed"], bool) or not isinstance(
        state["passes_completed"], int
    ):
        raise StateError("passes_completed must be an integer")
    if state["passes_completed"] < 0:
        raise StateError("passes_completed must not be negative")
    if (
        state["phase"] not in {"PREPARE", "START_IMPLEMENTATION", "TERMINAL"}
        and not state["implementation_thread_id"]
    ):
        raise StateError("non-initial state is missing its implementation thread ID")


class Controller:
    def __init__(
        self,
        *,
        store: RunStore,
        backend: Backend,
        validation_runner: ValidationRunner | None = None,
    ) -> None:
        self.store = store
        self.backend = backend
        self.validation_runner = validation_runner or ValidationRunner()

    def run(self, state: dict[str, Any]) -> dict[str, Any]:
        validate_state(state)
        while state["phase"] != "TERMINAL":
            phase = state["phase"]
            try:
                if phase == "IMPLEMENT":
                    self._implement(state)
                elif phase == "PREPARE":
                    self._prepare(state)
                elif phase == "START_IMPLEMENTATION":
                    self._start_implementation(state)
                elif phase == "REVIEW":
                    self._review(state)
                elif phase == "VALIDATE":
                    self._validate(state)
                elif phase == "FINALIZE":
                    self._finalize(state)
                elif phase == "REPAIR":
                    self._repair(state)
                elif phase == "DELIVER":
                    self._deliver(state)
                else:
                    raise StateError(f"cannot execute phase: {phase}")
            except KeyboardInterrupt:
                raise
            except ContractError as exc:
                outcome = (
                    "REVIEW_INCOMPLETE"
                    if phase == "REVIEW"
                    else "IMPLEMENTATION_INCOMPLETE"
                )
                self._stop(state, outcome, str(exc))
            except RepositoryError as exc:
                self._stop(state, "INVALID_CANDIDATE", str(exc))
            # SDK transport failures and delivery subprocess errors must be
            # converted into a durable safe stop before the process exits.
            except Exception as exc:  # noqa: BLE001
                if phase == "REVIEW":
                    outcome = "REVIEW_INCOMPLETE"
                elif phase in {"START_IMPLEMENTATION", "IMPLEMENT", "REPAIR"}:
                    outcome = "IMPLEMENTATION_FAILED"
                elif phase == "DELIVER":
                    outcome = "DELIVERY_FAILED"
                else:
                    outcome = "LOOP_FAILED"
                self._stop(state, outcome, f"{type(exc).__name__}: {exc}")
            validate_state(state)
        return state

    def _save(self, state: dict[str, Any]) -> None:
        state["updated_at"] = utc_now()
        self.store.save(state)

    def _stop(self, state: dict[str, Any], outcome: str, message: str) -> None:
        state["phase"] = "TERMINAL"
        state["outcome"] = outcome
        state["message"] = message
        self._save(state)

    def _prepare(self, state: dict[str, Any]) -> None:
        config = state["config"]
        prepare_story_worktree(
            Path(config["repository"]),
            worktree=Path(config["worktree"]),
            branch=config["branch"],
            base_commit=config["base_commit"],
        )
        state["phase"] = "START_IMPLEMENTATION"
        self._save(state)

    def _start_implementation(self, state: dict[str, Any]) -> None:
        thread_id = self.backend.start_implementation_thread(state["config"])
        if not isinstance(thread_id, str) or not thread_id.strip():
            raise ContractError("implementation backend returned an invalid thread ID")
        state["implementation_thread_id"] = thread_id
        state["phase"] = "IMPLEMENT"
        self._save(state)

    @staticmethod
    def _recovered_candidate(
        state: dict[str, Any], *, previous_sha: str
    ) -> dict[str, Any] | None:
        config = state["config"]
        snapshot = inspect_worktree(
            Path(config["repository"]),
            worktree=Path(config["worktree"]),
            branch=config["branch"],
        )
        if snapshot["head_sha"] == previous_sha or not snapshot["clean"]:
            return None
        return {
            "status": "CANDIDATE_READY",
            "story_ref": config["story"],
            "title": config["story_title"],
            "worktree": config["worktree"],
            "branch": config["branch"],
            "base_branch": config["base_branch"],
            "head_sha": snapshot["head_sha"],
            "pr_url": "",
            "summary": [
                "Recovered a clean advanced commit after an interrupted write turn."
            ],
            "checks": [],
            "known_gaps": [
                "The interrupted agent turn did not return its structured summary."
            ],
            "decision": "",
        }

    def _accept_candidate(
        self,
        state: dict[str, Any],
        payload: dict[str, Any],
        *,
        previous_sha: str = "",
    ) -> None:
        config = state["config"]
        if payload["story_ref"] != config["story"]:
            raise ContractError(
                f"candidate story mismatch: expected {config['story']}, got {payload['story_ref']}"
            )
        if payload["base_branch"] != config["base_branch"]:
            raise ContractError(
                f"candidate base mismatch: expected {config['base_branch']}, got {payload['base_branch']}"
            )
        if Path(payload["worktree"]).resolve() != Path(config["worktree"]).resolve():
            raise ContractError(
                f"candidate worktree mismatch: expected {config['worktree']}, got {payload['worktree']}"
            )
        if payload["branch"] != config["branch"]:
            raise ContractError(
                f"candidate branch mismatch: expected {config['branch']}, got {payload['branch']}"
            )
        inspected = inspect_candidate(Path(config["repository"]), payload)
        if previous_sha and inspected["head_sha"] == previous_sha:
            raise RepositoryError("repair did not produce a new candidate commit")
        candidate = dict(payload)
        candidate.update(inspected)
        state["candidate"] = candidate

    def _implement(self, state: dict[str, Any]) -> None:
        recovered = self._recovered_candidate(
            state, previous_sha=state["config"]["base_commit"]
        )
        if recovered is not None:
            self._accept_candidate(state, recovered)
            state["phase"] = "REVIEW"
            self._save(state)
            return
        raw_payload = self.backend.run_implementation(
            state["config"], state["implementation_thread_id"]
        )
        payload = validate_implementation(raw_payload)
        if payload["status"] == "DECISION_REQUIRED":
            state["last_repair"] = payload
            self._stop(state, "DECISION_REQUIRED", payload["decision"])
            return
        if payload["status"] == "IMPLEMENTATION_INCOMPLETE":
            state["last_repair"] = payload
            self._stop(state, "IMPLEMENTATION_INCOMPLETE", payload["decision"])
            return
        self._accept_candidate(state, payload)
        state["phase"] = "REVIEW"
        self._save(state)

    def _review(self, state: dict[str, Any]) -> None:
        config = state["config"]
        candidate = state["candidate"]
        validate_candidate_unchanged(Path(config["repository"]), candidate)
        review_config = dict(config)
        review_config["passes_completed"] = state["passes_completed"]
        thread_id, raw_review = self.backend.review(review_config, candidate)
        review = validate_review(raw_review)
        if review["candidate_sha"] != candidate["head_sha"]:
            raise RepositoryError(
                f"review is stale: expected {candidate['head_sha']}, got {review['candidate_sha']}"
            )
        validate_candidate_unchanged(Path(config["repository"]), candidate)
        state["pending_review_thread_id"] = thread_id
        state["pending_review"] = review
        state["phase"] = "VALIDATE"
        self._save(state)

    def _validate(self, state: dict[str, Any]) -> None:
        config = state["config"]
        candidate = state["candidate"]
        repository = Path(config["repository"])
        validate_candidate_unchanged(repository, candidate)
        validation = self.validation_runner.run(
            config["checks"],
            cwd=Path(candidate["worktree"]),
            timeout_seconds=config["check_timeout_seconds"],
        )
        validate_candidate_unchanged(repository, candidate)
        state["pending_validation"] = validation
        state["phase"] = "FINALIZE"
        self._save(state)

    @staticmethod
    def _remaining_delta(
        review: dict[str, Any], validation: dict[str, Any]
    ) -> list[dict[str, Any]]:
        delta: list[dict[str, Any]] = []
        if review["verdict"] == "CHANGES_REQUIRED":
            for finding in review["findings"]:
                delta.append({"source": "review", **finding})
        for result in validation["results"]:
            if result["status"] != "PASSED":
                delta.append(
                    {
                        "source": "validation",
                        "command": result["command"],
                        "status": result["status"],
                        "exit_code": result["exit_code"],
                        "output": result["output"],
                    }
                )
        return delta

    @staticmethod
    def _delta_fingerprint(delta: list[dict[str, Any]]) -> str:
        semantic: list[dict[str, Any]] = []
        for item in delta:
            if item["source"] == "review":
                semantic.append(
                    {
                        key: item[key]
                        for key in (
                            "source",
                            "id",
                            "priority",
                            "title",
                            "body",
                            "file",
                            "start",
                            "end",
                        )
                    }
                )
            else:
                semantic.append(
                    {
                        "source": item["source"],
                        "command": item["command"],
                        "status": item["status"],
                        "exit_code": item["exit_code"],
                        "output": re.sub(r"\s+", " ", item["output"]).strip(),
                    }
                )
        encoded = json.dumps(semantic, sort_keys=True, separators=(",", ":")).encode()
        return hashlib.sha256(encoded).hexdigest()

    def _finalize(self, state: dict[str, Any]) -> None:
        review = state["pending_review"]
        validation = state["pending_validation"]
        delta = self._remaining_delta(review, validation)
        next_pass = state["passes_completed"] + 1
        config = state["config"]

        if review["verdict"] == "DECISION_REQUIRED":
            decision = "DECISION_REQUIRED"
        elif review["verdict"] == "REVIEW_INCOMPLETE":
            decision = "REVIEW_INCOMPLETE"
        elif (
            review["verdict"] == "READY_FOR_HUMAN_REVIEW"
            and validation["overall_passed"]
        ):
            decision = "READY_FOR_HUMAN_REVIEW"
        else:
            fingerprint = self._delta_fingerprint(delta)
            if fingerprint == state["previous_delta_fingerprint"]:
                decision = "STALLED"
            elif next_pass >= config["max_review_passes"]:
                decision = "MAX_PASSES"
            else:
                decision = "REPAIR"

        record = {
            "iteration": next_pass,
            "candidate": state["candidate"],
            "review_thread_id": state["pending_review_thread_id"],
            "review": review,
            "validation": validation,
            "remaining_delta": delta,
            "controller_decision": decision,
            "recorded_at": state["updated_at"],
        }
        self.store.write_iteration(next_pass, record)
        state["passes_completed"] = next_pass

        if decision == "DECISION_REQUIRED" or decision == "REVIEW_INCOMPLETE":
            self._stop(state, decision, review["decision"])
        elif decision == "READY_FOR_HUMAN_REVIEW":
            if config["draft_pr"]:
                state["phase"] = "DELIVER"
                self._save(state)
            else:
                self._stop(
                    state,
                    decision,
                    "candidate passed fresh review and deterministic validation",
                )
        elif decision == "STALLED":
            self._stop(
                state, decision, "remaining delta did not change after a repair pass"
            )
        elif decision == "MAX_PASSES":
            self._stop(state, decision, "configured review-pass limit reached")
        else:
            state["previous_delta_fingerprint"] = self._delta_fingerprint(delta)
            state["pending_delta"] = delta
            state["phase"] = "REPAIR"
            self._save(state)

    def _repair(self, state: dict[str, Any]) -> None:
        config = state["config"]
        candidate = state["candidate"]
        recovered = self._recovered_candidate(state, previous_sha=candidate["head_sha"])
        if recovered is not None:
            state["last_repair"] = recovered
            self._accept_candidate(state, recovered, previous_sha=candidate["head_sha"])
            state["pending_review_thread_id"] = ""
            state["pending_review"] = {}
            state["pending_validation"] = {}
            state["pending_delta"] = []
            state["phase"] = "REVIEW"
            self._save(state)
            return
        raw_payload = self.backend.repair(
            config,
            state["implementation_thread_id"],
            candidate,
            state["pending_delta"],
        )
        payload = validate_implementation(raw_payload)
        state["last_repair"] = payload
        if payload["status"] == "DECISION_REQUIRED":
            self._stop(state, "DECISION_REQUIRED", payload["decision"])
            return
        if payload["status"] == "IMPLEMENTATION_INCOMPLETE":
            self._stop(state, "IMPLEMENTATION_INCOMPLETE", payload["decision"])
            return
        previous_sha = candidate["head_sha"]
        self._accept_candidate(state, payload, previous_sha=previous_sha)
        state["pending_review_thread_id"] = ""
        state["pending_review"] = {}
        state["pending_validation"] = {}
        state["pending_delta"] = []
        state["phase"] = "REVIEW"
        self._save(state)

    def _deliver(self, state: dict[str, Any]) -> None:
        config = state["config"]
        candidate = state["candidate"]
        repository = Path(config["repository"])
        validate_candidate_unchanged(repository, candidate)
        worktree = Path(candidate["worktree"])
        branch = candidate["branch"]

        subprocess.run(
            ["git", "push", "--set-upstream", "origin", branch],
            cwd=worktree,
            check=True,
            text=True,
        )
        existing = subprocess.run(
            [
                "gh",
                "pr",
                "view",
                branch,
                "--json",
                "url,state,isDraft,baseRefName,headRefName,headRefOid",
            ],
            cwd=worktree,
            check=False,
            capture_output=True,
            text=True,
        )
        if existing.returncode == 0:
            pr_url = self._validated_pr_url(state, existing.stdout)
        else:
            summary = (
                "\n".join(f"- {item}" for item in candidate["summary"])
                or "- Candidate produced by the persistent implementation worker."
            )
            checks = "\n".join(
                f"- `{command}` — passed" for command in config["checks"]
            )
            gaps = "\n".join(f"- {item}" for item in candidate["known_gaps"])
            if not gaps:
                gaps = "- No known gaps reported."
            body = "\n".join(
                [
                    f"Closes {config['story']}",
                    "",
                    "## Summary",
                    "",
                    summary,
                    "",
                    "## Verification",
                    "",
                    checks,
                    "",
                    f"Fresh review passes: {state['passes_completed']}",
                    f"Candidate: `{candidate['head_sha']}`",
                    f"Audit receipts: `{self.store.iterations_directory}`",
                    "",
                    "## Known gaps and risks",
                    "",
                    gaps,
                    "- Human approval is still required; this controller never approves or merges.",
                ]
            )
            created = subprocess.run(
                [
                    "gh",
                    "pr",
                    "create",
                    "--draft",
                    "--base",
                    config["base_branch"],
                    "--head",
                    branch,
                    "--title",
                    candidate["title"],
                    "--body",
                    body,
                ],
                cwd=worktree,
                check=True,
                capture_output=True,
                text=True,
            )
            if not created.stdout.strip():
                raise RuntimeError("gh pr create did not return a pull-request URL")
            confirmed = subprocess.run(
                [
                    "gh",
                    "pr",
                    "view",
                    branch,
                    "--json",
                    "url,state,isDraft,baseRefName,headRefName,headRefOid",
                ],
                cwd=worktree,
                check=True,
                capture_output=True,
                text=True,
            )
            pr_url = self._validated_pr_url(state, confirmed.stdout)
        state["pr_url"] = pr_url
        self._stop(
            state,
            "READY_FOR_HUMAN_REVIEW",
            f"draft pull request ready for human review: {pr_url}",
        )

    @staticmethod
    def _validated_pr_url(state: dict[str, Any], raw: str) -> str:
        try:
            payload = json.loads(raw)
            values = {
                key: payload[key]
                for key in (
                    "url",
                    "state",
                    "isDraft",
                    "baseRefName",
                    "headRefName",
                    "headRefOid",
                )
            }
        except (KeyError, TypeError, json.JSONDecodeError) as exc:
            raise RuntimeError("gh returned incomplete pull-request metadata") from exc
        config = state["config"]
        candidate = state["candidate"]
        expected = {
            "state": "OPEN",
            "isDraft": True,
            "baseRefName": config["base_branch"],
            "headRefName": candidate["branch"],
            "headRefOid": candidate["head_sha"],
        }
        mismatches = {
            key: {"expected": expected_value, "actual": values[key]}
            for key, expected_value in expected.items()
            if values[key] != expected_value
        }
        if mismatches:
            raise RuntimeError(
                f"existing pull request does not match authorized draft handoff: {mismatches}"
            )
        if not isinstance(values["url"], str) or not values["url"].strip():
            raise RuntimeError("pull-request URL is missing")
        return values["url"]
