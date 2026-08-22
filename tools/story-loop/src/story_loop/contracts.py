"""Structured contracts exchanged with Codex turns.

The JSON schemas constrain model output. These validators remain mandatory:
structured model output is still untrusted input at the orchestration boundary.
"""

from __future__ import annotations

import json
import re
from typing import Any


class ContractError(ValueError):
    """Raised when a model handoff does not match its declared contract."""


def _object_schema(properties: dict[str, Any]) -> dict[str, Any]:
    return {
        "type": "object",
        "properties": properties,
        "required": list(properties),
        "additionalProperties": False,
    }


def _string_array_schema() -> dict[str, Any]:
    return {"type": "array", "items": {"type": "string"}}


AGENT_CHECK_SCHEMA = _object_schema(
    {
        "command": {"type": "string"},
        "status": {"type": "string", "enum": ["PASSED", "FAILED", "SKIPPED"]},
        "evidence": {"type": "string"},
    }
)

IMPLEMENTATION_SCHEMA = _object_schema(
    {
        "status": {
            "type": "string",
            "enum": [
                "CANDIDATE_READY",
                "DECISION_REQUIRED",
                "IMPLEMENTATION_INCOMPLETE",
            ],
        },
        "story_ref": {"type": "string"},
        "title": {"type": "string"},
        "worktree": {"type": "string"},
        "branch": {"type": "string"},
        "base_branch": {"type": "string"},
        "head_sha": {"type": "string"},
        "pr_url": {"type": "string"},
        "summary": _string_array_schema(),
        "checks": {"type": "array", "items": AGENT_CHECK_SCHEMA},
        "known_gaps": _string_array_schema(),
        "decision": {"type": "string"},
    }
)

FINDING_SCHEMA = _object_schema(
    {
        "id": {"type": "string"},
        "priority": {"type": "integer", "minimum": 0, "maximum": 3},
        "title": {"type": "string"},
        "body": {"type": "string"},
        "file": {"type": "string"},
        "start": {"type": "integer", "minimum": 0},
        "end": {"type": "integer", "minimum": 0},
    }
)

LENS_SCHEMA = _object_schema(
    {
        "name": {
            "type": "string",
            "enum": ["contract", "correctness", "maintainability"],
        },
        "completed": {"type": "boolean"},
        "summary": {"type": "string"},
        "gaps": _string_array_schema(),
    }
)

ACCEPTANCE_COVERAGE_SCHEMA = _object_schema(
    {
        "criterion": {"type": "string"},
        "status": {
            "type": "string",
            "enum": ["COVERED", "GAP", "NOT_APPLICABLE"],
        },
        "evidence": {"type": "string"},
    }
)

REVIEW_SCHEMA = _object_schema(
    {
        "candidate_sha": {"type": "string"},
        "verdict": {
            "type": "string",
            "enum": [
                "READY_FOR_HUMAN_REVIEW",
                "CHANGES_REQUIRED",
                "DECISION_REQUIRED",
                "REVIEW_INCOMPLETE",
            ],
        },
        "summary": {"type": "string"},
        "lenses": {"type": "array", "items": LENS_SCHEMA},
        "acceptance_coverage": {
            "type": "array",
            "items": ACCEPTANCE_COVERAGE_SCHEMA,
        },
        "findings": {"type": "array", "items": FINDING_SCHEMA},
        "checks": {"type": "array", "items": AGENT_CHECK_SCHEMA},
        "decision": {"type": "string"},
    }
)

_SHA_RE = re.compile(r"^[0-9a-f]{40,64}$")


def parse_json_object(raw: str | None, contract_name: str) -> dict[str, Any]:
    if raw is None:
        raise ContractError(f"{contract_name} returned no final response")
    try:
        value = json.loads(
            raw,
            parse_constant=lambda value: (_ for _ in ()).throw(
                ValueError(f"invalid JSON constant {value}")
            ),
        )
    except (ValueError, json.JSONDecodeError) as exc:
        raise ContractError(f"{contract_name} returned invalid JSON: {exc}") from exc
    if not isinstance(value, dict):
        raise ContractError(f"{contract_name} must return a JSON object")
    return value


