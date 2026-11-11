---
id: 07-normalize-label-values
description: A label value taken from a Kubernetes object name must go through NormalizeLabelValue.
severity: LOW
complexity: 0
scope: line
applies_to:
  - "**/*.go"
exclude:
  - "**/*_test.go"
  - "**/zz_generated.*.go"
  - "**/*.pb.go"
  - "**/*_generated.go"
context_lines: 12
---

# Normalize label values

A Kubernetes object name may be 253 characters and may contain characters a
label value may not. A label value is capped at 63 and restricted to
alphanumerics plus `-`, `_` and `.`. Assigning one to the other is therefore
not a cast, it is a conversion that can fail — and when it fails the API
server rejects the whole write, so a single over-long zone name takes down
every child the handler creates.

`labelutil.NormalizeLabelValue` is that conversion: it lowercases, replaces
the unwanted characters, and shortens anything past 63 characters by keeping
the head and tail and hashing the middle. The hash is what makes it safe to
truncate — two long names that share a prefix still produce different labels.

Around fifty call sites in this repository already wrap the value. This rule
is about the ones that do not.

The detector only fires on the unambiguous shape: a label key assigned
directly from some object's `.Name` or `.Namespace`. A value that is a string
literal, a `string(obj.UID)`, or a variable that was normalized earlier is
left alone — there is nothing to decide, so no model is asked.

It will also catch a `client.MatchingLabels` selector built the same way, as
at `organization/internal/controller/team_controller.go:93`. A selector has to
be spelled exactly like the value it matches, so normalizing it is usually the
right fix too; if it genuinely is not, waive it.

Sibling concern, deliberately not covered: `labelutil.NormalizeNameValue` for
`ObjectMeta.Name`. Most name sites normalize inside a `Make…Name` helper, so
whether a given call site is correct cannot be read off the line. Those
helpers do not currently agree with each other — compare
`api/internal/handler/util/route_util.go:230` with
`spectre/internal/handler/util/naming.go:60` — which is a bug to fix, not a
rule to write.

## Detect

```regex
(?:LabelKey|BuildLabelKey\([^)]*\))\s*(?::|\]\s*=)\s*[A-Za-z_][\w.]*\.(?:Name|Namespace)\s*,?\s*$
```

## Examples

### Violation

```go source=application/internal/handler/application/handler.go:465@b53a2d9c
			config.BuildLabelKey("zone"):        zone.Name,
```

```go source=event/internal/handler/eventexposure/handler.go:383@b53a2d9c
			config.BuildLabelKey("zone"):        obj.Spec.Zone.Name,
```

```go source=admin/internal/handler/zone/gateway.go:75@b53a2d9c
		gateway.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
```

### Allowed

```go source=rover/internal/handler/rover/api/exposure.go:64@b53a2d9c
			config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(zoneRef.Name),
```

The same zone name, same label key, one function call apart. This is the line
the violations should look like.

```go source=spectre/internal/handler/spectreapplication_route.go:183@b53a2d9c
			config.OwnerUidLabelKey:      string(obj.UID),
```

A UID is a 36-character hex string and is a valid label value by
construction, so there is nothing to normalize. The detector does not fire on
it.
