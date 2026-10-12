// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	serverclient "github.com/telekom/controlplane/common-server/pkg/client"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

const (
	zoneLabelName = "zone"
	domainName    = "admin"

	// spacegatePathPrefix is the downstream path prefix added to all identity
	// routes (issuer, certs, discovery) when a zone's visibility is World.
	spacegatePathPrefix = "/spacegate"

	claimOriginZone     = "originZone"
	claimOriginStargate = "originStargate"
	claimClientId       = "clientId"

	EnablePassThrough = true
)

var _ handler.Handler[*adminv1.Zone] = &ZoneHandler{}

// ZoneHandler implements the Handler interface for Zone resources.
type ZoneHandler struct {
	// HTTPClient overrides the OIDC discovery client. When nil, a client trusting the effective
	// identity provider CA copy (or system roots) is built per reconciliation.
	HTTPClient *http.Client
	// Reader is the cache-backed reader for Zone liveness, provider CA sources, copies and
	// generated children. Required.
	Reader client.Reader
	// Writer writes durable CA copies outside the scoped client, so copies never take part in
	// generic sub-resource readiness or cleanup. Required.
	Writer client.Client
}

// NewZoneHandler returns a ZoneHandler and panics when the required reader or writer is missing.
func NewZoneHandler(reader client.Reader, writer client.Client) *ZoneHandler {
	if reader == nil || writer == nil {
		panic("zone.NewZoneHandler: reader and writer are required")
	}
	return &ZoneHandler{Reader: reader, Writer: writer}
}

func (h *ZoneHandler) httpClient() *http.Client {
	if h.HTTPClient != nil {
		return h.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (h *ZoneHandler) CreateOrUpdate(ctx context.Context, obj *adminv1.Zone) error {
	c := cclient.ClientFromContextOrDie(ctx)
	cclient.EnableFeature(c, cclient.CollectSubResources)

	err := h.createOrUpdate(ctx, obj)
	if errors.Is(err, errZoneNotLive) {
		// Deletion or recreation was observed directly: stop without further child writes. The
		// deletion (or the new Zone incarnation) is handled by its own reconciliation.
		logr.FromContextOrDiscard(ctx).V(0).Info("Stopped zone reconciliation: zone is being deleted or was recreated")
		return nil
	}
	return err
}

func (h *ZoneHandler) createOrUpdate(ctx context.Context, obj *adminv1.Zone) error {
	// No write is started once deletion or a recreated Zone is observed through a direct read.
	// Copy writes and provider publications repeat this check right before writing.
	if err := h.requireLive(ctx, obj); err != nil {
		return err
	}

	hc, err := newHandlingContext(ctx, h, obj)
	if err != nil {
		return err
	}

	if err := h.prepareProviders(ctx, hc); err != nil {
		return err
	}
	return h.runSteps(ctx, hc)
}

// prepareProviders reads and merges the CA sources, validates the provider configurations
// (endpoints and committed Realm snapshots) against that trust, and only then writes the Zone CA
// bundle. A rejected configuration therefore changes neither the bundle nor any provider.
func (h *ZoneHandler) prepareProviders(ctx context.Context, hc *HandlingContext) error {
	if err := h.prepareTrust(ctx, hc); err != nil {
		return err
	}
	if err := prepareIdentityProvider(ctx, hc); err != nil {
		return err
	}
	if err := validateGatewayEndpoints(hc); err != nil {
		return err
	}
	return h.writeTrust(ctx, hc)
}

func (h *ZoneHandler) runSteps(ctx context.Context, hc *HandlingContext) error {
	c := cclient.ClientFromContextOrDie(ctx)
	obj := hc.Zone

	// writes marks steps that create, update or delete generated children; they re-check Zone
	// liveness at their boundary so an observed deletion or recreation stops the pipeline before
	// the next group of writes. A deletion landing between that check and a write is not
	// prevented (Kubernetes has no cross-object transaction).
	steps := []struct {
		run    Step
		writes bool
	}{
		{createIdentityProvider, false}, // checks liveness itself right before publishing
		{h.configureDiscovery, false},
		{createDefaultIdentityRealm, true},
		{createInternalIdentityRealm, true},
		// Everything below references a realm that must already exist in the
		// identity provider, which rejects clients and tokens for a realm that
		// is not active yet.
		{waitForSubResources, false},
		{populateRealmName, false},
		{reconcileGateways, true},
		{reconcileInternalRoutes, true},
		{validateGatewayRouteInputs, false},
		{createIdentityRoutes, true},
		{createZoneHealthRoutes, true},
		{cleanupStaleRoutes, true},
		{populatePresetStatus, false},
	}

	for _, step := range steps {
		if step.writes {
			if err := h.requireLive(ctx, obj); err != nil {
				return err
			}
		}
		err := step.run(ctx, hc)
		if errors.Is(err, errWaitingForSubResources) {
			if reportErr := reportNotReadySubResources(ctx, obj); reportErr != nil {
				return reportErr
			}
			obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady, err.Error()))
			return nil
		}
		if err != nil {
			return err
		}
	}

	if err := reportNotReadySubResources(ctx, obj); err != nil {
		return err
	}

	if meta.IsStatusConditionTrue(obj.GetConditions(), condition.ConditionTypeReady) {
		obj.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Zone has been provisioned"))
		obj.SetCondition(condition.NewDoneProcessingCondition("Zone has been provisioned"))
		return nil
	}

	if c.AnyChanged() {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonProvisioning, "At least one sub-resource has been created or updated"))
		return nil
	}

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady, "Waiting for sub-resources to be ready"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Zone has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Zone has been provisioned"))

	return nil
}