def _strict_object(value: Any, keys: set[str], path: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise ContractError(f"{path} must be an object")
    actual = set(value)
    if actual != keys:
        missing = sorted(keys - actual)
        extra = sorted(actual - keys)
        raise ContractError(f"{path} keys mismatch; missing={missing}, extra={extra}")
    return value


def _string(value: Any, path: str, *, nonempty: bool = False) -> str:
    if not isinstance(value, str):
        raise ContractError(f"{path} must be a string")
    if nonempty and not value.strip():
        raise ContractError(f"{path} must not be empty")
    return value


def _string_list(value: Any, path: str) -> list[str]:
    if not isinstance(value, list):
        raise ContractError(f"{path} must be an array")
    for index, item in enumerate(value):
        _string(item, f"{path}[{index}]", nonempty=True)
    return list(value)


def _agent_checks(value: Any, path: str) -> list[dict[str, Any]]:
    if not isinstance(value, list):
        raise ContractError(f"{path} must be an array")
    result: list[dict[str, Any]] = []
    keys = {"command", "status", "evidence"}
    for index, item in enumerate(value):
        check = _strict_object(item, keys, f"{path}[{index}]")
        _string(check["command"], f"{path}[{index}].command", nonempty=True)
        status = _string(check["status"], f"{path}[{index}].status")
        if status not in {"PASSED", "FAILED", "SKIPPED"}:
            raise ContractError(f"{path}[{index}].status is invalid: {status}")
        _string(check["evidence"], f"{path}[{index}].evidence")
        result.append(dict(check))
    return result


def validate_implementation(value: Any) -> dict[str, Any]:
    keys = {
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
    payload = _strict_object(value, keys, "implementation")
    status = _string(payload["status"], "implementation.status")
    if status not in {
        "CANDIDATE_READY",
        "DECISION_REQUIRED",
        "IMPLEMENTATION_INCOMPLETE",
    }:
        raise ContractError(f"implementation.status is invalid: {status}")

    for key in (
        "story_ref",
        "title",
        "worktree",
        "branch",
        "base_branch",
        "head_sha",
        "pr_url",
        "decision",
    ):
        _string(payload[key], f"implementation.{key}")
    _string_list(payload["summary"], "implementation.summary")
    _agent_checks(payload["checks"], "implementation.checks")
    _string_list(payload["known_gaps"], "implementation.known_gaps")

    if status == "CANDIDATE_READY":
        for key in (
            "story_ref",
            "title",
            "worktree",
            "branch",
            "base_branch",
            "head_sha",
        ):
            _string(payload[key], f"implementation.{key}", nonempty=True)
        if not _SHA_RE.fullmatch(payload["head_sha"]):
            raise ContractError(
                "implementation.head_sha must be a full lowercase Git object ID"
            )
    elif not payload["decision"].strip():
        raise ContractError(f"implementation.decision is required for {status}")
    return dict(payload)


def validate_review(value: Any) -> dict[str, Any]:
    keys = {
        "candidate_sha",
        "verdict",
        "summary",
        "lenses",
        "acceptance_coverage",
        "findings",
        "checks",
        "decision",
    }
    payload = _strict_object(value, keys, "review")
    sha = _string(payload["candidate_sha"], "review.candidate_sha", nonempty=True)
    if not _SHA_RE.fullmatch(sha):
        raise ContractError(
            "review.candidate_sha must be a full lowercase Git object ID"
        )
    verdict = _string(payload["verdict"], "review.verdict")
    allowed = {
        "READY_FOR_HUMAN_REVIEW",
        "CHANGES_REQUIRED",
        "DECISION_REQUIRED",
        "REVIEW_INCOMPLETE",
    }
    if verdict not in allowed:
        raise ContractError(f"review.verdict is invalid: {verdict}")
    _string(payload["summary"], "review.summary", nonempty=True)
    _agent_checks(payload["checks"], "review.checks")
    _string(payload["decision"], "review.decision")

    lenses = payload["lenses"]
    if not isinstance(lenses, list):
        raise ContractError("review.lenses must be an array")
    lens_keys = {"name", "completed", "summary", "gaps"}
    lens_names: list[str] = []
    for index, item in enumerate(lenses):
        lens = _strict_object(item, lens_keys, f"review.lenses[{index}]")
        name = _string(lens["name"], f"review.lenses[{index}].name", nonempty=True)
        lens_names.append(name)
        if not isinstance(lens["completed"], bool):
            raise ContractError(f"review.lenses[{index}].completed must be a boolean")
        _string(lens["summary"], f"review.lenses[{index}].summary", nonempty=True)
        _string_list(lens["gaps"], f"review.lenses[{index}].gaps")
    if sorted(lens_names) != ["contract", "correctness", "maintainability"]:
        raise ContractError("review must contain exactly the three required lenses")

    coverage = payload["acceptance_coverage"]
    if not isinstance(coverage, list) or not coverage:
        raise ContractError("review.acceptance_coverage must be a non-empty array")
    coverage_keys = {"criterion", "status", "evidence"}
    for index, item in enumerate(coverage):
        entry = _strict_object(
            item, coverage_keys, f"review.acceptance_coverage[{index}]"
        )
        _string(
            entry["criterion"],
            f"review.acceptance_coverage[{index}].criterion",
            nonempty=True,
        )
        status = _string(entry["status"], f"review.acceptance_coverage[{index}].status")
        if status not in {"COVERED", "GAP", "NOT_APPLICABLE"}:
            raise ContractError(
                f"review.acceptance_coverage[{index}].status is invalid: {status}"
            )
        _string(
            entry["evidence"],
            f"review.acceptance_coverage[{index}].evidence",
            nonempty=True,
        )

    findings = payload["findings"]
    if not isinstance(findings, list):
        raise ContractError("review.findings must be an array")
    finding_keys = {"id", "priority", "title", "body", "file", "start", "end"}
    for index, item in enumerate(findings):
        finding = _strict_object(item, finding_keys, f"review.findings[{index}]")
        for key in ("id", "title", "body"):
            _string(finding[key], f"review.findings[{index}].{key}", nonempty=True)
        _string(finding["file"], f"review.findings[{index}].file")
        priority = finding["priority"]
        if (
            isinstance(priority, bool)
            or not isinstance(priority, int)
            or not 0 <= priority <= 3
        ):
            raise ContractError(
                f"review.findings[{index}].priority must be an integer from 0 to 3"
            )
        for key in ("start", "end"):
            line = finding[key]
            if isinstance(line, bool) or not isinstance(line, int) or line < 0:
                raise ContractError(
                    f"review.findings[{index}].{key} must be a non-negative integer"
                )
        if finding["end"] and finding["start"] and finding["end"] < finding["start"]:
            raise ContractError(f"review.findings[{index}] has an end before its start")

    if verdict == "READY_FOR_HUMAN_REVIEW" and findings:
        raise ContractError("a ready review must not contain findings")
    if verdict == "READY_FOR_HUMAN_REVIEW" and any(
        check["status"] != "PASSED" for check in payload["checks"]
    ):
        raise ContractError("a ready review must not contain failed or skipped checks")
    if verdict == "READY_FOR_HUMAN_REVIEW" and any(
        not lens["completed"] or lens["gaps"] for lens in lenses
    ):
        raise ContractError("a ready review requires all three lenses without gaps")
    if verdict == "READY_FOR_HUMAN_REVIEW" and any(
        entry["status"] != "COVERED" for entry in coverage
    ):
        raise ContractError(
            "a ready review requires every acceptance criterion covered"
        )
    if verdict == "CHANGES_REQUIRED" and not findings:
        raise ContractError("CHANGES_REQUIRED must contain at least one finding")
    if (
        verdict in {"DECISION_REQUIRED", "REVIEW_INCOMPLETE"}
        and not payload["decision"].strip()
    ):
        raise ContractError(f"review.decision is required for {verdict}")
    return dict(payload)
