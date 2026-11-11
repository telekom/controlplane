# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Deterministic resolution of *what* is being audited.

Four explicit modes exist and they must never be conflated:

``local``
    uncommitted changes against HEAD, including staged edits and untracked
    files, using their current working-tree content.

``worktree``
    explicitly combined branch and local changes against the merge base,
    using current working-tree content.

``staged``
    only the content in the git index. This is what the commit gate checks
    receipts for.

``range``
    committed content between the target merge base and a revision, normally
    HEAD. Local edits cannot change the result.
"""

from __future__ import annotations

import hashlib
import os
import re
import shutil
import subprocess
import tempfile
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path

HUNK_PATTERN = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@")


class GitError(RuntimeError):
    pass


def git(repo_root: Path, *args: str, check: bool = True) -> str:
    result = subprocess.run(
        ["git", *args],
        cwd=repo_root,
        capture_output=True,
        text=True,
    )
    if check and result.returncode != 0:
        raise GitError(f"git {' '.join(args)} failed: {result.stderr.strip()}")
    return result.stdout


def repo_root(start: Path | None = None) -> Path:
    result = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        cwd=start or Path.cwd(),
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise GitError("not inside a git repository")
    return Path(result.stdout.strip())


def commit(root: Path, revision: str) -> str:
    """Resolve and pin a commit ref before snapshotting or comparing it."""
    return git(
        root, "rev-parse", "--verify", "--end-of-options", f"{revision}^{{commit}}"
    ).strip()


def merge_base(
    root: Path, revision: str = "HEAD", target: str | None = None,
) -> str:
    """Resolve the base commit the change should be measured against."""
    target = target or target_branch(root)
    if target is not None:
        return git(root, "merge-base", commit(root, target), commit(root, revision)).strip()
    head = git(root, "symbolic-ref", "refs/remotes/origin/HEAD", check=False).strip()
    targets = [head] if head else ["origin/main", "origin/master", "main", "master"]
    for target in targets:
        base = git(root, "merge-base", target, revision, check=False).strip()
        if base:
            return base
    raise GitError("cannot resolve a target branch; set origin/HEAD or fetch origin/main")


def target_branch(root: Path) -> str | None:
    """Explicit target overrides the branch's configured target."""
    override = os.environ.get("AUDIT_TARGET", "").strip()
    if override:
        return override
    branch = git(root, "symbolic-ref", "--short", "HEAD", check=False).strip()
    if branch:
        target = git(root, "config", "--get", f"branch.{branch}.auditTarget", check=False).strip()
        if target:
            return target
    return git(root, "config", "--get", "audit.target", check=False).strip() or None


@dataclass(frozen=True)
class ChangedFile:
    path: str
    blob_sha: str
    added_lines: tuple[int, ...]
    changed_lines: tuple[int, ...] = ()

    @property
    def touched_lines(self) -> tuple[int, ...]:
        return self.changed_lines or self.added_lines

    @property
    def is_deleted(self) -> bool:
        return self.blob_sha == ""


def _added_lines(diff: str, include_deletions: bool = False) -> dict[str, list[int]]:
    result: dict[str, list[int]] = {}
    current: str | None = None
    for line in diff.splitlines():
        if line.startswith("+++ "):
            target = line[4:].removesuffix("\t")
            if target.startswith('"'):
                escapes = {
                    b"a": b"\a", b"b": b"\b", b"f": b"\f", b"n": b"\n",
                    b"r": b"\r", b"t": b"\t", b"v": b"\v",
                    b'"': b'"', b"\\": b"\\",
                }
                raw = re.sub(
                    rb'\\([0-7]{1,3}|[abfnrtv"\\])',
                    lambda m: bytes([int(m[1], 8)]) if m[1][:1].isdigit() else escapes[m[1]],
                    target[1:-1].encode("utf-8"),
                )
                target = os.fsdecode(raw)
            current = None if target == "/dev/null" else target[2:]
            if current:
                result.setdefault(current, [])
            continue
        match = HUNK_PATTERN.match(line)
        if match and current:
            start = int(match.group(1))
            count = int(match.group(2)) if match.group(2) is not None else 1
            result[current].extend(range(start, start + count))
            if include_deletions and count == 0:
                # Deletions have no new lines; mark both sides of the gap.
                result[current].extend((max(1, start), start + 1))
    return result


def _blob_sha(root: Path, path: str, staged: bool) -> str:
    if staged:
        out = git(root, "ls-files", "--stage", "--", path, check=False).strip()
        return out.split()[1] if out else ""
    absolute = root / path
    if not absolute.is_file():
        return ""
    return git(root, "hash-object", "--no-filters", "--", path).strip()


