from __future__ import annotations

import contextlib
import hashlib
import io
import json
import shlex
import subprocess
import tempfile
import time
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from story_loop import cli
from story_loop.backend import CodexBackend
from story_loop.contracts import (
    IMPLEMENTATION_SCHEMA,
    REVIEW_SCHEMA,
    ContractError,
    validate_review,
)
from story_loop.controller import Controller, new_state, validate_state
from story_loop.prompts import REVIEW_COORDINATOR_CONTRACT
from story_loop.repository import ValidationRunner
from story_loop.storage import RunLockedError, RunStore, StateError


def git(repository: Path, *arguments: str) -> str:
    result = subprocess.run(
        ["git", *arguments],
        cwd=repository,
        check=True,
        capture_output=True,
        text=True,
    )
    return result.stdout.strip()


def commit(repository: Path, filename: str, content: str, message: str) -> str:
    (repository / filename).write_text(content, encoding="utf-8")
    git(repository, "add", filename)
    git(repository, "commit", "-m", message)
    return git(repository, "rev-parse", "HEAD")


def make_repository(parent: Path) -> Path:
    repository = parent / "repo"
    repository.mkdir()
    git(repository, "init", "-b", "master")
    git(repository, "config", "user.name", "Story Loop Tests")
    git(repository, "config", "user.email", "story-loop@example.invalid")
    commit(repository, "base.txt", "base\n", "base")
    return repository


def implementation_payload(config: dict, worktree: Path) -> dict:
    return {
        "status": "CANDIDATE_READY",
        "story_ref": config["story"],
        "title": "Test story",
        "worktree": str(worktree),
        "branch": git(worktree, "branch", "--show-current"),
        "base_branch": config["base_branch"],
        "head_sha": git(worktree, "rev-parse", "HEAD"),
        "pr_url": "",
        "summary": ["implemented test candidate"],
        "checks": [{"command": "true", "status": "PASSED", "evidence": "exit 0"}],
        "known_gaps": [],
        "decision": "",
    }


def finding(identifier: str = "F1") -> dict:
    return {
        "id": identifier,
        "priority": 2,
        "title": "Repair the boundary",
        "body": "The candidate does not preserve the required boundary.",
        "file": "story.txt",
        "start": 1,
        "end": 1,
    }


def review_payload(
    sha: str,
    verdict: str,
    *,
    findings: list[dict] | None = None,
    criteria: list[str] | None = None,
) -> dict:
    decision = ""
    if verdict == "DECISION_REQUIRED":
        decision = "Choose whether this behavior is part of the story."
    elif verdict == "REVIEW_INCOMPLETE":
        decision = "Required verification could not run."
    return {
        "candidate_sha": sha,
        "verdict": verdict,
        "summary": f"review result: {verdict}",
        "lenses": [
            {
                "name": name,
                "completed": True,
                "summary": f"{name} lens completed",
                "gaps": [],
            }
            for name in ("contract", "correctness", "maintainability")
        ],
        "acceptance_coverage": [
            {
                "criterion": criterion,
                "status": "COVERED",
                "evidence": "candidate and deterministic evidence inspected",
            }
            for criterion in (criteria or ["Approved test criterion."])
        ],
        "findings": list(findings or []),
        "checks": [{"command": "true", "status": "PASSED", "evidence": "exit 0"}],
        "decision": decision,
    }


