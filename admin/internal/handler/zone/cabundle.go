// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/cabundle"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
)

// resolveCABundle validates all CA sources of the zone and publishes their union
// as the managed bundle. It runs before any child connection config is written,
// so an invalid source leaves the last published bundle and all children intact.
func resolveCABundle(ctx context.Context, hc *HandlingContext) error {
	bundle, err := cabundle.ResolveRefs(ctx, hc.KubeClient, hc.Zone.Namespace, hc.Zone.Spec.CARefs()...)
	if err != nil {
		return caBundleError(err)
	}
	hc.CABundle = bundle
	if bundle == nil {
		return nil
	}
	return publishCABundle(ctx, hc)
}

func caBundleError(err error) error {
	if errors.Is(err, cabundle.ErrInvalidBundle) ||
		errors.Is(err, cabundle.ErrSourceNotFound) ||
		errors.Is(err, cabundle.ErrSourceForbidden) {
		return ctrlerrors.BlockedErrorf("invalid CA configuration: %s", err)
	}
	return ctrlerrors.RetryableErrorf("failed to resolve CA configuration: %s", err)
}

// managedTLS returns the TLS config pointing to the managed bundle, or nil if
// the zone trusts the system roots only.
func (hc *HandlingContext) managedTLS() *types.TLSConfig {
	if hc.CABundle == nil {
		return nil
	}
	return &types.TLSConfig{ConfigMapRef: managedBundleRef()}
}

func managedBundleRef() types.ConfigMapKeyRef {
	return types.ConfigMapKeyRef{Name: cabundle.ManagedBundleName, Key: cabundle.ManagedBundleKey}
}

func (hc *HandlingContext) managedLabels() map[string]string {
	return map[string]string{
		cconfig.EnvironmentLabelKey:          hc.Environment.Name,
		cconfig.BuildLabelKey(zoneLabelName): hc.Zone.Name,
		cconfig.DomainLabelKey:               domainName,
	}
}

// isManagedBy reports whether obj carries the management labels of the zone.
func (hc *HandlingContext) isManagedBy(obj client.Object) bool {
	labels := obj.GetLabels()
	for k, v := range hc.managedLabels() {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// publishCABundle writes the validated bundle to the managed ConfigMap.
// The cached read may lag behind; resulting AlreadyExists or Conflict errors are retried.
func publishCABundle(ctx context.Context, hc *HandlingContext) error {
	log := logr.FromContextOrDiscard(ctx)
	key := client.ObjectKey{Namespace: hc.Namespace.Name, Name: cabundle.ManagedBundleName}
	data := map[string]string{cabundle.ManagedBundleKey: string(hc.CABundle)}

	cm := &corev1.ConfigMap{}
	err := hc.KubeClient.Get(ctx, key, cm)
	if apierrors.IsNotFound(err) {
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, Labels: hc.managedLabels()},
			Data:       data,
		}
		if createErr := hc.KubeClient.Create(ctx, cm); createErr != nil {
			return ctrlerrors.RetryableErrorf("failed to create CA bundle %s: %s", key, createErr)
		}
		log.V(0).Info("Created CA bundle", "configMap", key.String())
		return nil
	}
	if err != nil {
		return ctrlerrors.RetryableErrorf("failed to get CA bundle %s: %s", key, err)
	}
	if !hc.isManagedBy(cm) {
		return ctrlerrors.BlockedErrorf("ConfigMap %s exists but is not managed by zone %s", key, hc.Zone.Name)
	}
	if len(cm.Data) == 1 && cm.Data[cabundle.ManagedBundleKey] == string(hc.CABundle) && len(cm.BinaryData) == 0 {
		return nil
	}
	cm.Data = data
	cm.BinaryData = nil
	if err := hc.KubeClient.Update(ctx, cm); err != nil {
		return ctrlerrors.RetryableErrorf("failed to update CA bundle %s: %s", key, err)
	}
	log.V(0).Info("Updated CA bundle", "configMap", key.String())
	return nil
}

