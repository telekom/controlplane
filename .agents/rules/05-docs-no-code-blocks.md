---
id: 05-docs-no-code-blocks
description: Journey docs must illustrate with diagrams, never with code blocks.
severity: HIGH
complexity: 0
scope: line
applies_to:
  - "docs/docs/user-journey/**/*.md"
  - "docs/docs/user-journey/**/*.mdx"
  - "docs/docs/admin-journey/**/*.md"
  - "docs/docs/admin-journey/**/*.mdx"
---

# No code blocks in journey docs

A fenced block in a journey guide is almost always YAML, Go or a shell command
that belongs in the reference or developer docs. `mermaid` is the exception:
diagrams are what these pages should use instead.

This is a pure pattern match with no exceptions worth judging, so it is decided
by the detector. A fenced block that genuinely belongs on the page is handled
with a waiver, which records who accepted it and why.

## Detect

```regex
^```(?!mermaid\s*$)(?!\s*$).*$
```
