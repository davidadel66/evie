#!/usr/bin/env python3
"""Read-only source audit for the integrated evaluator (Python stdlib only).

``audit_evidence(items, seed)`` audits one actually delivered evidence group.
``merge_support(groups, seed)`` additionally unions authorized original spans
across delivered requests. Both return present_records, supported_records and
violations. Presence is not support: even an invalid item can disclose a known
record ID. Invalid items contribute no support spans. Complete original source
coverage is required, not an ID match or a short marker. Unlabeled public events
remain non-gold; unknown events are unresolved validation failures.

The caller owns actual-wire/receipt comparisons, forbidden-payload scans,
support-set alternatives, quality denominators and semantic answer assessment.
This module does not run a provider, mutate artifacts or grade reader answers.
"""

import calendar
import datetime
import hashlib
import re


_RFC3339 = re.compile(
    r"^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})"
    r"(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$"
)
_RANGE = re.compile(r"^(0|[1-9]\d*):(0|[1-9]\d*)$")
_HASH = re.compile(r"^[0-9a-fA-F]{64}$")
_SOURCE_FIELDS = (
    "session_id", "source_scope_key", "event_part", "source_type", "actor", "authority"
)


def _instant(value):
    """Exact RFC3339 instant in integer nanoseconds; never a float/microsecond."""
    match = _RFC3339.fullmatch(value if isinstance(value, str) else "")
    if not match:
        raise ValueError("invalid RFC3339 timestamp")
    year, month, day, hour, minute, second = map(int, match.groups()[:6])
    whole = datetime.datetime(year, month, day, hour, minute, second)
    fraction, zone = match.groups()[6:]
    offset = 0
    if zone != "Z":
        zone_hour, zone_minute = int(zone[1:3]), int(zone[4:6])
        if zone_hour > 23 or zone_minute > 59:
            raise ValueError("invalid RFC3339 offset")
        offset = (zone_hour * 60 + zone_minute) * 60
        if zone[0] == "-":
            offset = -offset
    return (calendar.timegm(whole.timetuple()) - offset) * 10**9 + int((fraction or "").ljust(9, "0"))


def _hash(value):
    if isinstance(value, str) and value.startswith("sha256:"):
        value = value[7:]
    if not isinstance(value, str) or not _HASH.fullmatch(value):
        raise ValueError("invalid SHA256 spelling")
    return value.lower()


def _own_span(source):
    if not isinstance(source, dict) or not isinstance(source.get("evidence"), str):
        raise ValueError("source is not an object with original text")
    raw = source["evidence"].encode("utf-8")
    kind, value = source["locator_kind"], source["locator_value"]
    if kind == "whole" and value == "":
        span = (0, len(raw))
    elif kind == "utf8_byte_range" and isinstance(value, str) and _RANGE.fullmatch(value):
        span = tuple(map(int, value.split(":")))
    else:
        raise ValueError("invalid whole/UTF-8 locator")
    if span[0] < 0 or span[1] <= span[0] or span[1] - span[0] != len(raw):
        raise ValueError("source text byte length differs from its locator")
    if _hash(source["evidence_sha256"]) != hashlib.sha256(raw).hexdigest():
        raise ValueError("source hash differs from exact text")
    if source["event_part"] != "content":
        raise ValueError("only original content sources are supported")
    _instant(source["observed_at"])
    return span, raw


def _same_source_metadata(source, original):
    for field in _SOURCE_FIELDS:
        if source.get(field) != original.get(field) or field not in source or field not in original:
            raise ValueError("source metadata differs: " + field)
    if _instant(source["observed_at"]) != _instant(original["observed_at"]):
        raise ValueError("source observation instant differs")


def _covers(spans, required):
    through = required[0]
    for start, end in sorted(spans):
        if end <= through:
            continue
        if start > through:
            return False
        through = end
        if through >= required[1]:
            return True
    return False


