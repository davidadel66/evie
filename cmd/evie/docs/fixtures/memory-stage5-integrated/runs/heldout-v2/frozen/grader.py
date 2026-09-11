#!/usr/bin/env python3
"""Validate explicit semantic assessments and calculate frozen quality gates.

This validates attribution evidence and arithmetic; it does not replace an
assessor's semantic judgment with answer keywords or a model-based judge.
"""

import argparse
import hashlib
import json
from pathlib import Path


COMPONENT_STATUSES = {"supported", "missing", "contradicted", "unsupported"}
PROPOSITION_STATUSES = {"grounded", "unsupported", "misattributed"}
BEHAVIORS = ("clarification", "abstention", "attribution", "temporal", "conflict", "general")
HARD = ("fabricated_source_citations", "retired_as_current_assertions",
        "authority_or_speaker_violations", "silent_conflict_resolutions")


def read(path):
    return json.loads(Path(path).read_text())


def sha(data):
    return hashlib.sha256(data).hexdigest()


def file_sha(path):
    return sha(Path(path).read_bytes())


def ratio(numerator, denominator):
    return numerator / denominator if denominator else None


def integer(value):
    return type(value) is int and value >= 0


def require(ok, message, errors):
    if not ok:
        errors.append(message)
    return bool(ok)


def strings(value):
    return isinstance(value, list) and all(isinstance(item, str) for item in value)


def identity(value, errors, label):
    if not require(isinstance(value, dict), label + " must be an identity object", errors):
        return
    require(value.get("kind") in {"agent", "human"}, label + " must explicitly declare agent or human", errors)
    for field in ("identity", "role"):
        require(isinstance(value.get(field), str) and bool(value[field].strip()), label + " lacks " + field, errors)


def exact_spans(value, answer, errors, label, required=False):
    if not require(strings(value), label + " must be a list of exact answer spans", errors):
        return
    require(not required or bool(value), label + " requires an answer span", errors)
    for span in value:
        require(bool(span) and span in answer, label + " is absent from the exact final answer", errors)


def delivered_messages(packet):
    """Text from retained actual requests with a successful HTTP response."""
    result = []
    retained = [item for item in packet.get("dispatches") or [] if item.get("actual_wire_retained")]
    inputs = packet.get("actual_provider_inputs") or []
    if len(retained) != len(inputs):
        return result
    for payload, dispatch in zip(inputs, retained):
        if not dispatch.get("successful_http_response_observed"):
            continue
        for message in payload.get("input", payload.get("messages", [])):
            if not isinstance(message, dict):
                continue
            content = message.get("content", [])
            if isinstance(content, str):
                content = [{"type": "input_text", "text": content}]
            if not isinstance(content, list):
                continue
            for part in content:
                if not isinstance(part, dict) or part.get("type") not in {"input_text", "output_text", "text"}:
                    continue
                text = part.get("text")
                if not isinstance(text, str) or text.startswith("EVIE_MEMORY_DATA\n"):
                    continue
                result.append((message.get("role"), text))
    return result


def working_texts(packet):
    """Original reader-session context actually supplied, never model self-support.

    A match proves availability, not truth, acceptance, or correct attribution.
    A validated summary may interpret a request; it cannot establish a personal
    preference. That distinction remains the explicit semantic judgment.
    """
    result = []
    summary = (packet.get("compaction_fixture") or {}).get("summary")
    originals = {("user" if source.get("actor") == "owner" else "assistant", source["evidence"])
                 for source in packet.get("_working_sources") or []
                 if source.get("actor") in {"owner", "assistant"}}
    for role, text in delivered_messages(packet):
        if text == packet.get("question"):
            continue
        if (role, text) in originals:
            result.append(text)
        elif role in {"system", "developer"} and summary and text == summary:
            result.append(text)
    return result


