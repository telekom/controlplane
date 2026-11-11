---
id: 01-prefer-slices-sort
description: Prefer the generic slices package over the legacy sort package.
severity: CRITICAL
complexity: 0
scope: line
applies_to:
  - "**/*.go"
exclude:
  - "**/zz_generated.*.go"
  - "**/*.pb.go"
  - "**/*_generated.go"
context_lines: 12
---

# Prefer slices over sort

`sort.Sort`, `sort.Slice` and friends predate generics. New and touched code
should use `slices.Sort` for ordered element types and `slices.SortFunc` with a
`cmp.Compare`-style comparator otherwise. The result is shorter, type-safe and
consistent across the monorepo.

This rule used to ask a model whether each call was replaceable. The selftest
settled that question: every `sort.*` call in this repository converts, including
the reverse-ordering case that was previously documented as an exception
(`slices.Sort` followed by `slices.Reverse`, or `slices.SortFunc` with the
arguments of `cmp.Compare` swapped). With no genuine exception left to judge,
the detector decides and the rule costs nothing to run.

If you hit a call that truly cannot convert — a type implementing
`sort.Interface` to satisfy a third-party contract, for instance — waive it.
The waiver records who accepted it and why, which is a better outcome than a
model re-deciding it on every audit.

## Detect

```regex
\bsort\.(Sort|Stable|Slice|SliceStable|Strings|Ints|Float64s)\(
```

## Examples

### Violation

```go source=event/internal/handler/eventsubscription/trigger.go:101@b53a2d9c
sort.Strings(keys)
```

```go source=admin/internal/handler/zone/links.go:52@b53a2d9c
sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
```

```go source=tools/snapshotter/pkg/store/file.go:77@b53a2d9c
sort.Sort(sort.Reverse(sort.IntSlice(versions)))
```