class _Catalog:
    def __init__(self, seed):
        self.bindings = seed.get("bindings") or {}
        self.events, self.labels, self.links, self.aliases = {}, {}, {}, {}
        self.requirements, self.errors = {}, []
        self.claims = {}
        originals = []
        for record, binding in self.bindings.items():
            source = binding["source"]
            originals.append((record, source))
            self.requirements[record] = source
            if binding.get("claim_id"):
                self.claims[binding["claim_id"]] = record
            if source.get("source_link_id"):
                self.links[source["source_link_id"]] = source
            for alias in binding.get("accepted_aliases") or []:
                self.aliases[alias["alias_id"]] = alias
        for source in seed.get("auxiliary_sources") or []:
            originals.append(("auxiliary:" + source["event_id"], source))
        for discussion, sources in (seed.get("recent_sources") or {}).items():
            for source in sources:
                originals.append(("recent:" + discussion + ":" + source["event_id"], source))
        # Full public originals extend the catalog; they never relabel a bound
        # record or create an accepted Claim/Source Link/identity association.
        for source in seed.get("all_public_sources") or []:
            event = source["event_id"]
            label = None if any(s["event_id"] == event for _, s in originals) else "unlabeled-event:" + event
            originals.append((label, source))
        for label, source in originals:
            event = source["event_id"]
            if label:
                self.labels.setdefault(event, set()).add(label)
            try:
                span, raw = _own_span(source)
                previous = self.events.get(event)
                if previous:
                    old_source, old_span, old_raw = previous
                    _same_source_metadata(source, old_source)
                    start, end = max(span[0], old_span[0]), min(span[1], old_span[1])
                    if start < end and raw[start-span[0]:end-span[0]] != old_raw[start-old_span[0]:end-old_span[0]]:
                        raise ValueError("canonical originals disagree on overlapping bytes")
                    if not (span[0] <= old_span[0] and span[1] >= old_span[1]):
                        continue
                self.events[event] = source, span, raw
            except (KeyError, TypeError, ValueError, UnicodeError) as error:
                self.errors.append({"code": "invalid_seed_source", "event_id": event, "detail": str(error)})

    def source(self, source):
        if not isinstance(source, dict):
            raise ValueError("source is not an object")
        event = source["event_id"]
        if event not in self.events:
            raise ValueError("unresolved original source event " + str(event))
        original, original_span, original_raw = self.events[event]
        _same_source_metadata(source, original)
        span, raw = _own_span(source)
        if span[0] < original_span[0] or span[1] > original_span[1]:
            raise ValueError("source locator escapes the retained original")
        if source["locator_kind"] == "whole" and span != original_span:
            raise ValueError("whole locator does not describe the complete original")
        expected = original_raw[span[0]-original_span[0]:span[1]-original_span[0]]
        if expected.decode("utf-8") != source["evidence"] or expected != raw:
            raise ValueError("source text differs from exact original bytes")
        if source.get("eligibility", "eligible") != "eligible":
            raise ValueError("source is not eligible")
        return event, span

    def source_reference(self, ref):
        if not isinstance(ref, dict):
            raise ValueError("identity source reference is not an object")
        original = self.links.get(ref.get("source_link_id"))
        if not original:
            raise ValueError("identity source link is unresolved")
        for field in ("event_id", "session_id", "event_part", "locator_kind", "locator_value", "authority"):
            if ref.get(field) != original.get(field):
                raise ValueError("identity source reference differs: " + field)
        if ref.get("scope_key") != original.get("source_scope_key"):
            raise ValueError("identity source scope differs")
        if _hash(ref["evidence_sha256"]) != _hash(original["evidence_sha256"]) or _instant(ref["observed_at"]) != _instant(original["observed_at"]):
            raise ValueError("identity source hash/observation differs")
        self.source(original)
        return original

    def identities(self, item, binding):
        valid = set()
        claim = item.get("claim") or {}
        entities = {binding.get("subject_entity_id"), claim.get("subject_entity_id"), (claim.get("object") or {}).get("entity_id")}
        known_entities = {b.get("subject_entity_id") for b in self.bindings.values()} | {a.get("entity_id") for a in self.aliases.values()}
        for match in item.get("identity_matches") or []:
            if not isinstance(match, dict):
                raise ValueError("identity match is not an object")
            kind, entity, alias_id = match.get("kind"), match.get("entity_id"), match.get("alias_id", "")
            if not entity or entity not in entities or entity not in known_entities:
                raise ValueError("identity does not belong to the supplied Claim")
            if kind == "entity" and not alias_id and not match.get("source") and not match.get("alias_value"):
                valid.add((kind, entity, alias_id))
                continue
            alias = self.aliases.get(alias_id)
            if kind != "alias" or not alias or alias.get("entity_id") != entity or match.get("alias_value") != alias.get("value"):
                raise ValueError("alias ID/entity/value differs from accepted mapping")
            source = self.source_reference(match.get("source") or {})
            if source["event_id"] != alias.get("source_event_id") or (alias.get("operation_id") and source.get("operation_id") != alias["operation_id"]):
                raise ValueError("alias source differs from its accepted origin")
            valid.add((kind, entity, alias_id))
        required = {(ref["kind"], ref["entity_id"], ref.get("alias_id", ""))
                    for ref in (binding.get("exact_reference") or {}).get("identity_matches") or []}
        return required <= valid