def collect(
    root: Path, mode: str = "worktree", base: str | None = None,
    revision: str = "HEAD",
) -> list[ChangedFile]:
    """Return the changed files for the requested mode, sorted by path."""
    if mode == "staged":
        diff_args = ["diff", "--cached", "--unified=0", "--no-color", "--no-ext-diff"]
        if base is not None:
            diff_args.append(base)
    elif mode in ("local", "worktree"):
        base = base or (
            commit(root, "HEAD") if mode == "local" else merge_base(root, revision)
        )
        diff_args = ["diff", "--unified=0", "--no-color", "--no-ext-diff", base]
    elif mode == "range":
        base = base or merge_base(root, revision)
        diff_args = [
            "diff", "--unified=0", "--no-color", "--no-ext-diff", base, revision,
        ]
    else:
        raise ValueError(f"unknown changeset mode: {mode}")

    diff = git(root, "-c", "core.quotePath=true", *diff_args, "--no-renames", "--no-textconv")
    added = _added_lines(diff)
    touched = _added_lines(diff, include_deletions=True)

    if mode in ("local", "worktree"):
        # Untracked files are part of the change too; a brand new controller
        # must not escape the audit just because it was never added.
        untracked = git(root, "ls-files", "-z", "--others", "--exclude-standard").split("\0")
        for path in untracked:
            if not path or path in added or path.startswith(".agents/.audit/"):
                continue
            try:
                count = len((root / path).read_text(encoding="utf-8").splitlines())
            except (OSError, UnicodeDecodeError):
                continue
            added[path] = list(range(1, count + 1))
            touched[path] = added[path]

    files = []
    committed = (
        set(git(root, "ls-tree", "-rz", "--name-only", revision).split("\0"))
        if mode == "range" else set()
    )
    for path in sorted(added):
        if path.startswith(".agents/.audit/"):
            continue
        if mode == "range" and path not in committed:
            continue
        sha = (
            git(root, "rev-parse", f"{revision}:{path}", check=False).strip()
            if mode == "range" else _blob_sha(root, path, staged=(mode == "staged"))
        )
        if not sha:
            continue
        files.append(
            ChangedFile(
                path=path,
                blob_sha=sha,
                added_lines=tuple(sorted(set(added[path]))),
                changed_lines=tuple(sorted(set(touched.get(path, [])))),
            )
        )
    return files


def read_blob(root: Path, file: ChangedFile, staged: bool) -> str:
    if staged:
        return git(root, "cat-file", "-p", file.blob_sha)
    try:
        return (root / file.path).read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return ""


@contextmanager
def snapshot(root: Path, mode: str, revision: str = "HEAD"):
    """Export a complete, isolated tree, including detector package context."""
    with tempfile.TemporaryDirectory(prefix="code-audit-") as directory:
        target = Path(directory)
        if mode in ("staged", "range"):
            _export_blobs(root, target, mode, revision)
        else:
            paths = git(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
            for name in paths.split("\0"):
                if not name or name.startswith(".agents/.audit/"):
                    continue
                source = root / name
                if source.is_file():
                    destination = target / name
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copyfile(source, destination)
        yield target


def _export_blobs(root: Path, target: Path, mode: str, revision: str) -> None:
    """Export raw blobs, without checkout filters or archive attributes."""
    entries = git(
        root, *(
            ("ls-files", "--stage", "-z") if mode == "staged"
            else ("ls-tree", "-rz", revision)
        )
    )
    blobs = []
    for entry in entries.split("\0"):
        if not entry:
            continue
        metadata, name = entry.split("\t", 1)
        parts = metadata.split()
        file_mode, oid, stage = (
            parts if mode == "staged" else (parts[0], parts[2], "0")
        )
        if stage != "0":
            raise GitError(f"cannot audit an unresolved index entry: {name}")
        if file_mode == "160000":
            continue
        if not name.startswith(".agents/.audit/"):
            blobs.append((name, oid, file_mode))
    result = subprocess.run(
        ["git", "cat-file", "--batch"], cwd=root,
        input="".join(f"{oid}\n" for _, oid, _ in blobs).encode(),
        capture_output=True,
    )
    if result.returncode:
        raise GitError(f"cannot export blobs: {result.stderr.decode().strip()}")
    offset = 0
    for name, oid, file_mode in blobs:
        end = result.stdout.index(b"\n", offset)
        header = result.stdout[offset:end].split()
        if len(header) != 3 or header[0].decode() != oid or header[1] != b"blob":
            raise GitError(f"cannot export blob for {name}: {header!r}")
        size = int(header[2])
        content = result.stdout[end + 1:end + 1 + size]
        destination = target / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(content)
        if file_mode == "100755":
            destination.chmod(0o755)
        offset = end + 2 + size


def freeze(root: Path, tree: Path, files: list[ChangedFile]) -> list[ChangedFile]:
    """Store the exact input blobs so evidence remains verifiable after edits."""
    frozen = []
    for file in files:
        path = tree / file.path
        if not path.is_file():
            raise GitError(f"snapshot is missing {file.path}")
        result = subprocess.run(
            ["git", "hash-object", "-w", "--stdin"], cwd=root,
            input=path.read_bytes(), capture_output=True,
        )
        if result.returncode:
            raise GitError(f"cannot freeze {file.path}: {result.stderr.decode().strip()}")
        sha = result.stdout.decode().strip()
        if sha != file.blob_sha:
            raise GitError(f"{file.path} changed while the audit snapshot was created; retry")
        frozen.append(file)
    return frozen


def tree_hash(tree: Path, exclude: tuple[str, ...] = ()) -> str:
    """Bind receipts to package and other cross-file dependencies, not just a blob."""
    digest = hashlib.sha256()
    for path in sorted(p for p in tree.rglob("*") if p.is_file()):
        if any(part in exclude for part in path.relative_to(tree).parts):
            continue
        digest.update(path.relative_to(tree).as_posix().encode())
        digest.update(b"\0")
        digest.update(hashlib.sha256(path.read_bytes()).digest())
    return digest.hexdigest()[:16]
