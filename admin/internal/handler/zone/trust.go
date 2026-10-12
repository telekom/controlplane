// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	serverclient "github.com/telekom/controlplane/common-server/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/providerca"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/urlutil"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
)

const (
	// AnnotationPrefix prefixes all Admin provenance and publication annotations.
	AnnotationPrefix = "admin.cp.ei.telekom.de/"

	// AnnotationDiscoveryIssuer is the effective discovery issuer base published with an IdentityProvider.
	AnnotationDiscoveryIssuer = "admin.cp.ei.telekom.de/discovery-issuer"

	// ConditionTypeTrustSourcesReady reports synchronization of requested provider CA sources.
	ConditionTypeTrustSourcesReady = "TrustSourcesReady"

	ReasonTrustSynchronized  = "Synchronized"
	ReasonTrustSourceInvalid = "SourceInvalid"
	ReasonTrustCopyInvalid   = "CopyInvalid"

	discoveryTimeout = 10 * time.Second
)

// zoneOrigin is the origin of the Zone CA bundle: the Zone identity without a single source.
func zoneOrigin(zone *adminv1.Zone) providerca.ProviderCAOrigin {
	return providerca.ProviderCAOrigin{ZoneUID: string(zone.UID), ZoneNamespace: zone.Namespace, ZoneName: zone.Name}
}

func sourceKey(ref *types.CABundleConfigMapRef) string { return ref.Name + "/" + ref.Key }

// trustSource is the prepared Zone CA bundle: the validated union of all sources. Once written,
// Bundle is the content read back from its durable copy.
type trustSource struct {
	Origin  providerca.ProviderCAOrigin
	Bundle  *serverclient.ProviderCABundle
	Sources int
}

// PublishedRef returns the reference consumers use for the durable Zone bundle.
func (t *trustSource) PublishedRef() *types.CABundleConfigMapRef {
	return &types.CABundleConfigMapRef{Name: t.Origin.CopyName(), Key: providerca.ProviderCACopyKey}
}

// requestedRefs returns all distinct CA references of the Zone, sorted.
func requestedRefs(zone *adminv1.Zone) []types.CABundleConfigMapRef {
	seen := map[string]types.CABundleConfigMapRef{}
	add := func(t *types.ProviderTLS) {
		if r := t.Ref(); r != nil {
			seen[sourceKey(r)] = *r
		}
	}
	for i := range zone.Spec.IdentityProviders {
		add(zone.Spec.IdentityProviders[i].TLS)
	}
	for i := range zone.Spec.Gateways {
		add(zone.Spec.Gateways[i].TLS)
	}
	refs := make([]types.CABundleConfigMapRef, 0, len(seen))
	for _, r := range seen {
		refs = append(refs, r)
	}
	slices.SortFunc(refs, func(a, b types.CABundleConfigMapRef) int {
		return strings.Compare(a.Name+"/"+a.Key, b.Name+"/"+b.Key)
	})
	return refs
}

// errZoneNotLive stops the pipeline without further writes once deletion or a recreated Zone
// (different UID) has been observed through a direct read.
var errZoneNotLive = errors.New("zone is being deleted, gone or was recreated")

// requireLive returns errZoneNotLive unless the Zone is live with the same UID.
func (h *ZoneHandler) requireLive(ctx context.Context, zone *adminv1.Zone) error {
	live, err := h.zoneLive(ctx, zone)
	if err != nil {
		return err
	}
	if !live {
		return errZoneNotLive
	}
	return nil
}