def source_ids(entry, supported, bindings, errors, label, require_source):
    records = entry.get("source_record_ids")
    events = entry.get("source_event_ids", [])
    if not require(strings(records) and strings(events), label + " source IDs must be string lists", errors):
        return
    allowed_events = {bindings[name]["source"]["event_id"]
                      for name in supported if name in bindings}
    for record in records:
        require(record in bindings and record in supported, label + " names unsupported source record " + record, errors)
    for event in events:
        require(event in allowed_events, label + " names an event without independently validated support", errors)
    if records and events:
        record_events = {bindings[record]["source"]["event_id"] for record in records if record in bindings}
        require(set(events) == record_events, label + " event IDs differ from the declared original source records", errors)
    require(not require_source or bool(records or events), label + " requires independently supported source IDs", errors)


def assess_basis(entry, packet, supported, errors, label, grounded):
    basis = entry.get("basis")
    require(basis in {"retrieved_evidence", "current_request", "working_context", "none"},
            label + " has invalid basis", errors)
    if basis == "retrieved_evidence":
        source_ids(entry, supported, packet["bindings"], errors, label, grounded)
        require(entry.get("context_spans") == [], label + " retrieved evidence must not masquerade as context", errors)
    elif basis in {"current_request", "working_context"}:
        spans = entry.get("context_spans")
        require(strings(spans) and bool(spans), label + " needs exact actual input spans", errors)
        texts = ([packet["question"]] if ("user", packet["question"]) in delivered_messages(packet) else []) if basis == "current_request" else working_texts(packet)
        if strings(spans):
            for span in spans:
                require(bool(span) and any(span in text for text in texts),
                        label + " basis is absent from actual permitted input", errors)
        require(entry.get("source_record_ids") == [] and entry.get("source_event_ids") == [],
                label + " cannot treat context-only interpretation as accepted retrieval", errors)
    else:
        require(not grounded, label + " cannot be grounded without a basis", errors)
        require(entry.get("source_record_ids") == [] and entry.get("source_event_ids") == []
                and entry.get("context_spans") == [], label + " absent basis must have empty provenance lists", errors)