class FakeBackend:
    def __init__(self, repository: Path, reviews: list[object]) -> None:
        self.repository = repository
        self.reviews = list(reviews)
        self.implementation_calls = 0
        self.implementation_thread_calls = 0
        self.review_calls = 0
        self.repair_calls = 0
        self.review_thread_ids: list[str] = []
        self.repair_deltas: list[list[dict]] = []
        self.repair_callback = None
        self.last_completed: tuple[str, dict] | None = None
        self.closed = False

    def start_implementation_thread(self, _config: dict) -> str:
        self.implementation_thread_calls += 1
        return "implementation-thread"

    def run_implementation(self, config: dict, thread_id: str) -> tuple[str, dict]:
        self.implementation_calls += 1
        if thread_id != "implementation-thread":
            raise AssertionError("implementation did not use its persisted thread")
        worktree = Path(config["worktree"])
        commit(worktree, "story.txt", "candidate\n", "candidate")
        self.last_completed = (
            "implementation-turn-1",
            implementation_payload(config, worktree),
        )
        return self.last_completed

    def recover_implementation(
        self, _config: dict, _thread_id: str, after_turn_id: str
    ) -> tuple[str, dict] | None:
        if self.last_completed is None or self.last_completed[0] == after_turn_id:
            return None
        return self.last_completed

    def review(self, config: dict, candidate: dict) -> tuple[str, dict]:
        self.review_calls += 1
        item = self.reviews.pop(0)
        if isinstance(item, BaseException):
            raise item
        thread_id = f"review-thread-{self.review_calls}"
        self.review_thread_ids.append(thread_id)
        if callable(item):
            payload = item(candidate)
        else:
            payload = item
        return thread_id, payload

    def repair(
        self,
        config: dict,
        thread_id: str,
        candidate: dict,
        delta: list[dict],
    ) -> tuple[str, dict]:
        self.repair_calls += 1
        self.repair_deltas.append(delta)
        if thread_id != "implementation-thread":
            raise AssertionError("repair did not resume the implementation thread")
        worktree = Path(candidate["worktree"])
        if self.repair_callback is None:
            commit(worktree, f"repair-{self.repair_calls}.txt", "fixed\n", "repair")
        else:
            self.repair_callback(self.repair_calls, delta)
        self.last_completed = (
            f"repair-turn-{self.repair_calls}",
            implementation_payload(config, worktree),
        )
        return self.last_completed

    def close(self) -> None:
        self.closed = True


class ControllerTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.repository = make_repository(self.root)

    def config(self, **overrides: object) -> dict:
        base_commit = git(self.repository, "rev-parse", "master")
        value = {
            "run_id": "test-run",
            "story": "https://example.invalid/issues/1",
            "repository": str(self.repository),
            "base_branch": "master",
            "base_commit": base_commit,
            "branch": "codex/test-run",
            "worktree": str(self.repository / ".worktrees" / "test-run"),
            "story_title": "Test story",
            "story_contract": "# Test story\n\n## Acceptance criteria\n\n- Approved test criterion.",
            "acceptance_criteria": ["Approved test criterion."],
            "review_contract": REVIEW_COORDINATOR_CONTRACT,
            "review_contract_sha256": hashlib.sha256(
                REVIEW_COORDINATOR_CONTRACT.encode()
            ).hexdigest(),
            "checks": ["true"],
            "check_timeout_seconds": 10,
            "max_review_passes": 3,
            "draft_pr": False,
            "model": "",
        }
        value.update(overrides)
        return value

    def run_controller(
        self, backend: FakeBackend, config: dict | None = None
    ) -> tuple[dict, RunStore]:
        state = new_state(config or self.config())
        store = RunStore(self.root / "state", state["run_id"])
        store.save(state)
        result = Controller(store=store, backend=backend).run(state)
        return result, store

    def test_happy_path_writes_ready_receipt(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                )
            ],
        )
        state, store = self.run_controller(backend)

        self.assertEqual("READY_FOR_HUMAN_REVIEW", state["outcome"])
        self.assertEqual(1, state["passes_completed"])
        self.assertEqual(1, backend.implementation_calls)
        self.assertEqual(["review-thread-1"], backend.review_thread_ids)
        receipt = json.loads((store.iterations_directory / "001.json").read_text())
        self.assertEqual("READY_FOR_HUMAN_REVIEW", receipt["controller_decision"])
        self.assertTrue(receipt["validation"]["overall_passed"])

    def test_review_finding_repairs_in_same_thread_then_uses_fresh_reviewer(
        self,
    ) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "CHANGES_REQUIRED", findings=[finding()]
                ),
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                ),
            ],
        )
        state, _store = self.run_controller(backend)

        self.assertEqual("READY_FOR_HUMAN_REVIEW", state["outcome"])
        self.assertEqual(2, state["passes_completed"])
        self.assertEqual(1, backend.repair_calls)
        self.assertEqual(
            ["review-thread-1", "review-thread-2"], backend.review_thread_ids
        )
        self.assertEqual("review", backend.repair_deltas[0][0]["source"])

    def test_validation_failure_becomes_repair_feedback(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                ),
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                ),
            ],
        )

        def repair(_number: int, _delta: list[dict]) -> None:
            commit(
                Path(self.config()["worktree"]),
                "marker",
                "present\n",
                "make validation pass",
            )

        backend.repair_callback = repair
        state, _store = self.run_controller(
            backend, self.config(checks=["test -f marker"])
        )

        self.assertEqual("READY_FOR_HUMAN_REVIEW", state["outcome"])
        self.assertEqual("validation", backend.repair_deltas[0][0]["source"])
        self.assertEqual("test -f marker", backend.repair_deltas[0][0]["command"])

    def test_changed_validation_evidence_is_not_stalled(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                ),
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                ),
            ],
        )

        def repair(_number: int, _delta: list[dict]) -> None:
            commit(
                Path(self.config()["worktree"]),
                "story.txt",
                "different failure\n",
                "advance failing evidence",
            )

        backend.repair_callback = repair
        state, _store = self.run_controller(
            backend,
            self.config(
                checks=["cat story.txt; exit 1"],
                max_review_passes=2,
            ),
        )
        self.assertEqual("MAX_PASSES", state["outcome"])

    def test_identical_validation_evidence_stops_as_stalled(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                ),
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                ),
            ],
        )
        state, _store = self.run_controller(
            backend, self.config(checks=["printf same; exit 1"])
        )
        self.assertEqual("STALLED", state["outcome"])

    def test_decision_required_stops_after_recording_validation(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "DECISION_REQUIRED"
                )
            ],
        )
        state, store = self.run_controller(backend)
        self.assertEqual("DECISION_REQUIRED", state["outcome"])
        self.assertTrue((store.iterations_directory / "001.json").is_file())
        self.assertEqual(0, backend.repair_calls)

    def test_incomplete_review_stops(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "REVIEW_INCOMPLETE"
                )
            ],
        )
        state, _store = self.run_controller(backend)
        self.assertEqual("REVIEW_INCOMPLETE", state["outcome"])

    def test_maximum_passes_stops_before_repair(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "CHANGES_REQUIRED", findings=[finding()]
                )
            ],
        )
        state, _store = self.run_controller(backend, self.config(max_review_passes=1))
        self.assertEqual("MAX_PASSES", state["outcome"])
        self.assertEqual(0, backend.repair_calls)

    def test_unchanged_delta_stops_as_stalled(self) -> None:
        reworded = finding("F2")
        reworded["title"] = "Same boundary, different wording"
        reworded["body"] = "Fresh review described the same location differently."
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "CHANGES_REQUIRED", findings=[finding()]
                ),
                lambda candidate: review_payload(
                    candidate["head_sha"],
                    "CHANGES_REQUIRED",
                    findings=[reworded],
                ),
            ],
        )
        state, _store = self.run_controller(backend)
        self.assertEqual("STALLED", state["outcome"])
        self.assertEqual(1, backend.repair_calls)

    def test_delta_fingerprint_ignores_fresh_review_wording_and_order(self) -> None:
        first = finding("F1")
        second = finding("F2")
        second["file"] = "other.go"
        second["start"] = 20
        second["end"] = 21
        original = [{"source": "review", **first}, {"source": "review", **second}]
        reworded = [
            {
                "source": "review",
                **second,
                "id": "NEW-2",
                "title": "Rephrased second finding",
                "body": "Different prose for the same location.",
            },
            {
                "source": "review",
                **first,
                "id": "NEW-1",
                "title": "Rephrased first finding",
                "body": "Another description for the same location.",
            },
        ]
        self.assertEqual(
            Controller._delta_fingerprint(original),
            Controller._delta_fingerprint(reworded),
        )

    def test_stale_review_candidate_stops(self) -> None:
        stale_sha = "0" * 40
        backend = FakeBackend(
            self.repository,
            [review_payload(stale_sha, "READY_FOR_HUMAN_REVIEW")],
        )
        state, _store = self.run_controller(backend)
        self.assertEqual("INVALID_CANDIDATE", state["outcome"])

    def test_stale_implementation_candidate_stops(self) -> None:
        backend = FakeBackend(self.repository, [])

        def stale(config: dict, _thread_id: str) -> tuple[str, dict]:
            worktree = Path(config["worktree"])
            commit(worktree, "story.txt", "candidate\n", "candidate")
            payload = implementation_payload(config, worktree)
            payload["head_sha"] = "0" * 40
            return "stale-turn", payload

        backend.run_implementation = stale  # type: ignore[method-assign]
        state, _store = self.run_controller(backend)
        self.assertEqual("INVALID_CANDIDATE", state["outcome"])

    def test_corrupt_nested_config_is_rejected_before_execution(self) -> None:
        state = new_state(self.config())
        state["config"]["acceptance_criteria"] = []
        with self.assertRaises(StateError):
            validate_state(state)

    def test_invalid_structured_review_stops_safely(self) -> None:
        backend = FakeBackend(self.repository, [{"verdict": "READY_FOR_HUMAN_REVIEW"}])
        state, _store = self.run_controller(backend)
        self.assertEqual("REVIEW_INCOMPLETE", state["outcome"])
        self.assertEqual(0, state["passes_completed"])

    def test_ready_review_with_failed_check_stops_safely(self) -> None:
        def contradictory(candidate: dict) -> dict:
            payload = review_payload(candidate["head_sha"], "READY_FOR_HUMAN_REVIEW")
            payload["checks"][0]["status"] = "FAILED"
            return payload

        backend = FakeBackend(self.repository, [contradictory])
        state, _store = self.run_controller(backend)
        self.assertEqual("REVIEW_INCOMPLETE", state["outcome"])

    def test_review_must_cover_exact_configured_acceptance_criteria(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"],
                    "READY_FOR_HUMAN_REVIEW",
                    criteria=["A different criterion."],
                )
            ],
        )
        state, _store = self.run_controller(backend)
        self.assertEqual("REVIEW_INCOMPLETE", state["outcome"])

    def test_candidate_must_descend_from_exact_configured_base(self) -> None:
        backend = FakeBackend(self.repository, [])

        def unrelated(config: dict, _thread_id: str) -> tuple[str, dict]:
            backend.implementation_calls += 1
            worktree = Path(config["worktree"])
            empty_tree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
            unrelated_sha = git(worktree, "commit-tree", empty_tree, "-m", "unrelated")
            git(worktree, "reset", "--hard", unrelated_sha)
            payload = implementation_payload(config, worktree)
            backend.last_completed = ("unrelated-turn", payload)
            return backend.last_completed

        backend.run_implementation = unrelated  # type: ignore[method-assign]
        state, _store = self.run_controller(backend)
        self.assertEqual("INVALID_CANDIDATE", state["outcome"])
        self.assertIn("does not descend", state["message"])

    def test_resume_recovers_commit_from_interrupted_implementation_turn(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                )
            ],
        )
        original = backend.run_implementation

        def interrupted(config: dict, thread_id: str) -> tuple[str, dict]:
            original(config, thread_id)
            raise KeyboardInterrupt()

        backend.run_implementation = interrupted  # type: ignore[method-assign]
        config = self.config()
        store = RunStore(self.root / "state", config["run_id"])
        state = new_state(config)
        store.save(state)
        controller = Controller(store=store, backend=backend)
        with self.assertRaises(KeyboardInterrupt):
            controller.run(state)

        resumed = store.load()
        self.assertEqual("IMPLEMENT", resumed["phase"])
        self.assertEqual("implementation-thread", resumed["implementation_thread_id"])
        backend.run_implementation = original  # type: ignore[method-assign]
        final = controller.run(resumed)
        self.assertEqual("READY_FOR_HUMAN_REVIEW", final["outcome"])
        self.assertEqual(1, backend.implementation_thread_calls)
        self.assertEqual(1, backend.implementation_calls)

    def test_resume_recovers_non_commit_decision_from_thread_history(self) -> None:
        backend = FakeBackend(self.repository, [])

        def interrupted_decision(config: dict, _thread_id: str) -> tuple[str, dict]:
            backend.implementation_calls += 1
            payload = {
                "status": "DECISION_REQUIRED",
                "story_ref": config["story"],
                "title": config["story_title"],
                "worktree": "",
                "branch": "",
                "base_branch": "",
                "head_sha": "",
                "pr_url": "",
                "summary": [],
                "checks": [],
                "known_gaps": [],
                "decision": "Choose the public behavior before implementation.",
            }
            backend.last_completed = ("decision-turn", payload)
            raise KeyboardInterrupt()

        backend.run_implementation = interrupted_decision  # type: ignore[method-assign]
        config = self.config()
        store = RunStore(self.root / "state", config["run_id"])
        state = new_state(config)
        store.save(state)
        controller = Controller(store=store, backend=backend)
        with self.assertRaises(KeyboardInterrupt):
            controller.run(state)

        resumed = store.load()
        self.assertEqual("IMPLEMENT", resumed["phase"])
        final = controller.run(resumed)
        self.assertEqual("DECISION_REQUIRED", final["outcome"])
        self.assertEqual("decision-turn", final["last_implementation_turn_id"])
        self.assertEqual(1, backend.implementation_calls)

    def test_resume_recovers_commit_from_interrupted_repair_turn(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                lambda candidate: review_payload(
                    candidate["head_sha"], "CHANGES_REQUIRED", findings=[finding()]
                ),
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                ),
            ],
        )

        def interrupted(_number: int, _delta: list[dict]) -> None:
            commit(
                Path(self.config()["worktree"]),
                "repair.txt",
                "fixed\n",
                "interrupted repair",
            )
            raise KeyboardInterrupt()

        backend.repair_callback = interrupted
        config = self.config()
        store = RunStore(self.root / "state", config["run_id"])
        state = new_state(config)
        store.save(state)
        controller = Controller(store=store, backend=backend)
        with self.assertRaises(KeyboardInterrupt):
            controller.run(state)

        resumed = store.load()
        self.assertEqual("REPAIR", resumed["phase"])
        backend.repair_callback = None
        final = controller.run(resumed)
        self.assertEqual("READY_FOR_HUMAN_REVIEW", final["outcome"])
        self.assertEqual(1, backend.repair_calls)

    def test_resume_does_not_repeat_completed_implementation(self) -> None:
        backend = FakeBackend(
            self.repository,
            [
                KeyboardInterrupt(),
                lambda candidate: review_payload(
                    candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                ),
            ],
        )
        config = self.config()
        store = RunStore(self.root / "state", config["run_id"])
        state = new_state(config)
        store.save(state)
        controller = Controller(store=store, backend=backend)
        with self.assertRaises(KeyboardInterrupt):
            controller.run(state)

        resumed = store.load()
        self.assertEqual("REVIEW", resumed["phase"])
        final = controller.run(resumed)
        self.assertEqual("READY_FOR_HUMAN_REVIEW", final["outcome"])
        self.assertEqual(1, backend.implementation_calls)


