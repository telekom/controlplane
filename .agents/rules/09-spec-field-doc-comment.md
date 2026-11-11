---
id: 09-spec-field-doc-comment
description: Every CustomResource spec field needs a doc comment with prose, not only kubebuilder markers.
severity: HIGH
complexity: 0
scope: line
applies_to:
  - "**/api/**/*_types.go"
exclude:
  - "**/zz_generated.*.go"
  - "**/*_test.go"
---

# Spec fields carry a purpose comment

The doc comment on a spec field becomes the field's `description` in the CRD
schema. Users read it in `kubectl explain` and in the generated reference, and
agents read it when they write a manifest. A field without one leaves both to
guess from the name.

This rule only decides whether prose exists. Whether the prose is useful is
`10-spec-field-doc-purpose`.

The detector `.agents/tools/spec_field_docs.py` finds the spec fields: the
exported, serialised fields of every struct named `*Spec`, and of every struct
in the same package such a field reaches. `*Status` structs are never followed,
and embedded or `json:",inline"` fields such as `metav1.ObjectMeta` are not
fields of their own. A field fails when the comment block directly above it,
together with any trailing comment, holds nothing but `+` markers.

Only changed field lines are reported, so editing a file does not inherit its
undocumented fields.

## Detect

```tool
command: python3 .agents/tools/spec_field_docs.py
violation_when: output_lines
```