def validate_assessment(assessment, packet, case, row, condition, freeze_sha, packet_sha,
                        public_sources=(), working_sources=()):
    errors = []
    packet = dict(packet, _working_sources=working_sources)
    required = set(case["gold"]["support_sets"][0])
    supported = set(row.get("union") or [])
    bindings = packet["bindings"]
    answer = packet.get("final_answer") or ""
    require(isinstance(assessment, dict), "assessment must be an object", errors)
    if errors:
        return {"errors": errors}
    for key, expected in (("schema_version", 1), ("case_id", case["id"]),
                          ("condition", condition), ("freeze_sha256", freeze_sha),
                          ("packet_sha256", packet_sha), ("answer_sha256", sha(answer.encode()))):
        require(assessment.get(key) == expected, key + " differs from frozen assessment input", errors)
    identity(assessment.get("assessor"), errors, "assessor")
    require("second_assessment" in assessment, "second_assessment must explicitly be null or an identity", errors)
    if assessment.get("second_assessment") is not None:
        identity(assessment["second_assessment"], errors, "second_assessment")
    require(assessment.get("review_complete") is True,
            "review_complete must attest all components, propositions and citations were examined", errors)
    require(isinstance(assessment.get("notes"), str) and bool(assessment["notes"].strip()),
            "semantic assessment requires explanatory notes", errors)

    components = assessment.get("components")
    expected_components = case["gold"].get("expected_answer_components") or []
    if not isinstance(components, list):
        errors.append("components must be a complete indexed list")
        components = []
    indexes = [item.get("index") for item in components if isinstance(item, dict)]
    require(len(indexes) == len(components) and all(type(i) is int for i in indexes)
            and sorted(indexes) == list(range(len(expected_components))),
            "components must contain every declared zero-based index exactly once", errors)
    component_hits = 0
    for item in components:
        if not isinstance(item, dict):
            continue
        label = "component " + str(item.get("index"))
        status = item.get("status")
        require(status in COMPONENT_STATUSES, label + " has invalid status", errors)
        exact_spans(item.get("answer_spans"), answer, errors, label, status != "missing")
        assess_basis(item, packet, supported, errors, label, status == "supported")
        if status == "missing":
            require(item.get("answer_spans") == [], label + " marked missing must not invent an answer span", errors)
        if status == "supported":
            component_hits += 1

    propositions = assessment.get("personal_propositions")
    if not isinstance(propositions, list):
        errors.append("personal_propositions must be an explicit complete list")
        propositions = []
    grounded = 0
    misattributed = 0
    for index, item in enumerate(propositions):
        label = "proposition " + str(index)
        if not require(isinstance(item, dict), label + " must be an object", errors):
            continue
        status = item.get("status")
        require(status in PROPOSITION_STATUSES, label + " has invalid status", errors)
        exact_spans(item.get("answer_spans"), answer, errors, label, True)
        assess_basis(item, packet, supported, errors, label, status == "grounded")
        grounded += status == "grounded"
        misattributed += status == "misattributed"
    require(not (required and component_hits) or bool(propositions),
            "supported personal components require an explicit personal-proposition inventory", errors)

    citations = assessment.get("citations")
    if not isinstance(citations, list):
        errors.append("citations must explicitly grade every required support record")
        citations = []
    citation_records = [item.get("record_id") for item in citations if isinstance(item, dict)]
    require(len(citation_records) == len(citations) and len(citation_records) == len(set(citation_records))
            and set(citation_records) == required, "citation records differ from required support set", errors)
    citation_hits = 0
    known_events = {binding["source"]["event_id"] for binding in bindings.values()}
    known_events.update(source["event_id"] for source in public_sources)
    fabricated = set()
    for item in citations:
        if not isinstance(item, dict):
            continue
        record, status = item.get("record_id"), item.get("status")
        event, span = item.get("cited_event_id"), item.get("answer_span")
        require(status in {"correct", "missing", "wrong"}, "citation has invalid status", errors)
        if status == "missing":
            require(event == "" and span == "", "missing citation must have empty ID/span", errors)
            continue
        require(isinstance(event, str) and bool(event) and isinstance(span, str)
                and bool(span) and span in answer and event in span,
                "citation ID and exact span must actually occur in answer", errors)
        if isinstance(event, str) and event and event not in known_events:
            fabricated.add(event)
        if status == "correct":
            require(record in bindings and record in supported
                    and event == bindings.get(record, {}).get("source", {}).get("event_id"),
                    "correct citation lacks the exact independently supported original event", errors)
            citation_hits += 1

    additional = assessment.get("additional_citations")
    require(isinstance(additional, list), "additional_citations must explicitly enumerate any other citations", errors)
    additional_citation_count = len(additional) if isinstance(additional, list) else 0
    additional_citation_hits = 0
    for item in additional if isinstance(additional, list) else []:
        if not require(isinstance(item, dict), "additional citation must be an object", errors):
            continue
        event, span = item.get("cited_event_id"), item.get("answer_span")
        require(item.get("status") in {"correct", "wrong"}, "additional citation has invalid status", errors)
        require(isinstance(event, str) and bool(event) and isinstance(span, str) and bool(span)
                and span in answer and event in span, "additional citation lacks an exact answer ID/span", errors)
        if isinstance(event, str) and event not in known_events:
            fabricated.add(event)
        if item.get("status") == "correct":
            source_ids(item, supported, bindings, errors, "additional citation", True)
            valid = {bindings[r]["source"]["event_id"] for r in item.get("source_record_ids", []) if r in bindings}
            valid.update(item.get("source_event_ids", []))
            require(event in valid, "additional correct citation event differs from its supported source", errors)
            additional_citation_hits += 1

    quotes = assessment.get("source_quotes")
    require(isinstance(quotes, list), "source_quotes must enumerate every claimed verbatim source quote", errors)
    for item in quotes if isinstance(quotes, list) else []:
        if not require(isinstance(item, dict), "source quote must be an object", errors):
            continue
        record, event = item.get("source_record_id"), item.get("source_event_id")
        span, quote = item.get("answer_span"), item.get("quote")
        require(item.get("status") in {"correct", "wrong"}, "source quote needs an explicit correctness assessment", errors)
        require(isinstance(span, str) and bool(span) and span in answer and isinstance(quote, str)
                and bool(quote) and quote in span, "source quote must actually occur within its answer span", errors)
        if item.get("status") == "correct":
            require(record in supported and record in bindings
                    and event == bindings.get(record, {}).get("source", {}).get("event_id")
                    and isinstance(quote, str) and quote in bindings.get(record, {}).get("source", {}).get("evidence", ""),
                    "source quote differs from exact independently supported original text", errors)

    behavior = assessment.get("behavior")
    if not isinstance(behavior, dict):
        errors.append("behavior must explicitly assess all six rubric dimensions")
        behavior = {}
    for name in BEHAVIORS:
        require(behavior.get(name) in {"pass", "fail", "not_applicable"}, "missing or invalid behavior: " + name, errors)
    require(type(behavior.get("unnecessary_clarification")) is bool,
            "unnecessary_clarification must be an explicit boolean", errors)
    hard = assessment.get("hard_violations")
    if not isinstance(hard, dict):
        errors.append("hard_violations must contain every explicit count")
        hard = {}
    for name in HARD:
        require(integer(hard.get(name)), "invalid hard-violation count: " + name, errors)
    if integer(hard.get("fabricated_source_citations")):
        require(hard["fabricated_source_citations"] >= len(fabricated), "fabricated citation inventory is undercounted", errors)
    if integer(hard.get("authority_or_speaker_violations")):
        require(not misattributed or hard["authority_or_speaker_violations"] > 0,
                "misattributed proposition requires an explicit authority/speaker violation", errors)
    semantic_pass = assessment.get("semantic_pass")
    require(type(semantic_pass) is bool, "semantic_pass must be an explicit boolean", errors)
    if semantic_pass:
        require(not any(hard.get(name, 0) for name in HARD), "semantic pass contradicts a hard violation", errors)
        require(not any(item.get("status") != "grounded" for item in propositions if isinstance(item, dict)),
                "semantic pass contradicts an ungrounded personal assertion", errors)
        require(not any(behavior.get(name) == "fail" for name in BEHAVIORS)
                and behavior.get("unnecessary_clarification") is False,
                "semantic pass contradicts a failed behavior", errors)
        require(packet.get("reader_error") == "<nil>" and bool(answer.strip()),
                "failed or empty reader output cannot be a semantic pass", errors)
        require(all(item.get("status") == "correct" for item in quotes if isinstance(item, dict)),
                "semantic pass contradicts an incorrect source quote", errors)
        require(all(item.get("status") != "wrong" for item in citations if isinstance(item, dict))
                and all(item.get("status") == "correct" for item in (additional if isinstance(additional, list) else []) if isinstance(item, dict)),
                "semantic pass contradicts an incorrect original source citation", errors)
        expected_behavior = {"clarify": "clarification", "abstain": "abstention", "general": "general"}.get(case["gold"]["response_kind"])
        if expected_behavior:
            require(behavior.get(expected_behavior) == "pass", "semantic pass lacks required response behavior", errors)
        baseline_absence = condition in {"no_recall", "recent_context"} and bool(required - supported)
        if baseline_absence:
            require(all(item.get("status") in {"supported", "missing"} for item in components if isinstance(item, dict)),
                    "honest baseline absence cannot waive contradicted or unsupported components", errors)
        else:
            require(component_hits == len(expected_components), "semantic pass has missing answer components", errors)
            require(citation_hits == len(required), "semantic pass lacks required original source citations", errors)
        if required and not baseline_absence:
            require(behavior.get("attribution") == "pass", "personal answer pass requires attribution assessment", errors)
    return {"errors": errors, "component_hits": component_hits,
            "citation_hits": citation_hits + additional_citation_hits,
            "required_citation_hits": citation_hits, "additional_citation_hits": additional_citation_hits,
            "additional_citation_denominator": additional_citation_count,
            "grounded_propositions": grounded, "personal_propositions": len(propositions),
            "hard_violations": hard, "semantic_pass": bool(semantic_pass),
            "unnecessary_clarification": behavior.get("unnecessary_clarification"),
            "assessor": assessment.get("assessor"), "notes": assessment.get("notes")}


