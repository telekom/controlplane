---
id: 17-merge-external-labels
description: Label writes on Kubernetes-native objects that may already exist must merge into their current labels, never replace the map.
severity: CRITICAL
complexity: 1
scope: line
applies_to:
  - "**/*.go"
exclude:
  - "**/*_test.go"
  - "**/zz_generated.*.go"
  - "**/*.pb.go"
  - "**/*_generated.go"
  - "**/testutil/**"
  - "**/test/**"
context_lines: 25
---

# Merge labels on Kubernetes-native objects

Built-in Kubernetes objects — `Namespace`, `Secret`, `ConfigMap`,
`ServiceAccount`, `Deployment`, anything from a `k8s.io/api/...` package — are
shared with the platform around us. Kyverno policies, Pod Security admission
(`pod-security.kubernetes.io/*`), service meshes, GitOps tooling and the API
server itself put labels on them. Kubernetes treats `metadata.labels` as one
map, so a write that replaces the map deletes every label we did not put there
ourselves. The next policy run adds them back, our next reconcile removes them
again, and the object flaps forever. Nothing fails; the platform silently
loses its selector — or, for a Namespace, its security policy.

So on a native object our labels are merged into whatever is already there:
read the current map, create it only if it is `nil`, and set our keys one by
one.

```go
if namespace.Labels == nil {
	namespace.Labels = make(map[string]string)
}
namespace.Labels[config.EnvironmentLabelKey] = envName
maps.Copy(namespace.Labels, owner.GetLabels())
```

## Scope

**Only Kubernetes-native kinds.** The controlplane's own custom resources —
anything in a `*.cp.ei.telekom.de` API group, such as `ApiExposure`, `Route`,
`Consumer`, `Client`, `Approval` — are owned by the controlplane alone. For
those, replacing the label map is allowed and often intended: it also drops
labels the controlplane no longer sets. Do not report them.

Decide the kind from what the context shows:

- a `corev1.`, `appsv1.`, `batchv1.`, `networkingv1.`, `rbacv1.` (or other
  `k8s.io/api`) type is native;
- so is an object whose constructor, variable or handler names a built-in kind
  unambiguously — `buildNamespaceObj(...)`, `nsObj` in a `NamespaceHandler`,
  a `secret` built by `NewSecretObj`;
- anything else is **not** native. Nearly every object a handler here writes is
  a controlplane custom resource, and the few native ones are visibly typed or
  named. An object whose kind the context does not reveal — `identityClient`,
  `route`, `consumer` — is out of scope: answer no, not unclear.

Within that scope the rule applies wherever the object may already exist in the
cluster — inside a `CreateOrUpdate` mutator (controller-runtime fetches the
live object before calling it), before an `Update`, or in a helper that
receives such an object.

What replaces the map on a native object, and is a finding:

- `obj.Labels = map[string]string{...}` with any keys;
- `obj.Labels = owner.Labels`, `= owner.GetLabels()`, `= maps.Clone(...)` — a
  borrowed or freshly built map, even if more keys are written into it
  afterwards;
- `obj.SetLabels(x)` where `x` was not obtained from `obj.GetLabels()`.

What is fine:

- `obj.Labels = make(map[string]string)` or `= map[string]string{}` guarded by
  `if obj.Labels == nil` — there was nothing to lose;
- `labels := obj.GetLabels()` … `obj.SetLabels(labels)` — the same map comes
  back, extended;
- labels in the `ObjectMeta` literal of an object that is only ever passed to
  `Create` — nothing exists yet.

Merging means a label we stop setting is not removed automatically. That is
the accepted trade-off: delete such a key explicitly with
`delete(obj.Labels, key)` where it matters.

## Detect

Every whole-map write goes through one of two shapes, so the detector is their
union. `==` and `!=` comparisons do not match. Whether the object is native
cannot be read off the line, so every hit is judged.

```regex
\.Labels\s*=[^=]|SetLabels\(
```

## Checks

- [replace-label-map] when=`\.Labels\s*=[^=]` Does a touched line use the assignment `x.Labels = ...` (not a `SetLabels` call) to give a new map to an object that the context shows is a Kubernetes-native kind and that may already exist in the cluster, other than an empty map inside an `if x.Labels == nil` guard?
- [setlabels-replace] when=`SetLabels\(` Does a touched line call `x.SetLabels(...)` on an object that the context shows is a Kubernetes-native kind and that may already exist in the cluster, with a map that is not `x`'s own `GetLabels()` result or an empty map inside a `GetLabels() == nil` guard?

## Examples

### Violation

```go source=organization/internal/handler/team/handler/namespace/namespace.go:23@14d7cd4f
	nsObj := buildNamespaceObj(buildNamespaceName(env, owner))

	mutator := func() error {
		nsObj.SetLabels(owner.GetLabels())
		return nil
	}
```

`buildNamespaceObj` returns a `*corev1.Namespace`. The live Namespace is
fetched before the mutator runs; `SetLabels` throws away every label on it —
Pod Security, Kyverno — and shares the `Team`'s map in its place.

```go source=admin/internal/handler/remoteorganization/handler.go:31@14d7cd4f
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: calculateName(envName, obj.Spec.Id),
		},
	}

	mutator := func() error {
		namespace.Labels = map[string]string{
			config.EnvironmentLabelKey: envName,
		}
		return nil
	}
```

A `corev1.Namespace` whose labels are reset to exactly one key on every
reconcile.

### Allowed

```go source=admin/internal/handler/zone/handlingContext.go:75@14d7cd4f
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.ToLower(fmt.Sprintf("%s--%s", environment.Name, obj.Name)),
		},
	}

	mutator := func() error {
		if namespace.Labels == nil {
			namespace.Labels = make(map[string]string)
		}
		namespace.Labels[cconfig.EnvironmentLabelKey] = environment.Name
		namespace.Labels[cconfig.BuildLabelKey(zoneLabelName)] = obj.Name
		namespace.Labels[cconfig.DomainLabelKey] = domainName
```

The same kind in the same shape of mutator as the second violation. The map is
only created when there is none; every other write is a single key.

```go source=organization/internal/handler/team/handler/identity_client/identity_client.go:37@14d7cd4f
	mutate := func() error {
		identityClient.Spec.ClientId = MakeClientId(owner)
		identityClient.Spec.ClientSecret = owner.Spec.Secret
		identityClient.Spec.Realm = zoneObj.Status.TeamApiIdentityRealm
		identityClient.SetLabels(owner.GetLabels())
```

Character for character the first violation's `SetLabels(owner.GetLabels())`,
from a sibling handler of the same `Team` controller. But nothing here says
`identityClient` is a built-in kind — it is an identity `Client`, a controlplane
custom resource — so replacing its labels is out of scope.

```go source=rover/internal/handler/rover/api/exposure.go:57@14d7cd4f
	mutator := func() error {
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

A whole-map assignment in a mutator, like the second violation — on an
`ApiExposure`, which the controlplane owns outright.
