---
id: 06-rover-child-labels
description: Sub-resources created from a Rover must carry zone, application and their domain identity label.
severity: HIGH
complexity: 1
scope: file
applies_to:
  - "rover/internal/handler/**/*.go"
exclude:
  - "**/*_test.go"
  - "**/suite_test.go"
context_lines: 60
---

# Rover sub-resource labels

Everything downstream of a Rover is found by label selector: the UI lists an
application's exposures, the janitor sweeps orphans per zone, and operators
debug a tenant by selecting on team. A child created without its labels still
reconciles correctly, so nothing fails — it just becomes invisible to every
consumer that does not already know its name. That is why this is caught at
review time and not by a test.

A sub-resource created from a Rover carries:

- `config.BuildLabelKey("zone")` — the target zone, from `zoneRef.Name` or
  `owner.Spec.Zone`
- `config.BuildLabelKey("application")` — the owning Rover, `owner.Name`
- its **domain identity label**, where the kind has one:
  `apiapi.BasePathLabelKey`, `eventv1.EventTypeLabelKey`,
  `agenticv1.AgenticBasePathLabelKey`, `agenticv1.AgentBasePathLabelKey`,
  `filev1.FileTypeNameLabelKey`

Values go through `labelutil.NormalizeLabelValue` — a base path or event type
is not a valid label value as written. That part is enforced deterministically
by `07-normalize-label-values`, so this rule does not ask about it.

Two things are deliberately **not** required.

`environment` is injected by the scoped client after the mutator runs
(`common/pkg/client/client.go:65`), so a handler that sets it by hand is
duplicating infrastructure, not following the rule.

`group` is set on the Rover itself by `rover-server`'s `EnsureLabelsOrDie` and
is not currently propagated to children. Propagate it if you have it, but no
check enforces it yet.

## Scope of the judgement

The detector fires wherever a handler takes ownership of an object. Not every
hit is a Rover child.

The `*specification` handlers — `apispecification`, `eventspecification`,
`filespecification`, `mcpspecification`, `agentspecification` — own **shared**
objects: `Api`, `EventType`, `FileType`, `McpServer`, `AgentCard`. One of
these is not scoped to a single application in a single zone, so it carries
the identity label alone. That is correct, not a finding.

The rule applies to objects owned by a `Rover` or by an `Application` derived
from one. Look at what is passed as the owner.

## Detect

Taking ownership is what makes an object a Rover child, so the controller
reference is the detector. It stays at least as broad as the `when=` patterns
below — here there are none, both checks always run on a candidate.

```regex
SetControllerReference\(
```

## Checks

- [missing-ownership-labels] Does the change create a Rover-owned sub-resource whose labels omit the `zone` key, the `application` key, or both?
- [missing-identity] Does the change create a sub-resource of a kind that has a domain identity label — base path, event type, file type — without setting it?

## Examples

### Violation

```go source=rover/internal/handler/rover/spectre/handler.go:140@b53a2d9c
	app := &spectrev1.SpectreApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeSpectreAppName(rover.Name),
			Namespace: rover.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(rover, app, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference on SpectreApplication")
		}
```

A Rover-owned child with no labels at all: it gets `environment` from the
scoped client and nothing else, so no zone or application selector finds it.
`SpectreApplication` has no identity label, so only the first check fires.

```go source=rover/internal/handler/rover/file/exposure.go:58@b53a2d9c
		if err := controllerutil.SetControllerReference(owner, fileExposure, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		fileExposure.Labels = map[string]string{
			config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(owner.Name),
		}
```

Zone and application are present, but the file-type identity label is not —
`filev1.FileTypeNameLabelKey` exists and every sibling domain sets its
equivalent.

### Allowed

```go source=rover/internal/handler/rover/api/exposure.go:58@b53a2d9c
		err = controllerutil.SetControllerReference(owner, apiExposure, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}
		apiExposure.Labels = map[string]string{
			apiapi.BasePathLabelKey:             labelutil.NormalizeLabelValue(exp.BasePath),
			config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(owner.Name),
		}
```

The full set, each value normalized. `environment` is absent on purpose.

```go source=rover/internal/handler/apispecification/handler.go:54@b53a2d9c
		err := controllerutil.SetControllerReference(apiSpec, api, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		api.Labels = map[string]string{
			apiapi.BasePathLabelKey: labelutil.NormalizeLabelValue(apiSpec.Spec.BasePath),
		}
```

Identity label only, and correct: `Api` is a shared object owned by an
`ApiSpecification`, not a child of one application in one zone. This is the
block that looks closest to the `file/exposure.go` violation — the difference
is the kind being created, not the shape of the code.
