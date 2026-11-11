---
id: 10-spec-field-doc-purpose
description: A spec field's doc comment must say what the field is for, not restate its name or markers.
severity: LOW
complexity: 1
scope: line
applies_to:
  - "**/api/**/*_types.go"
exclude:
  - "**/zz_generated.*.go"
  - "**/*_test.go"
context_lines: 12
---

# Spec field comments state a purpose

The doc comment on a spec field is the field's description in the CRD schema
and in `kubectl explain`. Users and agents decide what to put in the field from
that sentence alone, so it has to answer: what does setting this field do, or
what is the value used for?

A comment fails when everything it says is already visible in the field
declaration and its markers:

- the field name, re-worded: `ClientID is the OAuth2 client ID`, `Authentication configuration`
- the Go or JSON type, the optionality, or a marker's value: `Required string`, `Must match the pattern ...`

A comment passes as soon as one sentence adds something the declaration does not
carry: the behaviour the field controls, what consumes the value, what happens
when it is unset, a concrete example of a value, or a reference to the thing it
selects. Short is fine; a single clause such as "selects the gateway used for
this preset" is enough. Grammar, style and completeness are outside this rule.

The detector `.agents/tools/spec_field_docs.py --thin` lists the spec fields
whose prose adds at most one word beyond the field name, its Go type and filler
words such as "defines" or "configuration". Longer comments are passed without
a model; fields with no prose at all are `09-spec-field-doc-comment`.
The declaration and its comment span form one candidate, so changing only the
comment still triggers this check.

## Detect

```tool
command: python3 .agents/tools/spec_field_docs.py --thin
violation_when: output_lines
```

## Checks

- [name-only] Does the doc comment of the field on the candidate line only restate the field's name, type or markers, without saying what the field is for or what it changes?

## Examples

### Violation

```go source=notification/api/v1/notificationchannel_types.go:22@522cdd64
	// ClientID is the OAuth2 client ID
	// +kubebuilder:validation:Required
	ClientID string `json:"clientId"`
```

The sentence is the field name spelled out. A reader still does not know which
OAuth2 server or flow the client belongs to.

```go source=notification/api/v1/notificationchannel_types.go:186@522cdd64
	// Authentication configuration
	// +optional
	Authentication *Authentication `json:"authentication,omitempty"`
```

The type name, repeated. Nothing says what is authenticated against what.

### Allowed

```go source=admin/api/v1/zone_types.go:159@522cdd64
	// Port is the port number of the URL (e.g. 8000). If not set, the default port for the scheme is used (443 for https, 80 for http).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`
```

It opens like a restatement, but the second sentence states what happens when
the field is unset, which the markers do not say.

```go source=admin/api/v1/zone_types.go:175@522cdd64
	// Hidden controls whether this URL should be hidden from the Links section in the Zone status.
	// This can be used to hide internal-only URLs that should not be exposed to API consumers.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Hidden bool `json:"hidden"`
```

A boolean whose comment names the behaviour it switches and when to use it.