// stagedGuard rejects publication when the object fetched for mutation is not exactly the staged
// object (same UID and resourceVersion; absent if staged absent). Combined with the update's
// optimistic lock on that resourceVersion, a replacement or change after staging is never overwritten.
func stagedGuard(fetched, staged client.Object) error {
	if staged == nil {
		if fetched.GetResourceVersion() != "" {
			return ctrlerrors.RetryableErrorf("%s appeared after staging; restaging", client.ObjectKeyFromObject(fetched))
		}
		return nil
	}
	if fetched.GetUID() != staged.GetUID() || fetched.GetResourceVersion() != staged.GetResourceVersion() {
		return ctrlerrors.RetryableErrorf("%s changed after staging; restaging", client.ObjectKeyFromObject(fetched))
	}
	return nil
}

// zoneLive checks the cached Zone identity and deletion state.
func (h *ZoneHandler) zoneLive(ctx context.Context, zone *adminv1.Zone) (bool, error) {
	current := &adminv1.Zone{}
	if err := h.Reader.Get(ctx, client.ObjectKeyFromObject(zone), current); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, ctrlerrors.RetryableErrorf("reading zone %s: %s", zone.Name, err)
	}
	return current.UID == zone.UID && current.DeletionTimestamp.IsZero(), nil
}

// prepareTrust reads and validates all sources and merges them into the
// Zone CA bundle without writing it. Every Gateway and IdentityProvider of the Zone trusts that
// bundle; hc.Trust stays nil (system trust) when the Zone requests no source.
func (h *ZoneHandler) prepareTrust(ctx context.Context, hc *HandlingContext) error {
	hc.Trust = nil
	refs := requestedRefs(hc.Zone)
	if len(refs) == 0 {
		// Zones that never requested custom trust do not carry the condition. A previously written
		// bundle is retained for consumers that still reference it.
		if meta.FindStatusCondition(hc.Zone.Status.Conditions, ConditionTypeTrustSourcesReady) != nil {
			setTrustCondition(hc, metav1.ConditionTrue, ReasonTrustSynchronized, "No provider CA sources requested")
		}
		return nil
	}
	bundles := make([]*serverclient.ProviderCABundle, len(refs))
	for i := range refs {
		bundle, err := h.readSource(ctx, hc.Zone.Namespace, &refs[i])
		if err != nil {
			setTrustCondition(hc, metav1.ConditionFalse, ReasonTrustSourceInvalid, sourceKey(&refs[i])+": "+err.Error())
			return err
		}
		bundles[i] = bundle
	}
	merged, err := serverclient.MergeProviderCABundles(bundles...)
	if err != nil {
		err = ctrlerrors.BlockedErrorf("combined provider CA bundle of the zone is invalid: %s", err)
		setTrustCondition(hc, metav1.ConditionFalse, ReasonTrustSourceInvalid, err.Error())
		return err
	}
	hc.Trust = &trustSource{Origin: zoneOrigin(hc.Zone), Bundle: merged, Sources: len(refs)}
	return nil
}

// writeTrust persists the prepared Zone CA bundle with a single create or update and re-validates
// the durable copy. It runs only after all provider configurations were validated.
func (h *ZoneHandler) writeTrust(ctx context.Context, hc *HandlingContext) error {
	ts := hc.Trust
	if ts == nil {
		return nil
	}
	merged := ts.Bundle
	cm, copyErr := h.ensureCopy(ctx, hc, ts, merged, ts.Sources)
	if copyErr != nil {
		if !errors.Is(copyErr, errZoneNotLive) {
			setTrustCondition(hc, metav1.ConditionFalse, ReasonTrustCopyInvalid, copyErr.Error())
		}
		return copyErr
	}
	// Validate the write response without waiting for the informer cache to observe it.
	copyBundle, err := ValidateProviderCACopy(cm, &ts.Origin)
	if err == nil && copyBundle.Digest() != merged.Digest() {
		err = ctrlerrors.RetryableErrorf("provider CA copy %s does not hold the combined source content yet", ts.Origin.CopyName())
	}
	if err != nil {
		setTrustCondition(hc, metav1.ConditionFalse, ReasonTrustCopyInvalid, err.Error())
		return err
	}
	ts.Bundle = copyBundle
	setTrustCondition(hc, metav1.ConditionTrue, ReasonTrustSynchronized, "All requested provider CA sources are synchronized")
	return nil
}

