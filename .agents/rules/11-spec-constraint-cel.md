---
id: 11-spec-constraint-cel
description: A cross-field or immutability constraint documented on a spec field must be enforced by a CEL XValidation rule.
severity: HIGH
complexity: 1
scope: line
applies_to:
  - "**/api/**/*_types.go"
exclude:
  - "**/zz_generated.*.go"
  - "**/*_test.go"
context_lines: 40
---

# Documented spec constraints are enforced in CEL

When a spec comment states a rule, a user reasonably expects the API server to
reject a manifest that breaks it. A rule that lives only in prose lets the
invalid object in, and the failure surfaces later as a reconcile error, far from
the `kubectl apply` that caused it.

Single-field constraints are covered by ordinary schema markers (`Enum`,
`Minimum`, `Pattern`, `MinItems`, `Format`, ...). This rule is about the two
kinds of constraint those markers cannot express, which need a
`+kubebuilder:validation:XValidation` rule:

- **cross-field**: one field depends on, excludes, or requires another
  ("mutually exclusive with", "exactly one of", "required when type is ...")
- **immutability**: the value may not change once set ("immutable",
  "cannot be changed"), expressed as a transition rule over `oldSelf`

The XValidation rule may sit on the field itself or on the struct that holds the
related fields; both count. A check in a webhook does not count, because CEL is
preferred over custom validation wherever it can express the constraint.

Constraints that depend on other objects in the cluster, on external systems, or
on controller state are outside this rule: CEL cannot see them. Status fields
are outside this rule too.

## Detect

```regex
(?i)^\s*//\s*[^+\s].*\b(immutable|cannot be changed|can ?not be (modified|updated)|mutually exclusive|exactly one of|at least one of|only one of|cannot be (used|combined|set) together|must not be set|required (if|when|for)|only (valid|used|allowed|applies) (if|when)|(must|may) only be set (if|when)|ignored (if|when|unless))\b
```

## Checks

- [cel-missing] Does a marked comment on a spec field or spec struct state a cross-field or immutability constraint that no `XValidation` rule shown on that field or its struct enforces?

## Examples

### Violation

```go source=admin/api/v1/environment_types.go:17@522cdd64
	// RealmName overrides the default realm name (which is the environment name).
	// When set, identity and gateway realms will be created with this name instead
	// of deriving it from the environment's metadata.name.
	// This field is immutable after first reconciliation.
	// If not set, defaults to the environment's metadata.name.
	// +kubebuilder:validation:Pattern=`^[a-z0-9]+(-[a-z0-9]+)*$`
	// +kubebuilder:validation:MaxLength=63
	// +optional
	RealmName string `json:"realmName,omitempty"`
}
```

The comment declares the field immutable, but the only markers are `Pattern`
and `MaxLength`. The immutability is enforced in the admission webhook instead
of a transition rule on `oldSelf`.

### Allowed

```go source=event/api/v1/eventconfig_types.go:100@522cdd64
// EventConfigSpec defines the desired state of EventConfig.
// Exactly one of Local or Proxy must be set: Local for a zone running its own
// event backend (Horizon), Proxy for a zone that forwards all traffic to a target zone.
// +kubebuilder:validation:XValidation:rule="has(self.local) != has(self.proxy)",message="exactly one of spec.local or spec.proxy must be set"
type EventConfigSpec struct {
	// Zone references the Zone for which this EventConfig applies.
	Zone ctypes.ObjectRef `json:"zone"`

	// OverwriteEnvironmentName overrides the environment name sent to Horizon.
	// If omitted or empty, the Control Plane environment from context is used.
	// +optional
	OverwriteEnvironmentName string `json:"overwriteEnvironmentName,omitempty"`

	// Local configures a zone that runs its own event backend (Horizon).
	// Mutually exclusive with Proxy.
	// +optional
	Local *LocalBackend `json:"local,omitempty"`
```

The field comment states the same kind of constraint, and the struct carries
the `XValidation` rule that enforces it.
