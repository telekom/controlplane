# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""A deliberately tiny YAML subset parser for rule frontmatter.

Supporting only a constrained grammar is a feature, not a limitation: it keeps
rule files uniform and machine-checkable, and it removes the PyYAML dependency
so the engine runs anywhere git runs.

Supported::

    key: scalar
    key: "quoted scalar"
    key: [a, b, "c"]
    key:
      - a
      - b
"""

from __future__ import annotations


class YamlishError(ValueError):
    pass


def _scalar(raw: str, lineno: int):
    text = raw.strip()
    if not text:
        return ""
    if text[0] in "\"'":
        quote = text[0]
        end = text.find(quote, 1)
        if end == -1:
            raise YamlishError(f"line {lineno}: unterminated {quote} string")
        return text[1:end]
    text = _strip_comment(text)
    if text in ("true", "false"):
        return text == "true"
    if text.lstrip("-").isdigit():
        return int(text)
    return text


def _strip_comment(text: str) -> str:
    idx = text.find(" #")
    if idx != -1:
        text = text[:idx]
    elif text.startswith("#"):
        text = ""
    return text.strip()


def _inline_list(raw: str, lineno: int) -> list:
    body = raw.strip()
    if not body.endswith("]"):
        raise YamlishError(f"line {lineno}: unterminated inline list")
    body = body[1:-1].strip()
    if not body:
        return []
    items, current, quote = [], "", None
    for char in body:
        if quote:
            if char == quote:
                quote = None
            else:
                current += char
            continue
        if char in "\"'":
            quote = char
        elif char == ",":
            items.append(current)
            current = ""
        else:
            current += char
    items.append(current)
    return [_scalar(item, lineno) for item in items if item.strip()]


def parse(text: str) -> dict:
    """Parse the supported YAML subset into a dict."""
    result: dict = {}
    pending_key: str | None = None

    for offset, line in enumerate(text.splitlines()):
        lineno = offset + 1
        if not line.strip() or line.lstrip().startswith("#"):
            continue

        if line.startswith((" ", "\t")):
            stripped = line.strip()
            if not stripped.startswith("- "):
                raise YamlishError(f"line {lineno}: expected a '- ' list item")
            if pending_key is None:
                raise YamlishError(f"line {lineno}: list item without a key")
            result[pending_key].append(_scalar(stripped[2:], lineno))
            continue

        if ":" not in line:
            raise YamlishError(f"line {lineno}: expected 'key: value'")

        key, _, value = line.partition(":")
        key = key.strip()
        if not key:
            raise YamlishError(f"line {lineno}: empty key")
        value = value.strip()

        if not value:
            result[key] = []
            pending_key = key
        elif value.startswith("["):
            result[key] = _inline_list(value, lineno)
            pending_key = None
        else:
            result[key] = _scalar(value, lineno)
            pending_key = None

    return result


def split_frontmatter(text: str) -> tuple[dict, str]:
    """Split a markdown document into (frontmatter dict, body)."""
    if not text.startswith("---"):
        raise YamlishError("file must start with a '---' frontmatter block")
    lines = text.splitlines()
    for index in range(1, len(lines)):
        if lines[index].strip() == "---":
            front = "\n".join(lines[1:index])
            body = "\n".join(lines[index + 1 :])
            return parse(front), body
    raise YamlishError("unterminated frontmatter block")
