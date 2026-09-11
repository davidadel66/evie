#!/usr/bin/env python3
"""Read-only audit of recorded dense experiments, including preserved failures."""

import gzip
import hashlib
import json
import math
from pathlib import Path
import tarfile


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def verify(root):
    verified = []
    versions = {"v1": ["development-v1", "heldout-v1"],
                "v2": ["development-v2", "heldout-regression-v2"]}
    for version, names in versions.items():
        frozen = root / version / ("frozen-" + version)
        freeze_bytes = (frozen / "freeze.json").read_bytes()
        freeze = json.loads(freeze_bytes)
        if sha256((root / version / "run.py").read_bytes()) != freeze["runner_sha256"]:
            raise RuntimeError("frozen runner changed")
        compile((root / version / "run.py").read_text(), "run.py", "exec")
        archive = json.loads((frozen / "source-archive.json").read_text())
        archive_path = frozen / archive["archive"]
        if sha256(archive_path.read_bytes()) != archive["sha256"]:
            raise RuntimeError("source archive changed")
        seen = set()
        with tarfile.open(archive_path, "r:gz") as stream:
            for member in stream:
                if not member.isfile() or member.name not in archive["member_sha256"]:
                    raise RuntimeError("unexpected source archive entry")
                if sha256(stream.extractfile(member).read()) != archive["member_sha256"][member.name]:
                    raise RuntimeError("source archive entry changed")
                seen.add(member.name)
        if seen != set(archive["member_sha256"]):
            raise RuntimeError("missing source archive entry")
        if json.loads((frozen / "source-archive-verification.json").read_text())["exit_code"]:
            raise RuntimeError("source archive did not compile offline")
        for name in names:
            path = root / version / name
            samples = json.loads((path / "samples.json").read_text())
            report = json.loads((path / "report.json").read_text())
            count = requests = 0
            seen_associations = set()
            with gzip.open(path / "traces.ndjson.gz", "rt") as stream:
                for line in stream:
                    trace = json.loads(line)
                    sample = trace["sample"]
                    if sample != samples[count]:
                        raise RuntimeError("compact/raw trace mismatch")
                    marker = '"complete_requests":['
                    position = line.index(marker) + len(marker)
                    for request, expected_bytes in zip(trace["complete_requests"], sample["complete_request_bytes"], strict=True):
                        _, end = json.JSONDecoder().raw_decode(line, position)
                        if len(line[position:end].encode()) != expected_bytes:
                            raise RuntimeError("encoded provider request byte mismatch")
                        position = end + 1
                        requests += 1
                        for message in request.get("messages", []):
                            if message.get("content", "").startswith("EVIE_MEMORY_DATA\n"):
                                data = json.loads(message["content"].split("\n", 1)[1])
                                if data["evidence"] != trace["evidence"]:
                                    raise RuntimeError("provider evidence differs from retained evidence")
                    if len(trace["evidence"] or []) != len(sample["ids"] or []):
                        raise RuntimeError("source ID count mismatch")
                    if trace["receipt"] is not None and len(trace["receipt"]["evidence"] or []) != len(trace["evidence"] or []):
                        raise RuntimeError("receipt evidence count mismatch")
                    receipt = {ref["id"]: ref for ref in (trace["receipt"] or {}).get("evidence") or []}
                    for item in trace["evidence"] or []:
                        if not item.get("claim_id"):
                            continue
                        for source in item["sources"]:
                            link = source.get("source_link_id")
                            if not link or not any(ref.get("source_link_id") == link and ref["event_id"] == source["event_id"] for ref in receipt[item["id"]]["sources"]):
                                raise RuntimeError("supplied Source Link differs from durable receipt")
                            seen_associations.add((item["claim_id"], link, source["event_id"]))
                    count += 1
            if count != 192 or requests != 384:
                raise RuntimeError("missing measured turns or requests")
            for method in ["lexical", "hybrid"]:
                rows = [row for row in samples if row["method"] == method]
                latency = sorted(row["whole_turn_ns"] for row in rows)
                measured = report["methods"][method]
                if measured["matched"] != sum(row["matched"] for row in rows):
                    raise RuntimeError("reported recall differs from exact evidence IDs")
                paraphrases = [row for row in rows if row["category"] == "paraphrase"]
                recall = sum(row["matched"] for row in paraphrases) / sum(len(row["expected"]) for row in paraphrases)
                if recall != measured["paraphrase_recall"]:
                    raise RuntimeError("reported paraphrase recall differs")
                if measured["whole_turn"]["p95"] != latency[math.ceil(.95 * len(latency)) - 1]:
                    raise RuntimeError("reported nearest-rank percentile differs")
                if any(row["matched"] != len(row["expected"]) for row in rows if row["category"] == "lexical_control"):
                    raise RuntimeError("unreported lexical control miss")
            if report["freeze_sha256"] != sha256(freeze_bytes):
                raise RuntimeError("run's freeze reference changed")
            associations = json.loads((path / "supplied-source-associations.json").read_text())
            if associations["derived_from_trace_sha256"] != sha256((path / "traces.ndjson.gz").read_bytes()):
                raise RuntimeError("association supplement references changed traces")
            if {(row["claim_id"], row["source_link_id"], row["source_event_id"]) for row in associations["associations"]} != seen_associations:
                raise RuntimeError("supplement invented or omitted an observed association")
            mapping = json.loads((path / "source-mapping.json").read_text())
            all_claims = {key + "/claim" for key, value in mapping.items() if "claim_id" in value}
            observed_claims = {row["fixture_id"] for row in associations["associations"]}
            if set(associations["unobserved_fixture_ids"]) != all_claims - observed_claims:
                raise RuntimeError("unobserved Source Link omissions were not explicit")
            minimum_passed = report["methods"]["hybrid"]["paraphrase_recall"] >= .85
            if report["gates"]["paraphrase_recall_at_least_85_percent"] != minimum_passed:
                raise RuntimeError("recall gate was weakened or incorrectly reported")
            verified.append({"partition": name, "turns": count, "requests": requests,
                             "paraphrase_minimum_passed": minimum_passed})
    return {"verified": verified, "turns": sum(row["turns"] for row in verified),
            "exact_encoded_requests": sum(row["requests"] for row in verified),
            "source_archives_verified": len(versions)}


if __name__ == "__main__":
    print(json.dumps(verify(Path(__file__).resolve().parent), indent=2))
