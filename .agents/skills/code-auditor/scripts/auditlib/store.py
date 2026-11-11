# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Content-addressed verdict cache, commit receipts and waivers.

One key space serves three purposes:

* **cache**    - skip re-judging a snippet that was already judged
* **receipts** - let the commit gate prove an audit covers the staged blobs
* **clusters** - judge an identical snippet once, apply it everywhere

Because every key is derived from content (blob sha, rule text, model id) and
never from a timestamp, editing code or a rule invalidates exactly the entries
it should and nothing else.
"""

from __future__ import annotations

import hashlib
import json
import os
import time
from dataclasses import dataclass
from pathlib import Path

from . import judges
from .yamlish import YamlishError, parse as parse_yamlish

AUDIT_DIRNAME = ".audit"
WAIVERS_FILENAME = "auditignore"


def audit_dir(repo_root: Path) -> Path:
    return repo_root / ".agents" / AUDIT_DIRNAME


def model_id(repo_root: Path) -> str:
    """The judge the verdicts were produced by, recorded in every key.

    Resolved from the pinned judges, never from the session or the
    environment: the model asking for the audit is not the model answering it,
    and a pin that a shell can rewrite is not a pin. Use AUDIT_JUDGE to pick a
    different declared complexity.
    """
    return judges.load(repo_root).resolve().pin


def write_state(
    directory: Path, model: str, rules_index_hash: str, judge: str = "",
) -> None:
    """Record the last audit in ``directory``, the audit's state directory."""
    _save(
        directory / "state.json",
        {"model": model, "judge": judge, "rules_index_hash": rules_index_hash},
    )


def audited_model(directory: Path, config_root: Path) -> str:
    """The judge of the most recent audit.

    The commit gate reads this instead of the environment: a git hook has no
    AUDIT_MODEL set, and keying receipts off whatever the shell happens to
    export would make the gate pass or fail for the wrong reasons.
    """
    state = _load(directory / "state.json")
    configured = judges.load(config_root)
    if not state:
        return configured.resolve().pin
    identity = state.get("judge")
    model = state.get("model")
    if identity:
        current = configured.get(identity).pin
        if current == model:
            return current
    elif any(configured.get(name).pin == model for name in configured.complexities):
        return str(model)
    raise judges.JudgeError("the audited judge pin changed or was removed; re-run the audit")


def verdict_key(
    rule_content_hash: str, normalized_anchor: str, model: str, config: str = "",
) -> str:
    digest = hashlib.sha256()
    for part in (rule_content_hash, normalized_anchor, model, config):
        digest.update(part.encode("utf-8"))
        digest.update(b"\0")
    return digest.hexdigest()[:16]


def receipt_key(
    blob_sha: str, rule_content_hash: str, model: str,
    path: str = "", changed_lines: tuple[int, ...] = (), tree: str = "",
    config: str = "",
) -> str:
    digest = hashlib.sha256()
    for part in (
        "receipt-v3", path, blob_sha, rule_content_hash, model,
        ",".join(map(str, changed_lines)), tree, config,
    ):
        digest.update(part.encode("utf-8"))
        digest.update(b"\0")
    return digest.hexdigest()[:16]


def _load(path: Path) -> dict:
    if not path.is_file():
        return {}
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}
    return data if isinstance(data, dict) else {}


def _save(path: Path, data: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temp = path.with_suffix(path.suffix + ".tmp")
    temp.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temp.replace(path)


class VerdictCache:
    def __init__(self, directory: Path):
        self._path = directory / "verdicts.cache.json"
        self._data = _load(self._path)

    def get(self, key: str) -> dict | None:
        entry = self._data.get(key)
        return entry if isinstance(entry, dict) else None

    def put(self, key: str, verdict: str, evidence: dict | None, rule_id: str) -> None:
        self._data[key] = {
            "rule_id": rule_id,
            "verdict": verdict,
            "evidence": evidence,
            "recorded_at": int(time.time()),
        }

    def save(self) -> None:
        _save(self._path, self._data)


class ReceiptStore:
    """Per (file blob, rule) proof that an audit covered exactly this content."""

    def __init__(self, directory: Path):
        self._path = directory / "receipts.json"
        self._data = _load(self._path)

    def get(self, key: str) -> dict | None:
        entry = self._data.get(key)
        return entry if isinstance(entry, dict) else None

    def put(self, key: str, rule_id: str, path: str, verdict: str, severity: str) -> None:
        self._data[key] = {
            "rule_id": rule_id,
            "file": path,
            "verdict": verdict,
            "severity": severity,
            "recorded_at": int(time.time()),
        }

    def save(self) -> None:
        _save(self._path, self._data)


@dataclass(frozen=True)
class Waiver:
    item_id: str
    reason: str


class Waivers:
    """Item-keyed exemptions.

    Waivers are committed to the repository and expire by construction: the
    item id contains the snippet, so changing the code retires the waiver.
    """

    def __init__(self, config_root: Path):
        self.path = config_root / ".agents" / WAIVERS_FILENAME
        self._data: dict[str, Waiver] = {}
        self.errors: list[str] = []
        if self.path.is_file():
            self._parse(self.path.read_text(encoding="utf-8"))

    def _parse(self, text: str) -> None:
        try:
            raw = parse_yamlish(text)
        except YamlishError as exc:
            self.errors.append(f"{WAIVERS_FILENAME}: {exc}")
            return
        for key, value in raw.items():
            reason = str(value).strip()
            if not reason:
                self.errors.append(f"{WAIVERS_FILENAME}: waiver '{key}' has no reason")
                continue
            self._data[key] = Waiver(item_id=key, reason=reason)

    def get(self, item_id: str) -> Waiver | None:
        return self._data.get(item_id)

    def __len__(self) -> int:
        return len(self._data)