// retireCABundle runs when the zone has no CA sources. It clears the managed
// bundle reference from all retained managed children and deletes the managed
// bundle once nothing references it anymore.
func retireCABundle(ctx context.Context, hc *HandlingContext) error {
	if hc.CABundle != nil {
		return nil
	}
	if err := clearManagedTLSRefs(ctx, hc); err != nil {
		return err
	}

	key := client.ObjectKey{Namespace: hc.Namespace.Name, Name: cabundle.ManagedBundleName}
	cm := &corev1.ConfigMap{}
	if err := hc.KubeClient.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return ctrlerrors.RetryableErrorf("failed to get CA bundle %s: %s", key, err)
	}
	if !hc.isManagedBy(cm) {
		return nil
	}

	referenced, err := caBundleReferenced(ctx, hc)
	if err != nil {
		return err
	}
	log := logr.FromContextOrDiscard(ctx)
	if referenced {
		log.V(1).Info("Retaining CA bundle while it is referenced", "configMap", key.String())
		return nil
	}

	err = hc.KubeClient.Delete(ctx, cm, client.Preconditions{UID: &cm.UID, ResourceVersion: &cm.ResourceVersion})
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrlerrors.RetryableErrorf("failed to delete CA bundle %s: %s", key, err)
	}
	log.V(0).Info("Deleted unreferenced CA bundle", "configMap", key.String())
	return nil
}

func isManagedBundleRef(tls *types.TLSConfig) bool {
	return tls != nil && tls.ConfigMapRef.Name == cabundle.ManagedBundleName
}

// clearManagedTLSRefs removes the managed bundle reference from every managed
// Gateway and IdentityProvider of the zone, including instances no longer in the spec.
func clearManagedTLSRefs(ctx context.Context, hc *HandlingContext) error {
	opts := []client.ListOption{client.InNamespace(hc.Namespace.Name), client.MatchingLabels(hc.managedLabels())}

	gateways := &gatewayapi.GatewayList{}
	if err := hc.KubeClient.List(ctx, gateways, opts...); err != nil {
		return ctrlerrors.RetryableErrorf("failed to list gateways of zone %s: %s", hc.Zone.Name, err)
	}
	for i := range gateways.Items {
		gateway := &gateways.Items[i]
		if !isManagedBundleRef(gateway.Spec.TLS) {
			continue
		}
		gateway.Spec.TLS = nil
		if err := hc.KubeClient.Update(ctx, gateway); err != nil {
			return ctrlerrors.RetryableErrorf("failed to clear CA bundle of Gateway %s: %s", gateway.Name, err)
		}
	}

	idps := &identityapi.IdentityProviderList{}
	if err := hc.KubeClient.List(ctx, idps, opts...); err != nil {
		return ctrlerrors.RetryableErrorf("failed to list identity providers of zone %s: %s", hc.Zone.Name, err)
	}
	for i := range idps.Items {
		idp := &idps.Items[i]
		if !isManagedBundleRef(idp.Spec.TLS) {
			continue
		}
		idp.Spec.TLS = nil
		if err := hc.KubeClient.Update(ctx, idp); err != nil {
			return ctrlerrors.RetryableErrorf("failed to clear CA bundle of IdentityProvider %s: %s", idp.Name, err)
		}
	}
	return nil
}

// caBundleReferenced reports whether any Gateway or IdentityProvider in the zone
// namespace, or any Realm connection snapshot, still references the managed bundle.
func caBundleReferenced(ctx context.Context, hc *HandlingContext) (bool, error) {
	ns := hc.Namespace.Name

	gateways := &gatewayapi.GatewayList{}
	if err := hc.KubeClient.List(ctx, gateways, client.InNamespace(ns)); err != nil {
		return false, ctrlerrors.RetryableErrorf("failed to list gateways in %s: %s", ns, err)
	}
	for i := range gateways.Items {
		if isManagedBundleRef(gateways.Items[i].Spec.TLS) {
			return true, nil
		}
	}

	idps := &identityapi.IdentityProviderList{}
	if err := hc.KubeClient.List(ctx, idps, client.InNamespace(ns)); err != nil {
		return false, ctrlerrors.RetryableErrorf("failed to list identity providers in %s: %s", ns, err)
	}
	for i := range idps.Items {
		if isManagedBundleRef(idps.Items[i].Spec.TLS) {
			return true, nil
		}
	}

	realms := &identityapi.RealmList{}
	if err := hc.KubeClient.List(ctx, realms); err != nil {
		return false, ctrlerrors.RetryableErrorf("failed to list realms: %s", err)
	}
	for i := range realms.Items {
		conn := realms.Items[i].Status.Connection
		if conn != nil && conn.TLS != nil && conn.TLS.Namespace == ns && conn.TLS.ConfigMapRef.Name == cabundle.ManagedBundleName {
			return true, nil
		}
	}
	return false, nil
}
