---
id: 03-spelling
description: Changed files must be free of misspellings.
severity: LOW
complexity: 0
scope: line
applies_to:
  - "**/*.go"
  - "**/*.md"
  - "**/*.yaml"
  - "**/*.yml"
exclude:
  - "**/zz_generated.*.go"
  - "**/*.pb.go"
  - "**/go.sum"
  - "**/testdata/**"
  - "**/__snapshots__/**"
  - "CHANGELOG.md"
---

# Spelling

Spelling is decided entirely by `codespell`; no model is involved and no
judgement is required. Project vocabulary that `codespell` gets wrong belongs
in `.agents/codespell-ignore.txt`, not in an audit ignore.

If `codespell` is not installed the rule reports an engine error rather than
silently passing — a check that cannot run must never look like a check that
passed.

## Detect

```tool
command: python3 .agents/tools/spelling.py --ignore-words=.agents/codespell-ignore.txt --
violation_when: output_lines
output_format: jsonl
```
