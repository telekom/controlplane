#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""List the CustomResource spec fields declared in Go API type files.

A spec field is an exported, serialised field of a struct named ``*Spec``, or
of a struct in the same package that a spec field reaches. Status types are
never followed. Types from other packages are out of scope.

Each reported line reads ``path:line: message`` so the audit engine can use
it as a tool detector.

Usage::

    spec_field_docs.py [--thin] FILE...

Without ``--thin`` the fields that carry no purpose comment are reported:
their doc comment is absent or holds only ``+`` markers. With ``--thin`` the
documented fields whose prose adds at most one word beyond the field name,
its type and filler words are reported, as candidates for a name-only
restatement.
"""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path

TYPE_START = re.compile(r"^type\s+(\w+)\s+struct\s*\{\s*$")
FIELD = re.compile(
    r"^\s*(?P<names>[A-Z]\w*(?:\s*,\s*[A-Z]\w*)*)\s+(?P<type>[^`/]+?)\s*"
    r"(?P<tag>`[^`]*`)?\s*(?P<trailing>//.*)?$"
)
EMBEDDED = re.compile(r"^\s*\*?(?P<type>[\w.]+)\s*(?P<tag>`[^`]*`)?\s*(//.*)?$")
JSON_TAG = re.compile(r'json:"(?P<value>[^"]*)"')
IDENT = re.compile(r"[A-Za-z_]\w*(?:\.\w+)?")
CAMEL = re.compile(r"(?<=[a-z0-9])(?=[A-Z])|(?<=[A-Z])(?=[A-Z][a-z])")
WORD = re.compile(r"[A-Za-z0-9]+")
THIN_NOVEL_WORDS = 1
FILLER = frozenset(
    "a an the of for to in on by with and or is are be this that it its "
    "defines specifies contains holds represents describes sets configures "
    "configuration config settings setting field value values optional required "
    "use used string list map name names".split()
)


@dataclass
class Field:
    name: str
    line: int
    type_names: list[str]
    documented: bool
    serialised: bool
    prose: str = ""
    type_expr: str = ""


@dataclass
class Struct:
    name: str
    path: Path
    fields: list[Field] = field(default_factory=list)


def _type_names(expr: str) -> list[str]:
    """Same-package type identifiers referenced by a field type expression."""
    names = []
    for token in IDENT.findall(expr.replace("map[", " ").replace("]", " ")):
        if "." in token or token in {"map", "chan", "func", "interface", "struct"}:
            continue
        names.append(token)
    return names


def _serialised(tag: str | None) -> bool:
    if not tag:
        return True
    match = JSON_TAG.search(tag)
    if not match:
        return True
    name, _, options = match.group("value").partition(",")
    return name != "-" and "inline" not in options.split(",")


def _prose(comments: list[str]) -> str:
    """The comment text that is not a ``+`` marker."""
    kept = []
    for comment in comments:
        text = comment.lstrip()[2:].strip()
        if text and not text.startswith("+"):
            kept.append(text)
    return " ".join(kept)


def _words(text: str) -> list[str]:
    spaced = CAMEL.sub(" ", text)
    return [w.lower() for w in WORD.findall(spaced)]


def novel_words(item: Field) -> list[str]:
    """Prose words that the field name, its type and filler words do not cover."""
    known = set(_words(item.name)) | set(_words(item.type_expr)) | FILLER
    known |= {item.name.lower()} | {w.lower() for w in WORD.findall(item.type_expr)}
    return [w.lower() for w in WORD.findall(item.prose) if w.lower() not in known]


def is_thin(item: Field) -> bool:
    return len(novel_words(item)) <= THIN_NOVEL_WORDS


def parse_structs(path: Path) -> list[Struct]:
    """Every top-level struct in a Go file, with its direct fields."""
    lines = path.read_text(encoding="utf-8").splitlines()
    structs: list[Struct] = []
    current: Struct | None = None
    depth = 0
    comments: list[str] = []

    for number, raw in enumerate(lines, start=1):
        line = raw.rstrip()
        if current is None:
            match = TYPE_START.match(line)
            if match:
                current = Struct(match.group(1), path)
                depth = 1
                comments = []
            continue

        stripped = line.strip()
        opened, closed = stripped.count("{"), stripped.count("}")
        if depth == 1 and stripped == "}":
            structs.append(current)
            current = None
            continue
        if depth > 1:
            depth += opened - closed
            continue
        if stripped.startswith("//"):
            comments.append(stripped)
            continue
        if not stripped:
            comments = []
            continue

        field_match = FIELD.match(line)
        embedded = None if field_match else EMBEDDED.match(line)
        if field_match:
            trailing = field_match.group("trailing")
            prose = _prose(comments + ([trailing] if trailing else []))
            for name in re.split(r"\s*,\s*", field_match.group("names")):
                current.fields.append(
                    Field(
                        name=name,
                        line=number,
                        type_names=_type_names(field_match.group("type")),
                        documented=bool(prose),
                        serialised=_serialised(field_match.group("tag")),
                        prose=prose,
                        type_expr=field_match.group("type"),
                    )
                )
        elif embedded:
            current.fields.append(
                Field(
                    name="",
                    line=number,
                    type_names=_type_names(embedded.group("type")),
                    documented=True,
                    serialised=False,
                )
            )
        depth += opened - closed
        comments = []
    return structs


def _package_files(directory: Path) -> list[Path]:
    return sorted(
        p
        for p in directory.glob("*.go")
        if not p.name.endswith("_test.go") and not p.name.startswith("zz_generated")
    )


def spec_structs(directory: Path) -> dict[str, Struct]:
    """Structs of a package reachable from a ``*Spec`` struct."""
    by_name: dict[str, Struct] = {}
    for path in _package_files(directory):
        for struct in parse_structs(path):
            by_name.setdefault(struct.name, struct)

    pending = [name for name in by_name if name.endswith("Spec")]
    reached: dict[str, Struct] = {}
    while pending:
        name = pending.pop()
        if name in reached or name not in by_name or name.endswith("Status"):
            continue
        reached[name] = by_name[name]
        for item in by_name[name].fields:
            pending.extend(item.type_names)
    return reached


def report(paths: list[str], thin: bool) -> list[str]:
    """Undocumented spec fields, or with ``thin`` the documented ones whose prose
    adds at most ``THIN_NOVEL_WORDS`` words to the field name and type."""
    out = []
    cache: dict[Path, dict[str, Struct]] = {}
    for raw in paths:
        path = Path(raw)
        if path.suffix != ".go" or not path.is_file():
            continue
        directory = path.resolve().parent
        if directory not in cache:
            cache[directory] = spec_structs(directory)
        for struct in cache[directory].values():
            if struct.path.resolve() != path.resolve():
                continue
            for item in struct.fields:
                if not item.serialised:
                    continue
                if thin and item.documented and is_thin(item):
                    state = f"comment adds only {novel_words(item)} to the name"
                elif not thin and not item.documented:
                    state = "has no purpose comment"
                else:
                    continue
                out.append(
                    f"{raw}:{item.line}: spec field {struct.name}.{item.name} {state}"
                )
    return sorted(
        out, key=lambda entry: (entry.split(":")[0], int(entry.split(":")[1]))
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--thin", action="store_true")
    parser.add_argument("files", nargs="*")
    args = parser.parse_args(argv)
    for entry in report(args.files, args.thin):
        print(entry)
    return 0


if __name__ == "__main__":
    sys.exit(main())