class StorageTest(unittest.TestCase):
    def test_second_owner_cannot_take_run_lock(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            store = RunStore(Path(directory), "locked-run")
            with store.lock(), self.assertRaises(RunLockedError), store.lock():
                pass

    def test_orphan_temporary_file_does_not_replace_valid_state(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            store = RunStore(Path(directory), "atomic-run")
            state = {"value": "durable"}
            store.save(state)
            (store.directory / ".state.json.interrupted.tmp").write_text(
                '{"value":"partial"', encoding="utf-8"
            )
            self.assertEqual(state, store.load())

    def test_iteration_write_is_idempotent_but_not_mutable(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            store = RunStore(Path(directory), "receipts")
            path = store.write_iteration(1, {"decision": "READY"})
            self.assertEqual(path, store.write_iteration(1, {"decision": "READY"}))
            with self.assertRaises(StateError):
                store.write_iteration(1, {"decision": "CHANGED"})


class ValidationRunnerTest(unittest.TestCase):
    def test_timeout_terminates_descendant_process_group(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            cwd = Path(directory)
            marker = cwd / "late-marker"
            command = f"(sleep 0.5; touch {shlex.quote(str(marker))}) & wait"
            result = ValidationRunner().run([command], cwd=cwd, timeout_seconds=0.1)

            self.assertEqual("TIMED_OUT", result["results"][0]["status"])
            time.sleep(0.7)
            self.assertFalse(marker.exists())


class ContractTest(unittest.TestCase):
    def test_schema_and_runtime_payload_fields_stay_aligned(self) -> None:
        implementation_fields = {
            "status",
            "story_ref",
            "title",
            "worktree",
            "branch",
            "base_branch",
            "head_sha",
            "pr_url",
            "summary",
            "checks",
            "known_gaps",
            "decision",
        }
        review_fields = {
            "candidate_sha",
            "verdict",
            "summary",
            "lenses",
            "acceptance_coverage",
            "findings",
            "checks",
            "decision",
        }
        self.assertEqual(implementation_fields, set(IMPLEMENTATION_SCHEMA["required"]))
        self.assertEqual(
            implementation_fields, set(IMPLEMENTATION_SCHEMA["properties"])
        )
        self.assertEqual(review_fields, set(REVIEW_SCHEMA["required"]))
        self.assertEqual(review_fields, set(REVIEW_SCHEMA["properties"]))

    def test_ready_review_rejects_skipped_check(self) -> None:
        payload = review_payload("a" * 40, "READY_FOR_HUMAN_REVIEW")
        payload["checks"][0]["status"] = "SKIPPED"
        with self.assertRaises(ContractError):
            validate_review(payload)

    def test_ready_review_requires_all_three_completed_lenses(self) -> None:
        payload = review_payload("a" * 40, "READY_FOR_HUMAN_REVIEW")
        payload["lenses"][1]["completed"] = False
        payload["lenses"][1]["gaps"] = ["correctness inspection did not run"]
        with self.assertRaises(ContractError):
            validate_review(payload)

    def test_review_requires_exactly_the_three_named_lenses(self) -> None:
        payload = review_payload("a" * 40, "CHANGES_REQUIRED", findings=[finding()])
        payload["lenses"][2]["name"] = "correctness"
        with self.assertRaises(ContractError):
            validate_review(payload)


class DeliveryMetadataTest(unittest.TestCase):
    def state(self) -> dict:
        return {
            "config": {"base_branch": "master"},
            "candidate": {
                "branch": "codex/story",
                "head_sha": "a" * 40,
            },
        }

    def metadata(self, **overrides: object) -> str:
        payload = {
            "url": "https://example.invalid/pr/1",
            "state": "OPEN",
            "isDraft": True,
            "baseRefName": "master",
            "headRefName": "codex/story",
            "headRefOid": "a" * 40,
        }
        payload.update(overrides)
        return json.dumps(payload)

    def test_exact_open_draft_is_accepted(self) -> None:
        self.assertEqual(
            "https://example.invalid/pr/1",
            Controller._validated_pr_url(self.state(), self.metadata()),
        )

    def test_non_draft_or_stale_pr_is_rejected(self) -> None:
        for metadata in (
            self.metadata(isDraft=False),
            self.metadata(baseRefName="other"),
            self.metadata(headRefOid="b" * 40),
            self.metadata(state="MERGED"),
        ):
            with self.subTest(metadata=metadata), self.assertRaises(RuntimeError):
                Controller._validated_pr_url(self.state(), metadata)


class BackendBoundaryTest(unittest.TestCase):
    class Values:
        deny_all = "deny-all"
        read_only = "read-only"
        workspace_write = "workspace-write"

    class Input:
        def __init__(self, **values: str) -> None:
            self.values = values

    class Thread:
        def __init__(self, identifier: str, payload: dict) -> None:
            self.id = identifier
            self.payload = payload
            self.runs: list[dict] = []
            self.inputs: list[object] = []

        def set_name(self, _name: str) -> None:
            return None

        def run(self, input_value: object, **options: object) -> SimpleNamespace:
            self.inputs.append(input_value)
            self.runs.append(options)
            return SimpleNamespace(
                id=f"{self.id}-turn-{len(self.runs)}",
                final_response=json.dumps(self.payload),
            )

    class Codex:
        def __init__(self, payload_factory) -> None:
            self.payload_factory = payload_factory
            self.started: list[tuple[dict, BackendBoundaryTest.Thread]] = []
            self.resumed: list[tuple[str, dict, BackendBoundaryTest.Thread]] = []

        def thread_start(self, **options: object):
            thread = BackendBoundaryTest.Thread(
                f"fresh-{len(self.started) + 1}", self.payload_factory()
            )
            self.started.append((options, thread))
            return thread

        def thread_resume(self, thread_id: str, **options: object):
            thread = BackendBoundaryTest.Thread("resumed", self.payload_factory())
            self.resumed.append((thread_id, options, thread))
            return thread

    def test_completed_write_turn_can_be_recovered_from_sdk_history(self) -> None:
        payload = {
            "status": "DECISION_REQUIRED",
            "story_ref": "story",
            "title": "Story",
            "worktree": "",
            "branch": "",
            "base_branch": "",
            "head_sha": "",
            "pr_url": "",
            "summary": [],
            "checks": [],
            "known_gaps": [],
            "decision": "Choose the intended behavior.",
        }

        class HistoryItem:
            def model_dump(self, **_options: object) -> dict:
                return {
                    "type": "agentMessage",
                    "phase": "final_answer",
                    "text": json.dumps(payload),
                }

        class HistoryThread:
            def read(self, *, include_turns: bool) -> SimpleNamespace:
                if not include_turns:
                    raise AssertionError("turn history was not requested")
                turns = [
                    SimpleNamespace(id="processed-turn", status="completed", items=[]),
                    SimpleNamespace(
                        id="decision-turn",
                        status="completed",
                        items=[HistoryItem()],
                    ),
                ]
                return SimpleNamespace(thread=SimpleNamespace(turns=turns))

        fake_codex = SimpleNamespace(
            thread_resume=lambda *_args, **_options: HistoryThread()
        )
        backend = CodexBackend(repository=Path("/repo"))
        backend._codex = fake_codex
        backend._sdk = {
            "ApprovalMode": self.Values,
            "Sandbox": self.Values,
            "TextInput": self.Input,
        }

        recovered = backend.recover_implementation(
            {"worktree": "/repo/worktree"},
            "implementation-thread",
            "processed-turn",
        )

        self.assertIsNotNone(recovered)
        assert recovered is not None
        self.assertEqual("decision-turn", recovered[0])
        self.assertEqual("DECISION_REQUIRED", recovered[1]["status"])

    def test_reviews_always_start_fresh_read_only_threads(self) -> None:
        sha = "a" * 40
        fake_codex = self.Codex(lambda: review_payload(sha, "READY_FOR_HUMAN_REVIEW"))
        backend = CodexBackend(repository=Path("/repo"))
        backend._codex = fake_codex
        backend._sdk = {
            "ApprovalMode": self.Values,
            "Sandbox": self.Values,
            "TextInput": self.Input,
        }
        config = {
            "run_id": "run",
            "story": "story",
            "base_branch": "master",
            "base_commit": "d" * 40,
            "passes_completed": 0,
            "story_contract": "approved contract",
            "acceptance_criteria": ["criterion one"],
            "review_contract": REVIEW_COORDINATOR_CONTRACT,
        }
        candidate = {
            "worktree": "/repo/worktree",
            "branch": "codex/story",
            "head_sha": sha,
            "pr_url": "",
        }

        first, _ = backend.review(config, candidate)
        second, _ = backend.review(config, candidate)

        self.assertEqual(("fresh-1", "fresh-2"), (first, second))
        self.assertEqual([], fake_codex.resumed)
        self.assertEqual(2, len(fake_codex.started))
        for options, thread in fake_codex.started:
            self.assertEqual("read-only", options["sandbox"])
            self.assertEqual("read-only", thread.runs[0]["sandbox"])
            prompt = thread.inputs[0].values["text"]
            self.assertIn("Exact base commit: " + "d" * 40, prompt)
            self.assertIn(REVIEW_COORDINATOR_CONTRACT, prompt)
            self.assertIn('"criterion one"', prompt)

    def test_repair_resumes_implementation_thread_workspace_write(self) -> None:
        sha = "b" * 40
        config = {
            "run_id": "run",
            "story": "story",
            "repository": "/repo",
            "base_branch": "master",
            "checks": ["true"],
            "story_contract": "approved contract",
        }
        payload = {
            "status": "CANDIDATE_READY",
            "story_ref": "story",
            "title": "Story",
            "worktree": "/repo/worktree",
            "branch": "codex/story",
            "base_branch": "master",
            "head_sha": sha,
            "pr_url": "",
            "summary": ["fixed"],
            "checks": [],
            "known_gaps": [],
            "decision": "",
        }
        fake_codex = self.Codex(lambda: payload)
        backend = CodexBackend(repository=Path("/repo"))
        backend._codex = fake_codex
        backend._sdk = {
            "ApprovalMode": self.Values,
            "Sandbox": self.Values,
            "TextInput": self.Input,
        }

        backend.repair(config, "implementation-thread", payload, [{"source": "review"}])

        self.assertEqual("implementation-thread", fake_codex.resumed[0][0])
        self.assertEqual("workspace-write", fake_codex.resumed[0][1]["sandbox"])
        self.assertEqual("workspace-write", fake_codex.resumed[0][2].runs[0]["sandbox"])

    def test_implementation_thread_is_created_before_write_turn(self) -> None:
        sha = "c" * 40
        payload = {
            "status": "CANDIDATE_READY",
            "story_ref": "story",
            "title": "Story",
            "worktree": "/repo/worktree",
            "branch": "codex/story",
            "base_branch": "master",
            "head_sha": sha,
            "pr_url": "",
            "summary": ["implemented"],
            "checks": [],
            "known_gaps": [],
            "decision": "",
        }
        fake_codex = self.Codex(lambda: payload)
        backend = CodexBackend(repository=Path("/repo"))
        backend._codex = fake_codex
        backend._sdk = {
            "ApprovalMode": self.Values,
            "Sandbox": self.Values,
            "TextInput": self.Input,
        }
        config = {
            "run_id": "run",
            "story": "story",
            "story_title": "Story",
            "story_contract": "approved contract",
            "base_branch": "master",
            "base_commit": "d" * 40,
            "branch": "codex/story",
            "worktree": "/repo/worktree",
            "checks": ["true"],
        }

        thread_id = backend.start_implementation_thread(config)
        self.assertEqual("fresh-1", thread_id)
        self.assertEqual([], fake_codex.resumed)
        result = backend.run_implementation(config, thread_id)

        self.assertEqual(sha, result[1]["head_sha"])
        self.assertEqual("/repo/worktree", fake_codex.started[0][0]["cwd"])
        self.assertEqual("workspace-write", fake_codex.started[0][0]["sandbox"])
        self.assertEqual(thread_id, fake_codex.resumed[0][0])
        self.assertEqual("workspace-write", fake_codex.resumed[0][1]["sandbox"])


class CliTest(unittest.TestCase):
    def test_start_runs_with_injected_backend_and_prints_receipt_locations(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            repository = make_repository(Path(directory))
            contract_file = Path(directory) / "story.md"
            contract_file.write_text(
                "# Test story\n\n## Acceptance criteria\n\n- Approved test criterion.\n",
                encoding="utf-8",
            )
            backend = FakeBackend(
                repository,
                [
                    lambda candidate: review_payload(
                        candidate["head_sha"], "READY_FOR_HUMAN_REVIEW"
                    )
                ],
            )
            output = io.StringIO()
            with (
                mock.patch("story_loop.cli._backend", return_value=backend),
                contextlib.redirect_stdout(output),
            ):
                exit_code = cli.main(
                    [
                        "start",
                        "--story",
                        "https://example.invalid/issues/42",
                        "--repo",
                        str(repository),
                        "--base",
                        "master",
                        "--story-contract-file",
                        str(contract_file),
                        "--check",
                        "true",
                    ]
                )
            result = json.loads(output.getvalue())
            self.assertEqual(0, exit_code)
            self.assertEqual("issue-42", result["run_id"])
            self.assertEqual("READY_FOR_HUMAN_REVIEW", result["outcome"])
            self.assertEqual(
                (repository / ".worktrees" / "issue-42").resolve(),
                Path(result["worktree"]).resolve(),
            )
            self.assertEqual(
                "", git(repository, "status", "--porcelain", "--untracked-files=no")
            )
            self.assertTrue(Path(result["state_path"]).is_file())
            self.assertTrue(backend.closed)


if __name__ == "__main__":
    unittest.main()