def build_report(freeze_path, evidence_path, assessments):
    freeze = read(freeze_path)
    freeze_hash = file_sha(freeze_path)
    for label in ("workload", "gates", "rubric", "assessment"):
        if file_sha(freeze[label + "_path"]) != freeze[label + "_sha256"]:
            raise ValueError("frozen " + label + " changed")
    if file_sha(__file__) != freeze["grader_sha256"]:
        raise ValueError("grader differs from its pre-run frozen implementation")
    workload, gates, evidence = read(freeze["workload_path"]), read(freeze["gates_path"]), read(evidence_path)
    for key, expected_hash in (("freeze_sha256", freeze_hash), ("scorer_sha256", freeze["scorer_sha256"]),
                               ("auditor_sha256", freeze["auditor_sha256"])):
        if evidence.get(key) != expected_hash:
            raise ValueError("evidence audit is not bound to the frozen " + key)
    if workload["partition"] != freeze["partition"] or evidence["partition"] != freeze["partition"]:
        raise ValueError("selected workload, evidence and freeze partitions differ")
    cases, conditions = workload["cases"], gates["conditions"]
    if len(cases) != gates["reader_cases"] or gates["reader_repetitions_per_case_condition"] != 1:
        raise ValueError("grader requires the frozen one-assessment-per-case-condition procedure")
    if (len({case["id"] for case in cases}) != len(cases) or len(set(conditions)) != len(conditions)
            or len({case["family"] for case in cases}) != gates["behavior_families"]
            or len(cases) * len(conditions) != gates["reader_turns"]):
        raise ValueError("declared case/family/condition cohort differs from frozen gates")
    manifest_path = Path(freeze_path).parent / "input-manifest.json"
    if file_sha(manifest_path) != freeze["input_manifest_sha256"]:
        raise ValueError("frozen canonical input manifest changed")
    manifest = read(manifest_path)
    declared_inputs = {case["id"] + "/" + name for case in cases for name in ("seed.json", "seed.db")}
    inputs = Path(freeze["inputs_path"]).resolve()
    actual_files = {str(path.relative_to(inputs)) for path in inputs.rglob("*") if path.is_file()}
    if set(manifest) != declared_inputs or actual_files != declared_inputs:
        raise ValueError("canonical inputs must contain exactly every declared closed seed DB/source map")
    for relative, expected_hash in manifest.items():
        path = (inputs / relative).resolve()
        if not path.is_relative_to(inputs) or file_sha(path) != expected_hash:
            raise ValueError("frozen canonical input changed: " + relative)
    keyed_rows = {(row["case_id"], row["condition"]): row for row in evidence["cases"]}
    if len(keyed_rows) != len(evidence["cases"]):
        raise ValueError("duplicate evidence case/condition")
    expected = {(case["id"], condition) for case in cases for condition in conditions}
    if set(keyed_rows) - expected:
        raise ValueError("evidence report contains undeclared cases or conditions")
    packet_root = Path(evidence_path).parent / "assessment-packets"
    rows, issues, hashes = [], [], {}
    for case in cases:
        if len(case["gold"]["support_sets"]) != 1:
            raise ValueError("selected version-1 workload must freeze exactly one sufficient alternative")
        required = set(case["gold"]["support_sets"][0])
        for condition in conditions:
            stem = case["id"] + "-" + condition
            path, packet_path = Path(assessments) / (stem + ".json"), packet_root / (stem + ".json")
            row = {"case_id": case["id"], "family": case["family"], "condition": condition,
                   "component_denominator": len(case["gold"].get("expected_answer_components") or []) if required else 0,
                   "citation_denominator": len(required), "required_citation_denominator": len(required),
                   "additional_citation_denominator": 0, "assessment_present": path.exists(),
                   "assessment_valid": False, "component_hits": 0, "citation_hits": 0,
                   "required_citation_hits": 0, "additional_citation_hits": 0,
                   "grounded_propositions": 0, "personal_propositions": 0,
                   "semantic_pass": False, "errors": [], "gate_roles": case.get("gate_roles") or []}
            try:
                metrics = keyed_rows[(case["id"], condition)]
                packet = read(packet_path)
                if (packet["case_id"] != case["id"] or packet["condition"] != condition
                        or packet["gold"] != case["gold"] or packet["evidence_metrics"] != metrics
                        or set(metrics["required"]) != required or metrics["source_denominator"] != len(required)):
                    raise ValueError("packet/evidence/workload identity or declared denominator differs")
                seed = read(Path(freeze["inputs_path"]) / case["id"] / "seed.json")
                if (packet["bindings"] != seed["bindings"] or packet["question"] != seed["rendered_question"]
                        or packet.get("compaction_fixture") != seed.get("compaction")):
                    raise ValueError("packet source bindings or rendered question differ from canonical seed")
                assessment = read(path)
                public_sources = seed.get("all_public_sources") or []
                reader_id = seed["readers"]["empty" if condition == "no_recall" else "recent"]["id"]
                working_sources = [source for source in public_sources if source["session_id"] == reader_id]
                result = validate_assessment(assessment, packet, case, metrics, condition, freeze_hash, file_sha(packet_path), public_sources, working_sources)
                row.update(errors=result["errors"], assessor=result.get("assessor"), notes=result.get("notes"))
                row["reported_hard_violations"] = result.get("hard_violations", {})
                row["additional_citation_denominator"] = result.get("additional_citation_denominator", 0)
                row["citation_denominator"] += row["additional_citation_denominator"]
                row["assessment_valid"] = not result["errors"]
                if row["assessment_valid"]:
                    for key in ("citation_hits", "required_citation_hits", "additional_citation_hits", "grounded_propositions", "personal_propositions", "semantic_pass", "unnecessary_clarification"):
                        row[key] = result[key]
                    row["component_hits"] = result["component_hits"] if required else 0
                hashes[stem] = {"assessment_sha256": file_sha(path), "packet_sha256": file_sha(packet_path)}
            except (OSError, ValueError, KeyError, TypeError) as error:
                row["errors"].append(str(error))
            rows.append(row)
            issues.extend(stem + ": " + message for message in row["errors"])
    declared_names = {case["id"] + "-" + condition + ".json" for case in cases for condition in conditions}
    extras = sorted(path.name for path in Path(assessments).glob("*.json") if path.name not in declared_names)
    issues.extend("undeclared assessment file: " + name for name in extras)
    evidence_summaries = {}
    for condition in conditions:
        total, answerable, complete, unwanted, present = 0, 0, 0, 0, 0
        hits = {kind: 0 for kind in ("initial", "final", "union")}
        all_present = True
        for case in cases:
            required = set(case["gold"]["support_sets"][0])
            total += len(required)
            answerable += bool(required)
            metrics = keyed_rows.get((case["id"], condition))
            if metrics is None:
                all_present = False
                continue
            available = {kind: set(metrics.get(field) or []) for kind, field in
                         (("initial", "first"), ("final", "final"), ("union", "union"))}
            label = case["id"] + "-" + condition
            for kind in hits:
                observed = len(required & available[kind])
                hits[kind] += observed
                if metrics.get(kind + "_source_hits") != observed:
                    issues.append(label + ": evidence source hit arithmetic differs for " + kind)
            if not (available["initial"] | available["final"]) <= available["union"]:
                issues.append(label + ": first/final support is absent from the turn union")
            if not available["union"] <= {record["id"] for record in case["records"]}:
                issues.append(label + ": source support contains an undeclared canonical record")
            expected_complete = required <= available["final"] if required else None
            if metrics.get("complete_final_support") != expected_complete:
                issues.append(label + ": complete-support flag differs from exact source sets")
            complete += expected_complete is True
            actual_present = set(metrics.get("present_records") or [])
            allowed = required | set(case["gold"].get("acceptable_context_record_ids") or [])
            actual_unwanted = actual_present - allowed
            if set(metrics.get("unwanted") or []) != actual_unwanted:
                issues.append(label + ": unwanted evidence set differs from declared wanted records")
            unwanted += len(actual_unwanted)
            present += len(actual_present)
        summary = {kind + "_source_micro_recall": ratio(value, total) for kind, value in hits.items()}
        summary.update(required_source_records=total, answerable_cases=answerable,
                       complete_final_support_fraction=ratio(complete, answerable),
                       unwanted_evidence_fraction=ratio(unwanted, present) if all_present else None)
        evidence_summaries[condition] = summary
        for key, value in summary.items():
            if evidence["conditions"].get(condition, {}).get(key) != value:
                issues.append(condition + ": evidence summary arithmetic differs for " + key)
    summaries = {}
    for condition in conditions:
        cohort = [row for row in rows if row["condition"] == condition]
        complete = all(row["assessment_valid"] for row in cohort)
        component_n = sum(row["component_hits"] for row in cohort)
        component_d = sum(row["component_denominator"] for row in cohort)
        citation_n = sum(row["citation_hits"] for row in cohort)
        citation_d = sum(row["citation_denominator"] for row in cohort)
        prop_n = sum(row["grounded_propositions"] for row in cohort)
        prop_d = sum(row["personal_propositions"] for row in cohort)
        summaries[condition] = {"planned_assessments": len(cohort), "valid_assessments": sum(row["assessment_valid"] for row in cohort),
            "complete": complete, "supported_components": component_n, "component_denominator": component_d,
            "supported_answer_component_recall": ratio(component_n, component_d), "correct_citations": citation_n,
            "citation_denominator": citation_d, "original_event_citation_accuracy": ratio(citation_n, citation_d),
            "required_correct_citations": sum(row["required_citation_hits"] for row in cohort),
            "required_citation_denominator": sum(row["required_citation_denominator"] for row in cohort),
            "additional_correct_citations": sum(row["additional_citation_hits"] for row in cohort),
            "additional_citation_denominator": sum(row["additional_citation_denominator"] for row in cohort),
            "observed_grounded_propositions": prop_n, "observed_personal_propositions": prop_d,
            "personal_proposition_grounding": ratio(prop_n, prop_d) if complete else None,
            "semantic_passes": sum(row["semantic_pass"] for row in cohort)}
    checks = []
    def check(name, observed, threshold, comparison, condition=None):
        passed = observed is not None and ((observed >= threshold) if comparison == ">=" else (observed <= threshold) if comparison == "<=" else observed == threshold)
        checks.append({"name": name, "condition": condition, "observed": observed,
                       "comparison": comparison, "threshold": threshold, "passed": passed})
    check("all_planned_assessments_valid", sum(row["assessment_valid"] for row in rows), len(expected), "==")
    check("undeclared_assessments", len(extras), 0, "==")
    check("evidence_audit_violations", len(evidence.get("violations") or []), 0, "==")
    quality, production = gates["quality"], gates["quality_condition"]
    for key in ("initial_source_micro_recall", "final_source_micro_recall", "union_source_micro_recall", "complete_final_support_fraction"):
        gate_key = "complete_support_case_fraction_min" if key == "complete_final_support_fraction" else key + "_min"
        check(key, evidence_summaries[production][key], quality[gate_key], ">=", production)
    check("unwanted_evidence_fraction", evidence_summaries[production]["unwanted_evidence_fraction"], quality["unwanted_evidence_fraction_max"], "<=", production)
    for condition, mappings in ((production, {"supported_answer_component_recall":"supported_answer_component_recall_min", "personal_proposition_grounding":"personal_proposition_grounding_min", "original_event_citation_accuracy":"original_event_citation_accuracy_min"}),
                                ("oracle", {"supported_answer_component_recall":"oracle_supported_answer_component_recall_min", "personal_proposition_grounding":"oracle_grounding_min", "original_event_citation_accuracy":"oracle_citation_accuracy_min"})):
        for metric, gate_key in mappings.items():
            check(metric, summaries[condition][metric], quality[gate_key], ">=", condition)
        for selector in quality["required_role_selectors"]:
            cohort = [row for row in rows if row["condition"] == condition and selector["gate_role"] in row["gate_roles"]]
            check("role_count:" + selector["gate_role"], len(cohort), selector["minimum_selected_cases"], ">=", condition)
            if selector["all_matching_cases_must_pass"]:
                check("role_passes:" + selector["gate_role"], sum(row["semantic_pass"] for row in cohort), len(cohort), "==", condition)
            if selector["gate_role"] == "clear_reference":
                check("unnecessary_clarification", sum(row.get("unnecessary_clarification") is not False for row in cohort), 0, "==", condition)
        for family in sorted({case["family"] for case in cases}):
            cohort = [row for row in rows if row["condition"] == condition and row["family"] == family]
            check("family_semantic_pass:" + family, ratio(sum(row["semantic_pass"] for row in cohort), len(cohort)), quality["each_family_semantic_pass_fraction_min"], ">=", condition)
    hard_totals = {}
    for name in HARD:
        values = [row.get("reported_hard_violations", {}).get(name) for row in rows]
        complete = all(integer(value) for value in values)
        hard_totals[name] = sum(value for value in values if integer(value)) if complete else None
        check(name, hard_totals[name], gates["hard_boundaries"][name + "_max"], "<=")
    return {"schema_version": 1, "partition": freeze["partition"], "freeze_sha256": freeze_hash,
            "assessment_directory": str(Path(assessments).resolve()),
            "evidence_report_path": str(Path(evidence_path).resolve()),
            "evidence_report_sha256": file_sha(evidence_path), "grader_sha256": file_sha(__file__),
            "assessment_hashes": hashes, "conditions": summaries, "evidence_conditions": evidence_summaries,
            "cases": rows, "validation_errors": issues,
            "hard_violation_totals": hard_totals, "quality_gates": checks,
            "all_quality_gates_pass": not issues and all(item["passed"] for item in checks),
            "release_ready": False,
            "remaining_required_verification": ["deterministic acceptance and operating failures", "local and reader resource gates", "index build/rebuild/restart and storage/RSS gates", "repository verify-change and final review"],
            "assessment_limit": "Semantic labels are explicit assessor judgments, not automated proof or implied independent human review. Invalid/missing assessments retain coverage denominators; unknown grounding/hard counts remain unavailable."}


def main():
    parser = argparse.ArgumentParser()
    for name in ("freeze", "evidence-report", "assessments", "output"):
        parser.add_argument("--" + name, required=True)
    args = parser.parse_args()
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=False)
    report = build_report(Path(args.freeze).resolve(), Path(args.evidence_report).resolve(), Path(args.assessments).resolve())
    with (output / "quality-report.json").open("x") as stream:
        json.dump(report, stream, indent=2, sort_keys=True, allow_nan=False)
        stream.write("\n")
    print(json.dumps({"quality_gates_pass": report["all_quality_gates_pass"], "release_ready": False,
                      "validation_errors": len(report["validation_errors"])}))
    return 0 if report["all_quality_gates_pass"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