def _historical_support(item, binding):
    expected = binding.get("exact_reference") or {}
    if expected.get("intent") and item.get("intent") != expected["intent"]:
        return False
    if expected.get("valid_at_constrained"):
        if not item.get("valid_at_constrained") or _instant(item.get("valid_at")) != _instant(expected["valid_at"]):
            return False
    if binding.get("retired") and item.get("current_status") != "retired":
        return False
    if binding.get("claim_id"):
        effective = item.get("effective_valid_time")
        if effective is None:
            return False
        for edge in ("from", "to"):
            actual, required = effective.get(edge), (binding.get("valid_time") or {}).get(edge)
            if (actual is None) != (required is None) or (actual is not None and _instant(actual) != _instant(required)):
                return False
    return True


def _validate_lifecycle(item, binding):
    known = _instant(item["as_known_at"])
    _instant(item["valid_at"])
    if item.get("intent") not in ("current", "historical"):
        raise ValueError("unknown retrieval intent")
    retired = bool(binding.get("retired"))
    if item.get("current_status") != ("retired" if retired else "active"):
        raise ValueError("current lifecycle label differs from the bound record")
    if retired and item["intent"] != "historical":
        raise ValueError("retired evidence supplied as current")
    if item.get("kind") == "accepted_memory" and binding.get("accepted_at") and _instant(binding["accepted_at"]) > known:
        raise ValueError("Claim is newer than the knowledge pin")
    history = [state for state in binding.get("lifecycle") or [] if _instant(state["transaction_time"]) <= known]
    if history:
        expected = max(history, key=lambda state: (_instant(state["transaction_time"]), state["scope_revision"]))["state"]
        if item.get("status") != expected:
            raise ValueError("lifecycle at knowledge pin differs")
    elif item.get("status") not in (("active", "retired") if retired else ("active",)):
        raise ValueError("invalid original lifecycle label")


