# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Deterministic resolution of *what* is being audited.

Three modes exist and they must never be conflated:

``worktree``
    everything that differs from the merge base, staged or not. This is what
    the agent audits at the end of a feature.

``staged``
    only the content in the git index. This is what the commit gate checks
    receipts for.

``range``
    committed content between the target merge base and a revision, normally
    HEAD. Local edits cannot change the result.
"""

from __future__ import annotations

import hashlib
import io
import re
import shutil
import subprocess
import tarfile
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


def merge_base(root: Path, revision: str = "HEAD") -> str:
    """Resolve the base commit the change should be measured against."""
    head = git(root, "symbolic-ref", "refs/remotes/origin/HEAD", check=False).strip()
    targets = [head] if head else ["origin/main", "origin/master", "main", "master"]
    for target in targets:
        base = git(root, "merge-base", target, revision, check=False).strip()
        if base:
            return base
    raise GitError("cannot resolve a target branch; set origin/HEAD or fetch origin/main")


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
            target = line[4:].strip()
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
    return git(root, "hash-object", "--", path).strip()


def collect(
    root: Path, mode: str = "worktree", base: str | None = None,
    revision: str = "HEAD",
) -> list[ChangedFile]:
    """Return the changed files for the requested mode, sorted by path."""
    if mode == "staged":
        diff_args = ["diff", "--cached", "--unified=0", "--no-color", "--no-ext-diff"]
    elif mode == "worktree":
        base = base or merge_base(root, revision)
        diff_args = ["diff", "--unified=0", "--no-color", "--no-ext-diff", base]
    elif mode == "range":
        base = base or merge_base(root, revision)
        diff_args = [
            "diff", "--unified=0", "--no-color", "--no-ext-diff", base, revision,
        ]
    else:
        raise ValueError(f"unknown changeset mode: {mode}")

    diff = git(root, *diff_args)
    added = _added_lines(diff)
    touched = _added_lines(diff, include_deletions=True)

    if mode == "worktree":
        # Untracked files are part of the change too; a brand new controller
        # must not escape the audit just because it was never added.
        untracked = git(root, "ls-files", "--others", "--exclude-standard").split("\n")
        for path in (p.strip() for p in untracked):
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
        set(git(root, "ls-tree", "-r", "--name-only", revision).splitlines())
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
        if mode == "staged":
            git(root, "checkout-index", "--all", f"--prefix={target}/")
        elif mode == "range":
            archive = subprocess.run(
                ["git", "archive", revision], cwd=root, capture_output=True
            )
            if archive.returncode:
                raise GitError(f"cannot snapshot {revision}: {archive.stderr.decode().strip()}")
            with tarfile.open(fileobj=io.BytesIO(archive.stdout)) as tree:
                tree.extractall(target, filter="data")
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


def tree_hash(tree: Path) -> str:
    """Bind receipts to package and other cross-file dependencies, not just a blob."""
    digest = hashlib.sha256()
    for path in sorted(p for p in tree.rglob("*") if p.is_file()):
        digest.update(path.relative_to(tree).as_posix().encode())
        digest.update(b"\0")
        digest.update(hashlib.sha256(path.read_bytes()).digest())
    return digest.hexdigest()[:16]
