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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
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
	HTTPClient *http.Client
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

	hc, err := newHandlingContext(ctx, obj, h.httpClient())
	if err != nil {
		return err
	}

	steps := []Step{
		createIdentityProvider,
		createDefaultIdentityRealm,
		createInternalIdentityRealm,
		// Everything below references a realm that must already exist in the
		// identity provider, which rejects clients and tokens for a realm that
		// is not active yet.
		waitForSubResources,
		reconcileGateways,
		reconcileInternalRoutes,
		createIdentityRoutes,
		cleanupStaleRoutes,
		populateRealmName,
		populatePresetStatus,
	}

	for _, step := range steps {
		err := step(ctx, hc)
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

func (h *ZoneHandler) Delete(ctx context.Context, obj *adminv1.Zone) error {
	return nil
}