def _audit(groups, seed):
    catalog = _Catalog(seed)
    present, spans, violations = set(), {}, list(catalog.errors)
    for group_index, evidence in enumerate(groups):
        if evidence is not None and not isinstance(evidence, list):
            violations.append({"group": group_index, "code": "invalid_evidence", "detail": "evidence group is not a list"})
            continue
        for index, item in enumerate(evidence or []):
            if not isinstance(item, dict):
                violations.append({"group": group_index, "item": index, "code": "invalid_evidence", "detail": "evidence item is not an object"})
                continue
            label = {"group": group_index, "item": index, "evidence_id": item.get("id")}
            record = catalog.claims.get(item.get("claim_id"))
            if record:
                present.add(record)
            source_items = item.get("sources") or []
            for source in source_items if isinstance(source_items, list) else []:
                if not isinstance(source, dict):
                    continue
                event = source.get("event_id")
                present.update(catalog.labels.get(event, {"unresolved-event:" + str(event)}))
            try:
                pending = []
                if not isinstance(source_items, list) or not source_items:
                    raise ValueError("evidence has no original sources")
                authorized = [catalog.source(source) for source in source_items]
                known = _instant(item["as_known_at"])
                _instant(item["valid_at"])
                if any(_instant(source["observed_at"]) > known for source in source_items):
                    raise ValueError("source is newer than the knowledge pin")
                kind = item.get("kind")
                if kind == "accepted_memory":
                    if not record:
                        raise ValueError("accepted Claim has no canonical binding")
                    binding = catalog.bindings[record]
                    if item.get("id") != "claim:" + binding["claim_id"]:
                        raise ValueError("accepted evidence ID differs from Claim ID")
                    if item.get("claim_operation_id") != binding.get("claim_operation_id"):
                        raise ValueError("accepted Claim operation differs")
                    claim = item.get("claim") or {}
                    if not isinstance(claim, dict):
                        raise ValueError("structured Claim is not an object")
                    if claim and (claim.get("claim_id") != binding["claim_id"] or claim.get("created_operation_id") != binding["claim_operation_id"]):
                        raise ValueError("structured Claim identity differs")
                    if claim and binding.get("subject_entity_id") and claim.get("subject_entity_id") != binding["subject_entity_id"]:
                        raise ValueError("structured Claim subject differs")
                    original = binding["source"]
                    if len(source_items) != 1 or source_items[0].get("source_link_id") != original.get("source_link_id"):
                        raise ValueError("accepted Source Link differs")
                    if source_items[0].get("operation_id") != binding["claim_operation_id"]:
                        raise ValueError("accepted Source Link operation differs")
                    if authorized[0] != (original["event_id"], _own_span(original)[0]):
                        raise ValueError("accepted Source Link does not cover its full original locator")
                    if any(source_items[0][key] != original[key] for key in ("locator_kind", "locator_value")):
                        raise ValueError("accepted Source Link locator differs")
                    _validate_lifecycle(item, binding)
                    identity_supported = catalog.identities(item, binding)
                    if identity_supported and _historical_support(item, binding):
                        pending.append((record, authorized[0][1]))
                elif kind == "conversation_excerpt":
                    if item.get("claim_id") or item.get("claim_operation_id") or item.get("claim") or item.get("identity_matches"):
                        raise ValueError("conversation excerpt fabricated accepted/identity metadata")
                    if len(source_items) != 1 or item.get("text") != source_items[0]["evidence"]:
                        raise ValueError("conversation display differs from its one exact original span")
                    event, span = authorized[0]
                    if item.get("id") != "excerpt:" + event + ":" + str(span[0]) + ":" + str(span[1]):
                        raise ValueError("conversation evidence ID differs from its original locator")
                    if source_items[0].get("source_link_id") or source_items[0].get("operation_id"):
                        raise ValueError("conversation source fabricated an accepted link/operation")
                    # Retired corresponding intervals remain a deterministic
                    # source boundary even when this item is not a gold record.
                    corresponding = []
                    for binding in catalog.bindings.values():
                        if binding["source"]["event_id"] == event:
                            required_span = _own_span(binding["source"])[0]
                            if max(span[0], required_span[0]) < min(span[1], required_span[1]):
                                corresponding.append(binding)
                    retired = [binding for binding in corresponding if binding.get("retired")]
                    lifecycle = {"retired": bool(retired)}
                    if retired and all(binding.get("lifecycle") for binding in retired):
                        retired_at_pin = False
                        for binding in retired:
                            states = [state for state in binding["lifecycle"] if _instant(state["transaction_time"]) <= known]
                            if states:
                                latest = max(states, key=lambda state: (_instant(state["transaction_time"]), state["scope_revision"]))
                                retired_at_pin = retired_at_pin or latest["state"] == "retired"
                        lifecycle["lifecycle"] = [{"state": "retired" if retired_at_pin else "active", "transaction_time": item["as_known_at"], "scope_revision": 0}]
                    _validate_lifecycle(item, lifecycle)
                    for name, binding in catalog.bindings.items():
                        if not binding.get("claim_id") and binding["source"]["event_id"] == event and _historical_support(item, binding):
                            pending.append((name, span))
                else:
                    raise ValueError("unknown evidence kind")
                for name, span in pending:
                    spans.setdefault(name, []).append(span)
            except (KeyError, TypeError, ValueError, UnicodeError) as error:
                violations.append({**label, "code": "invalid_evidence", "detail": str(error)})
    supported = [record for record, parts in spans.items() if _covers(parts, _own_span(catalog.requirements[record])[0])]
    return {"present_records": sorted(present), "supported_records": sorted(supported), "violations": violations}


def audit_evidence(evidence, seed):
    """Audit one delivered request's evidence; never grant ID-only credit."""
    return _audit([evidence], seed)


def merge_support(evidence_groups, seed):
    """Union only validated authorized spans across actual delivered groups."""
    return _audit(evidence_groups, seed)
