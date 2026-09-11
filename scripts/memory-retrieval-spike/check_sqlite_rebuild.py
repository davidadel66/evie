"""Supplemental operational check; never reads questions or changes frozen gates."""
import argparse
import hashlib
import json
from pathlib import Path
import sqlite3
import struct
import tempfile
import time


def read_rows(path):
    with sqlite3.connect(Path(path).resolve().as_uri() + "?mode=ro", uri=True) as db:
        return db.execute("SELECT actor_kind,id,vector FROM vectors ORDER BY actor_kind,id").fetchall()


def digest(rows):
    result = hashlib.sha256()
    for row in rows:
        for value in row:
            value = value.encode() if isinstance(value, str) else value
            result.update(struct.pack("!Q", len(value)))
            result.update(value)
    return result.hexdigest()


def run(development, heldout, output):
    first, second = read_rows(development), read_rows(heldout)
    if not first or first != second:
        raise RuntimeError("independent corpus embedding/build generations differ")
    with tempfile.TemporaryDirectory(prefix="evie-vector-rebuild-") as temporary:
        target = Path(temporary) / "rebuilt.sqlite"
        started = time.perf_counter_ns()
        with sqlite3.connect(target) as db:
            db.execute("CREATE TABLE vectors (actor_kind TEXT, id TEXT, vector BLOB, PRIMARY KEY(actor_kind,id))")
            db.executemany("INSERT INTO vectors VALUES (?,?,?)", first)
            db.commit()
        build_ns = time.perf_counter_ns() - started
        started = time.perf_counter_ns()
        reopened = read_rows(target)
        reopen_ns = time.perf_counter_ns() - started
        if reopened != first:
            raise RuntimeError("rebuild/reopen changed persisted vectors")
        report = {
            "version": "sqlite-vector-rebuild-v1",
            "scope": "Post-comparison operational check only; no questions, ranking metrics, configuration tuning, or changed gates.",
            "rows": len(first),
            "independent_reembedding_builds_byte_identical": True,
            "rebuild_from_persisted_vectors_nanoseconds": build_ns,
            "reopen_and_compare_all_rows_nanoseconds": reopen_ns,
            "rebuild_reopen_byte_identical": True,
            "rows_sha256": digest(first),
            "database_bytes": target.stat().st_size,
            "limitation": "One supplemental rebuild sample. This is not crash injection, unique-memory accounting, or a query-latency benchmark.",
        }
        with Path(output).open("x") as stream:
            json.dump(report, stream, indent=2, allow_nan=False)
            stream.write("\n")
        print(json.dumps(report))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("development_database")
    parser.add_argument("heldout_database")
    parser.add_argument("output")
    args = parser.parse_args()
    run(args.development_database, args.heldout_database, args.output)
