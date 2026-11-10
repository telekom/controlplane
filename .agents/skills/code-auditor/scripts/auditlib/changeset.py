# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Deterministic resolution of *what* is being audited.

Two modes exist and they must never be conflated:

``worktree``
    everything that differs from the merge base, staged or not. This is what
    the agent audits at the end of a feature.

``staged``
    only the content in the git index. This is what the commit gate checks
    receipts for.
"""

from __future__ import annotations

import re
import subprocess
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


def merge_base(root: Path) -> str:
    """Resolve the base commit the change should be measured against."""
    upstream = git(root, "rev-parse", "--abbrev-ref", "@{upstream}", check=False).strip()
    if upstream:
        base = git(root, "merge-base", upstream, "HEAD", check=False).strip()
        if base:
            return base
    head = git(root, "symbolic-ref", "refs/remotes/origin/HEAD", check=False).strip()
    default = head.rsplit("/", 1)[-1] if head else "main"
    base = git(root, "merge-base", f"origin/{default}", "HEAD", check=False).strip()
    if base:
        return base
    return git(root, "rev-parse", "HEAD").strip()


@dataclass(frozen=True)
class ChangedFile:
    path: str
    blob_sha: str
    added_lines: tuple[int, ...]

    @property
    def is_deleted(self) -> bool:
        return self.blob_sha == ""


def _added_lines(diff: str) -> dict[str, list[int]]:
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
    return result


def _blob_sha(root: Path, path: str, staged: bool) -> str:
    if staged:
        out = git(root, "ls-files", "--stage", "--", path, check=False).strip()
        return out.split()[1] if out else ""
    absolute = root / path
    if not absolute.is_file():
        return ""
    return git(root, "hash-object", "--", path).strip()


def collect(root: Path, mode: str = "worktree", base: str | None = None) -> list[ChangedFile]:
    """Return the changed files for the requested mode, sorted by path."""
    if mode == "staged":
        diff_args = ["diff", "--cached", "--unified=0", "--no-color", "--no-ext-diff"]
    elif mode == "worktree":
        base = base or merge_base(root)
        diff_args = ["diff", "--unified=0", "--no-color", "--no-ext-diff", base]
    else:
        raise ValueError(f"unknown changeset mode: {mode}")

    diff = git(root, *diff_args)
    added = _added_lines(diff)

    if mode == "worktree":
        # Untracked files are part of the change too; a brand new controller
        # must not escape the audit just because it was never added.
        untracked = git(root, "ls-files", "--others", "--exclude-standard").split("\n")
        for path in (p.strip() for p in untracked):
            if not path or path in added:
                continue
            try:
                count = len((root / path).read_text(encoding="utf-8").splitlines())
            except (OSError, UnicodeDecodeError):
                continue
            added[path] = list(range(1, count + 1))

    files = []
    for path in sorted(added):
        sha = _blob_sha(root, path, staged=(mode == "staged"))
        if not sha:
            continue
        files.append(
            ChangedFile(
                path=path,
                blob_sha=sha,
                added_lines=tuple(sorted(set(added[path]))),
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
