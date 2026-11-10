---
id: 04-over-technical-docs
description: User-facing journey docs must stay readable for non-engineers.
severity: HIGH
tier: 2
scope: file
report_scope: changed
applies_to:
  - "docs/docs/user-journey/**/*.md"
  - "docs/docs/user-journey/**/*.mdx"
  - "docs/docs/admin-journey/**/*.md"
  - "docs/docs/admin-journey/**/*.mdx"
exclude:
  - "**/_category_.json"
context_lines: 200
---

# Over-technical user documentation

The journey guides are read by API providers and consumers, not by operator
developers. They explain what someone wants to achieve and the steps to get
there. Architecture and reference material has its own home and is exempt from
this rule.

Mechanical constraints (no code blocks, diagrams present) are enforced
separately by `05-docs-no-code-blocks`. What is left here needs a reader's
judgement, so keep it to that.

Judge the marked lines. The rest of the document is context — read it to see
what the guide already says, but a finding must sit on a line this change
touched. A contributor who fixed one paragraph does not inherit the backlog of
the twenty they did not.

Naming a thing the reader works with is not jargon: `Rover file`,
`Application`, `roverctl` and `Approval` are the product's own vocabulary.
Jargon is the machinery behind them.

## Checks

- [jargon] Do the marked lines use implementation vocabulary (CRD, custom resource, reconcile, controller, operator, finalizer, webhook, handler, admission, namespace, `resource` as a Kubernetes object) where a product term would do?
- [sentence-length] Do the marked lines contain a sentence of roughly 35 words or more, or one stacking three or more subordinate clauses?
- [vocabulary] Do the marked lines name a concept differently from how the rest of the document already names it?
- [diagram] Do the marked lines add a multi-step flow written as prose, which a mermaid diagram would carry better?

## Examples

### Violation

```md source=docs/docs/user-journey/approvals.mdx:63@b53a2d9c
- **Active scopes** reflect the last approved set the subscription controller
  successfully wrote to the downstream resources. Gateway or event-delivery
  readiness is reported separately.
```

```md source=docs/docs/user-journey/applications.mdx:11@b53a2d9c
Each Rover file maps to exactly one Application. When the Rover operator processes your file, it creates an Application resource that:
```

### Allowed

````md source=docs/docs/user-journey/exposing-apis.mdx:7@b53a2d9c
Exposing an API is a two-step process. First, you register your API specification so the platform knows about your API. Then, you declare the exposure in a Rover file to configure routing and access control. Both steps are applied in the same way using `roverctl apply`.
````
