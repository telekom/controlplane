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

This is a deterministic Markdown fence scan, so it is decided by the detector.
It distinguishes opening and closing fences, including unlabelled and tilde
fences within blockquotes and lists. Mermaid blocks remain exempt inside
those containers too. A fenced block that genuinely belongs on the page is handled
with a waiver, which records who accepted it and why.

## Detect

```tool
command: python3 .agents/tools/journey_fences.py --jsonl
violation_when: output_lines
output_format: jsonl
```