func setTrustCondition(hc *HandlingContext, status metav1.ConditionStatus, reason, message string) {
	hc.Zone.SetCondition(metav1.Condition{
		Type: ConditionTypeTrustSourcesReady, Status: status, Reason: reason, Message: message,
		ObservedGeneration: hc.Zone.Generation,
	})
}

func classifyReadErr(what string, err error) error {
	switch {
	case apierrors.IsNotFound(err):
		return ctrlerrors.BlockedErrorf("%s not found", what)
	case apierrors.IsForbidden(err):
		return ctrlerrors.BlockedErrorf("%s is forbidden", what)
	default:
		return ctrlerrors.RetryableErrorf("reading %s: %s", what, err)
	}
}

// readSource validates a cached source. Unlabeled sources are hidden by the cache filter.
func (h *ZoneHandler) readSource(ctx context.Context, namespace string, ref *types.CABundleConfigMapRef) (*serverclient.ProviderCABundle, error) {
	what := fmt.Sprintf("provider CA source %s/%s", namespace, ref.Name)
	resolved, err := providerca.Resolve(ctx, h.Reader, namespace, ref, serverclient.ParseProviderCABundle)
	if err == nil {
		return resolved.Bundle, nil
	}
	re := providerca.AsResolveError(err)
	if re == nil {
		return nil, ctrlerrors.RetryableErrorf("resolving %s: %s", what, err)
	}
	switch re.Kind {
	case providerca.FailureInvalidReference:
		return nil, ctrlerrors.BlockedErrorf("%s: %s", what, re.Err)
	case providerca.FailureNotFound:
		return nil, ctrlerrors.BlockedErrorf("%s not found or lacks label %s=%s", what, types.ProviderCALabelKey, types.ProviderCALabelValue)
	case providerca.FailureForbidden, providerca.FailureReadFailed:
		return nil, classifyReadErr(what, re.Err)
	case providerca.FailureInvalidBundle:
		return nil, ctrlerrors.BlockedErrorf("%s key %q is invalid: %s", what, ref.Key, re.Err)
	default:
		return nil, ctrlerrors.BlockedErrorf("%s %s", what, re.Err)
	}
}

func copyLabels(hc *HandlingContext) map[string]string {
	return map[string]string{
		cconfig.EnvironmentLabelKey:          hc.Environment.Name,
		cconfig.BuildLabelKey(zoneLabelName): hc.Zone.Name,
		cconfig.DomainLabelKey:               domainName,
		types.ProviderCALabelKey:             types.ProviderCALabelValue,
	}
}

// verifyCopyOwnership checks that an existing object at the copy name belongs to origin.
// It never adopts objects lacking the complete origin.
func verifyCopyOwnership(cm *corev1.ConfigMap, origin *providerca.ProviderCAOrigin, env string) error {
	what := fmt.Sprintf("provider CA copy %s/%s", cm.Namespace, cm.Name)
	got, ok := providerca.ProviderCAOriginFromObject(cm)
	if !ok || got != *origin {
		return ctrlerrors.BlockedErrorf("%s does not carry the expected origin; refusing to adopt or overwrite", what)
	}
	if len(cm.OwnerReferences) > 0 {
		return ctrlerrors.BlockedErrorf("%s has unexpected ownerReferences", what)
	}
	if cm.Labels[cconfig.EnvironmentLabelKey] != env || cm.Labels[cconfig.BuildLabelKey(zoneLabelName)] != origin.ZoneName {
		return ctrlerrors.BlockedErrorf("%s has mismatching environment/zone labels", what)
	}
	return nil
}

