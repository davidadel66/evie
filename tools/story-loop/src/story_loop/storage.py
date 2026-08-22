"""Durable state, immutable receipts, and single-owner run locking."""

from __future__ import annotations

import fcntl
import json
import os
import re
import tempfile
from pathlib import Path
from types import TracebackType
from typing import Any


class StateError(RuntimeError):
    """Raised when durable loop state is missing, corrupt, or inconsistent."""


class RunLockedError(StateError):
    """Raised when another controller already owns a run."""


_RUN_ID_RE = re.compile(r"^[a-z0-9][a-z0-9._-]{0,79}$")


def validate_run_id(run_id: str) -> str:
    if not _RUN_ID_RE.fullmatch(run_id):
        raise StateError(
            "run ID must be 1-80 lowercase letters, digits, dots, underscores, or hyphens"
        )
    return run_id


def atomic_write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(
        prefix=f".{path.name}.", suffix=".tmp", dir=path.parent
    )
    temporary_path = Path(temporary)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
            json.dump(value, stream, indent=2, sort_keys=True, allow_nan=False)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary_path, path)
        directory_fd = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
    finally:
        try:
            temporary_path.unlink()
        except FileNotFoundError:
            pass


class RunStore:
    def __init__(self, root: Path, run_id: str) -> None:
        self.run_id = validate_run_id(run_id)
        self.directory = root / self.run_id
        self.state_path = self.directory / "state.json"
        self.iterations_directory = self.directory / "iterations"
        self.lock_path = self.directory / "run.lock"

    def exists(self) -> bool:
        return self.state_path.is_file()

    def load(self) -> dict[str, Any]:
        try:
            with self.state_path.open(encoding="utf-8") as stream:
                value = json.load(stream)
        except FileNotFoundError as exc:
            raise StateError(f"run does not exist: {self.run_id}") from exc
        except (OSError, json.JSONDecodeError) as exc:
            raise StateError(f"cannot read run state {self.state_path}: {exc}") from exc
        if not isinstance(value, dict):
            raise StateError(f"run state must be a JSON object: {self.state_path}")
        return value

    def save(self, state: dict[str, Any]) -> None:
        atomic_write_json(self.state_path, state)

    def write_iteration(self, number: int, record: dict[str, Any]) -> Path:
        if number < 1:
            raise StateError("iteration numbers begin at 1")
        path = self.iterations_directory / f"{number:03d}.json"
        if path.exists():
            try:
                with path.open(encoding="utf-8") as stream:
                    existing = json.load(stream)
            except (OSError, json.JSONDecodeError) as exc:
                raise StateError(
                    f"cannot read existing iteration receipt {path}: {exc}"
                ) from exc
            if existing != record:
                raise StateError(
                    f"iteration receipt conflicts with durable state: {path}"
                )
            return path
        atomic_write_json(path, record)
        return path

    def lock(self) -> RunLock:
        return RunLock(self.lock_path)


class RunLock:
    def __init__(self, path: Path) -> None:
        self.path = path
        self._stream: Any = None

    def __enter__(self) -> RunLock:  # noqa: PYI034
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._stream = self.path.open("a+", encoding="utf-8")
        try:
            fcntl.flock(self._stream.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            self._stream.close()
            self._stream = None
            raise RunLockedError(
                f"run is already active: {self.path.parent.name}"
            ) from exc
        self._stream.seek(0)
        self._stream.truncate()
        self._stream.write(f"pid={os.getpid()}\n")
        self._stream.flush()
        return self

    def __exit__(
        self,
        _exc_type: type[BaseException] | None,
        _exc: BaseException | None,
        _traceback: TracebackType | None,
    ) -> None:
        if self._stream is None:
            return
        fcntl.flock(self._stream.fileno(), fcntl.LOCK_UN)
        self._stream.close()
        self._stream = None