// configureDiscovery builds the discovery client for the effective identity provider trust.
func (h *ZoneHandler) configureDiscovery(_ context.Context, hc *HandlingContext) error {
	if h.HTTPClient != nil || hc.IDP == nil {
		return nil
	}
	var bundle *serverclient.ProviderCABundle
	if hc.Trust != nil {
		bundle = hc.Trust.Bundle
	}
	dc, err := newDiscoveryClient(bundle)
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot configure OIDC discovery: %s", err)
	}
	hc.HTTPClient = dc
	return nil
}

// errWaitingForSubResources halts the step pipeline without failing the
// reconciliation. The remaining steps run once the sub-resources are ready.
var errWaitingForSubResources = errors.New("waiting for sub-resources")

// waitForSubResources is a barrier step: it stops the pipeline until every
// sub-resource created by the preceding steps reports Ready=True.
//
// It is a no-op once the zone has been provisioned, because zone readiness is
// latched: a sub-resource degrading later must not stall the remaining steps.
func waitForSubResources(ctx context.Context, hc *HandlingContext) error {
	if meta.IsStatusConditionTrue(hc.Zone.GetConditions(), condition.ConditionTypeReady) {
		return nil
	}

	c := cclient.ClientFromContextOrDie(ctx)
	for _, sub := range cclient.SubResources(c) {
		if meta.IsStatusConditionTrue(sub.GetConditions(), condition.ConditionTypeReady) {
			continue
		}
		gvk, err := apiutil.GVKForObject(sub, c.Scheme())
		if err != nil {
			return fmt.Errorf("determining sub-resource kind: %w", err)
		}
		return fmt.Errorf("%w: %s %s/%s is not ready yet", errWaitingForSubResources,
			gvk.Kind, sub.GetNamespace(), sub.GetName())
	}

	return nil
}

// reportNotReadySubResources emits an event for every sub-resource the scoped
// client observed with Ready=False.
func reportNotReadySubResources(ctx context.Context, obj *adminv1.Zone) error {
	c := cclient.ClientFromContextOrDie(ctx)
	recorder := contextutil.RecorderFromContextOrDie(ctx)

	for _, child := range cclient.NotReadyObjects(c) {
		gvk, err := apiutil.GVKForObject(child, c.Scheme())
		if err != nil {
			return fmt.Errorf("determining sub-resource kind: %w", err)
		}
		ready := meta.FindStatusCondition(child.GetConditions(), condition.ConditionTypeReady)
		recorder.Eventf(obj, corev1.EventTypeWarning, condition.ReasonSubResourceNotReady,
			"Sub-resource %s %s/%s is not ready: %s: %s",
			gvk.Kind, child.GetNamespace(), child.GetName(), ready.Reason, ready.Message)
	}
	return nil
}

// Delete intentionally keeps every sub-resource. In particular the rover admin Client and its
// secret-manager entry are retained: deleting the Client would delete the backing identity
// provider client.
func (h *ZoneHandler) Delete(ctx context.Context, obj *adminv1.Zone) error {
	return nil
}