// ensureCopy writes the validated combined bundle to the durable Zone copy with a single create
// or update and returns the resulting copy. It only writes when the PEM or essential metadata
// differ, and only while the Zone is live. Conflicts from stale cached reads are retried.
func (h *ZoneHandler) ensureCopy(ctx context.Context, hc *HandlingContext, ts *trustSource, bundle *serverclient.ProviderCABundle, sources int) (*corev1.ConfigMap, error) {
	log := logr.FromContextOrDiscard(ctx)
	key := client.ObjectKey{Namespace: hc.Namespace.Name, Name: ts.Origin.CopyName()}
	pem := string(bundle.CanonicalPEM())

	existing := &corev1.ConfigMap{}
	err := h.Reader.Get(ctx, key, existing)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, ctrlerrors.RetryableErrorf("reading provider CA copy %s: %s", key, err)
	}
	if apierrors.IsNotFound(err) {
		if liveErr := h.requireLive(ctx, hc.Zone); liveErr != nil {
			return nil, liveErr
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: key.Name, Namespace: key.Namespace, Labels: copyLabels(hc), Annotations: ts.Origin.Annotations(),
			},
			Data: map[string]string{providerca.ProviderCACopyKey: pem},
		}
		if err := h.Writer.Create(ctx, cm); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return nil, ctrlerrors.RetryableErrorf("creating provider CA copy %s: it exists but is not visible in the cache; "+
					"if this persists, restore its label %s=%s", key, types.ProviderCALabelKey, types.ProviderCALabelValue)
			}
			return nil, ctrlerrors.RetryableErrorf("creating provider CA copy %s: %s", key, err)
		}
		log.V(0).Info("Created provider CA copy", "copy", key.Name, "sources", sources, "certificates", bundle.Len())
		return cm, nil
	}

	if err := verifyCopyOwnership(existing, &ts.Origin, hc.Environment.Name); err != nil {
		return nil, err
	}
	desired := existing.DeepCopy()
	if desired.Labels == nil {
		desired.Labels = map[string]string{}
	}
	for k, v := range copyLabels(hc) {
		desired.Labels[k] = v
	}
	desired.Data = map[string]string{providerca.ProviderCACopyKey: pem}
	if equalStringMaps(existing.Labels, desired.Labels) && equalStringMaps(existing.Data, desired.Data) && len(existing.BinaryData) == 0 {
		return existing, nil
	}
	desired.BinaryData = nil
	if existing.Immutable != nil && *existing.Immutable {
		return nil, ctrlerrors.BlockedErrorf("provider CA copy %s is immutable and cannot be updated", key)
	}
	if liveErr := h.requireLive(ctx, hc.Zone); liveErr != nil {
		return nil, liveErr
	}
	if err := h.Writer.Update(ctx, desired); err != nil {
		return nil, ctrlerrors.RetryableErrorf("updating provider CA copy %s: %s", key, err)
	}
	log.V(0).Info("Updated provider CA copy", "copy", key.Name, "sources", sources, "certificates", bundle.Len())
	return desired, nil
}

func equalStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// ValidateProviderCACopy validates a durable copy against the expected origin.
func ValidateProviderCACopy(cm *corev1.ConfigMap, origin *providerca.ProviderCAOrigin) (*serverclient.ProviderCABundle, error) {
	what := fmt.Sprintf("provider CA copy %s/%s", cm.Namespace, cm.Name)
	got, err := providerca.ValidateProviderCACopyMetadata(cm)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("%s %s", what, err)
	}
	if got != *origin {
		return nil, ctrlerrors.BlockedErrorf("%s has invalid origin metadata", what)
	}
	bundle, err := serverclient.ParseProviderCABundle([]byte(cm.Data[providerca.ProviderCACopyKey]))
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("%s is invalid: %s", what, err)
	}
	return bundle, nil
}

