"""Codex SDK adapter for the persistent implementer and fresh reviewers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from .contracts import (
    IMPLEMENTATION_SCHEMA,
    REVIEW_SCHEMA,
    parse_json_object,
    validate_implementation,
    validate_review,
)


class CodexBackend:
    """Thin adapter around the stable Python SDK.

    Imports are lazy so unit tests can run without installing the SDK or
    starting its bundled runtime.
    """

    def __init__(
        self,
        *,
        repository: Path,
        implement_skill: Path,
        review_skill: Path,
        model: str | None = None,
    ) -> None:
        self.repository = repository.resolve()
        self.implement_skill = implement_skill.resolve()
        self.review_skill = review_skill.resolve()
        self.model = model
        self._codex: Any = None
        self._sdk: dict[str, Any] = {}

    def _ensure_client(self) -> Any:
        if self._codex is not None:
            return self._codex
        try:
            from openai_codex import (
                ApprovalMode,
                Codex,
                Sandbox,
                SkillInput,
                TextInput,
            )
        except ImportError as exc:
            raise RuntimeError(
                "openai-codex is not installed; run `uv sync --project tools/story-loop`"
            ) from exc
        self._sdk = {
            "ApprovalMode": ApprovalMode,
            "Sandbox": Sandbox,
            "SkillInput": SkillInput,
            "TextInput": TextInput,
        }
        self._codex = Codex()
        return self._codex

    def close(self) -> None:
        if self._codex is not None:
            self._codex.close()
            self._codex = None

    def start_implementation(
        self, config: dict[str, Any]
    ) -> tuple[str, dict[str, Any]]:
        codex = self._ensure_client()
        sdk = self._sdk
        thread = codex.thread_start(
            approval_mode=sdk["ApprovalMode"].deny_all,
            cwd=str(self.repository),
            model=self.model,
            sandbox=sdk["Sandbox"].workspace_write,
        )
        thread.set_name(f"story-loop implement {config['run_id']}")
        prompt = "\n".join(
            [
                "Use the implement-story workflow for exactly one approved story.",
                f"Story: {config['story']}",
                f"Base branch: {config['base_branch']}",
                f"Repository: {config['repository']}",
                "You are the persistent implementation worker in an outer review-repair loop.",
                "Prepare the skill's isolated worktree, implement only the story contract, run its deterministic checks, and commit the candidate.",
                "The outer controller owns the fresh review passes and draft-PR delivery. Do not push, open or edit a pull request, approve, or merge.",
                "Return only the requested structured result. A CANDIDATE_READY result must identify the exact clean worktree, branch, and full HEAD SHA.",
                "Use empty strings or arrays for non-applicable fields; never omit a field.",
                f"Controller validation commands (informational only; the controller runs them independently): {json.dumps(config['checks'])}",
            ]
        )
        result = thread.run(
            [
                sdk["SkillInput"](
                    name="implement-story", path=str(self.implement_skill)
                ),
                sdk["TextInput"](text=prompt),
            ],
            approval_mode=sdk["ApprovalMode"].deny_all,
            cwd=str(self.repository),
            model=self.model,
            output_schema=IMPLEMENTATION_SCHEMA,
            sandbox=sdk["Sandbox"].workspace_write,
        )
        payload = validate_implementation(
            parse_json_object(result.final_response, "implementation")
        )
        return thread.id, payload

    def repair(
        self,
        config: dict[str, Any],
        thread_id: str,
        candidate: dict[str, Any],
        delta: list[dict[str, Any]],
    ) -> dict[str, Any]:
        codex = self._ensure_client()
        sdk = self._sdk
        thread = codex.thread_resume(
            thread_id,
            approval_mode=sdk["ApprovalMode"].deny_all,
            cwd=candidate["worktree"],
            model=self.model,
            sandbox=sdk["Sandbox"].workspace_write,
        )
        prompt = "\n".join(
            [
                "Continue the same story as its persistent implementation worker.",
                f"The reviewed candidate was commit {candidate['head_sha']} in {candidate['worktree']}.",
                "Apply the smallest focused repair that resolves the validated delta below.",
                "Re-read affected source-of-truth files when needed, run relevant deterministic checks, and commit the repair on the same branch.",
                "Do not widen the story, push, create or edit a pull request, approve, or merge.",
                "If the feedback requires a product/specification choice, return DECISION_REQUIRED instead of guessing.",
                "Return only the requested structured result with every field present.",
                json.dumps({"remaining_delta": delta}, indent=2, sort_keys=True),
            ]
        )
        result = thread.run(
            prompt,
            approval_mode=sdk["ApprovalMode"].deny_all,
            cwd=candidate["worktree"],
            model=self.model,
            output_schema=IMPLEMENTATION_SCHEMA,
            sandbox=sdk["Sandbox"].workspace_write,
        )
        return validate_implementation(
            parse_json_object(result.final_response, "repair")
        )

    def review(
        self,
        config: dict[str, Any],
        candidate: dict[str, Any],
    ) -> tuple[str, dict[str, Any]]:
        codex = self._ensure_client()
        sdk = self._sdk

        # Freshness is enforced here: every call starts a new thread. Reviewers
        # are never resumed or forked from the implementer or a prior reviewer.
        thread = codex.thread_start(
            approval_mode=sdk["ApprovalMode"].deny_all,
            cwd=candidate["worktree"],
            ephemeral=False,
            model=self.model,
            sandbox=sdk["Sandbox"].read_only,
        )
        pass_number = int(config.get("passes_completed", 0)) + 1
        thread.set_name(f"story-loop review {config['run_id']} pass {pass_number}")
        prompt = "\n".join(
            [
                "Use the review-story workflow for one exact, immutable candidate.",
                f"Story: {config['story']}",
                f"Candidate worktree: {candidate['worktree']}",
                f"Candidate branch: {candidate['branch']}",
                f"Candidate commit: {candidate['head_sha']}",
                f"Base branch: {config['base_branch']}",
                f"Pull request, if one already exists: {candidate.get('pr_url', '')}",
                "This thread is a fresh read-only reviewer. Do not edit files, post comments, approve, merge, or delegate fixes.",
                "Run read-only deterministic checks when useful, but keep their results distinct from model findings.",
                "Return only the requested structured result with every field present.",
                "Use zero for unknown line numbers and an empty decision string when no decision is required.",
                "READY_FOR_HUMAN_REVIEW must have no findings. CHANGES_REQUIRED must contain actionable findings.",
            ]
        )
        result = thread.run(
            [
                sdk["SkillInput"](name="review-story", path=str(self.review_skill)),
                sdk["TextInput"](text=prompt),
            ],
            approval_mode=sdk["ApprovalMode"].deny_all,
            cwd=candidate["worktree"],
            model=self.model,
            output_schema=REVIEW_SCHEMA,
            sandbox=sdk["Sandbox"].read_only,
        )
        payload = validate_review(parse_json_object(result.final_response, "review"))
        return thread.id, payload

    def smoke(self) -> dict[str, Any]:
        codex = self._ensure_client()
        sdk = self._sdk
        account = codex.account()
        thread = codex.thread_start(
            approval_mode=sdk["ApprovalMode"].deny_all,
            cwd=str(self.repository),
            ephemeral=True,
            model=self.model,
            sandbox=sdk["Sandbox"].read_only,
        )
        schema = {
            "type": "object",
            "properties": {"ok": {"type": "boolean"}},
            "required": ["ok"],
            "additionalProperties": False,
        }
        result = thread.run(
            'Do not use tools or inspect files. Return {"ok": true}.',
            approval_mode=sdk["ApprovalMode"].deny_all,
            cwd=str(self.repository),
            model=self.model,
            output_schema=schema,
            sandbox=sdk["Sandbox"].read_only,
        )
        payload = parse_json_object(result.final_response, "smoke check")
        if payload != {"ok": True}:
            raise RuntimeError(f"unexpected smoke response: {payload!r}")
        account_payload = account.model_dump(mode="json", exclude_none=True)
        return {
            "authenticated": bool(account_payload.get("account")),
            "model_turn": "passed",
            "sandbox": "read-only",
        }