// normalizeURL returns a canonical form of an absolute URL, e.g. for the published discovery issuer.
func normalizeURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if !u.IsAbs() || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("URL %q must be absolute with a host and without userinfo or fragment", raw)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = urlutil.CanonicalHost(u)
	if u.Path == "" && u.RawPath == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func requireHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !urlutil.IsHTTPS(u) {
		return fmt.Errorf("URL %q must use https when a CA bundle is configured", raw)
	}
	return nil
}

// preparedIdentityProvider is the validated IdentityProvider configuration staged between
// prepareIdentityProvider and createIdentityProvider.
type preparedIdentityProvider struct {
	Config          *adminv1.IdentityProviderConfig
	AdminURL        string
	DiscoveryIssuer string
	// Staged is the published provider read before validation; nil if absent.
	Staged *identityapi.IdentityProvider
}

// newPreparedIdentityProvider validates the IdentityProvider configuration. Target moves are not
// migrated: Realms follow the new target and resources at the previous target are left in place.
func newPreparedIdentityProvider(hc *HandlingContext, idp *adminv1.IdentityProviderConfig, adminURL string, staged *identityapi.IdentityProvider) (*preparedIdentityProvider, error) {
	hostname, err := issuerHostname(idp)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("cannot resolve issuer hostname: %s", err)
	}
	prepared := &preparedIdentityProvider{Config: idp, AdminURL: adminURL, DiscoveryIssuer: "https://" + hostname + "/", Staged: staged}
	if n, normErr := normalizeURL(prepared.DiscoveryIssuer); normErr == nil {
		prepared.DiscoveryIssuer = n
	}
	if hc.Trust != nil {
		if httpsErr := requireHTTPS(adminURL); httpsErr != nil {
			return nil, ctrlerrors.BlockedErrorf("identity provider %s: %s", idp.Name, httpsErr)
		}
	}
	return prepared, nil
}

// validateGatewayEndpoints requires https Gateway admin URLs when the Zone has custom trust. The
// Gateway issuer URL is always derived as https.
func validateGatewayEndpoints(hc *HandlingContext) error {
	if hc.Trust == nil {
		return nil
	}
	for i := range hc.Zone.Spec.Gateways {
		config := &hc.Zone.Spec.Gateways[i]
		if err := requireHTTPS(config.Admin.Url); err != nil {
			return ctrlerrors.BlockedErrorf("gateway %q: %s", config.Name, err)
		}
	}
	return nil
}

func newDiscoveryClient(bundle *serverclient.ProviderCABundle) (*http.Client, error) {
	c := &http.Client{Timeout: discoveryTimeout, CheckRedirect: serverclient.DenyRedirects}
	if bundle == nil {
		return c, nil
	}
	transport, err := serverclient.NewProviderTransport(nil, bundle)
	if err != nil {
		return nil, fmt.Errorf("building discovery transport: %w", err)
	}
	c.Transport = transport
	return c, nil
}

// checkChildProvenance rejects modifying a generated child of another Zone incarnation.
func checkChildProvenance(hc *HandlingContext, obj metav1.Object) error {
	if uid, ok := obj.GetAnnotations()[providerca.AnnotationZoneUID]; ok {
		if uid != string(hc.Zone.UID) {
			return ctrlerrors.BlockedErrorf("%s/%s belongs to another Zone incarnation (uid %s)", obj.GetNamespace(), obj.GetName(), uid)
		}
		return nil
	}
	labels := obj.GetLabels()
	if labels[cconfig.EnvironmentLabelKey] != hc.Environment.Name || labels[cconfig.BuildLabelKey(zoneLabelName)] != hc.Zone.Name {
		return ctrlerrors.BlockedErrorf("%s/%s is not a generated child of this Zone", obj.GetNamespace(), obj.GetName())
	}
	return nil
}

func setZoneUID(hc *HandlingContext, obj metav1.Object) {
	a := obj.GetAnnotations()
	if a == nil {
		a = map[string]string{}
	}
	a[providerca.AnnotationZoneUID] = string(hc.Zone.UID)
	obj.SetAnnotations(a)
}
