// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// applicationLabelKey is the label key that ApiExposures carry to reference their
// owning Application by name. Identical to api/internal's ApplicationLabelKey —
// we cannot import internal, so we reconstruct it from the shared BuildLabelKey.
var applicationLabelKey = cconfig.BuildLabelKey("application")

// ProviderBinding captures the resolved identity chain from a Route through its
// owning ApiExposure to the Application that exposed the API. Used to populate
// PlacementIntent fields for authorization fingerprinting.
type ProviderBinding struct {
	ApiExposureName      string
	ApiExposureNamespace string
	ApiExposureUID       string
	ApplicationName      string
}

// verifyProviderBinding checks that the resolved gateway Route is owned by an
// ApiExposure whose application label matches the declared provider Application.
//
// Verification chain:
//  1. Route carries cp.ei.telekom.de/owner.uid = ApiExposure UID.
//  2. List ApiExposures in the Route's namespace; find the one whose metadata.uid
//     matches the label value.
//  3. Verify the ApiExposure is active (status.active == true).
//  4. Verify the Route is referenced in the ApiExposure's status.route or
//     status.proxyRoutes.
//  5. Read the ApiExposure's cp.ei.telekom.de/application label.
//  6. Compare with providerApp.Name.
//
// Returns a ProviderBinding on success, or a BlockedError on any verification
// failure.
func (h *ListenerHandler) verifyProviderBinding(
	ctx context.Context,
	route *gatewayv1.Route,
	providerApp *applicationv1.Application,
) (*ProviderBinding, error) {
	logger := log.FromContext(ctx)

	// Step 1: Read the Route's owner.uid label.
	ownerUID := ""
	if route.Labels != nil {
		ownerUID = route.Labels[cconfig.OwnerUidLabelKey]
	}

	if ownerUID == "" {
		return nil, ctrlerrors.BlockedErrorf("Route %q in namespace %q has no owner.uid label — cannot verify provider binding",
			route.Name, route.Namespace)
	}

	// Step 2: List ApiExposures in the provider's namespace (team namespace)
	// and find the one whose metadata.uid matches the owner label.
	// ApiExposures live in the team namespace alongside the provider Application,
	// NOT in the zone namespace where the Route lives.
	c := cclient.ClientFromContextOrDie(ctx)
	exposureList := &apiv1.ApiExposureList{}
	if err := c.List(ctx, exposureList, client.InNamespace(providerApp.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list ApiExposures in namespace %q: %w", providerApp.Namespace, err)
	}

	var exposure *apiv1.ApiExposure
	for i := range exposureList.Items {
		if string(exposureList.Items[i].UID) == ownerUID {
			exposure = &exposureList.Items[i]
			break
		}
	}

	if exposure == nil {
		return nil, ctrlerrors.BlockedErrorf("no ApiExposure with UID %q found in namespace %q for Route %q",
			ownerUID, providerApp.Namespace, route.Name)
	}

	logger.V(1).Info("Resolved ApiExposure for Route",
		"route", route.Name, "apiExposure", exposure.Name, "apiExposureUID", ownerUID)

	// Step 3: Verify the ApiExposure is active.
	if !exposure.Status.Active {
		return nil, ctrlerrors.BlockedErrorf("ApiExposure %q (UID %s) is not active",
			exposure.Name, ownerUID)
	}

	// Step 4: Verify the Route is referenced in the ApiExposure's status.
	if !routeReferencedByExposure(route, exposure) {
		return nil, ctrlerrors.BlockedErrorf("Route %q is not referenced by ApiExposure %q status (route or proxyRoutes)",
			route.Name, exposure.Name)
	}

	// Step 5: Read the application label from the ApiExposure.
	appName := ""
	if exposure.Labels != nil {
		appName = exposure.Labels[applicationLabelKey]
	}

	if appName == "" {
		return nil, ctrlerrors.BlockedErrorf("ApiExposure %q has no application label", exposure.Name)
	}

	// Step 6: Compare with the declared provider — both name AND namespace
	// must match. The ApiExposure lives in the provider's team namespace,
	// so exposure.Namespace must equal providerApp.Namespace.
	if appName != providerApp.Name || exposure.Namespace != providerApp.Namespace {
		return nil, ctrlerrors.BlockedErrorf(
			"provider binding mismatch: ApiExposure %q (ns %q) is owned by application %q, but declared provider is %q (ns %q)",
			exposure.Name, exposure.Namespace, appName, providerApp.Name, providerApp.Namespace)
	}

	return &ProviderBinding{
		ApiExposureName:      exposure.Name,
		ApiExposureNamespace: exposure.Namespace,
		ApiExposureUID:       string(exposure.UID),
		ApplicationName:      appName,
	}, nil
}

// routeReferencedByExposure checks whether the Route is referenced in the
// ApiExposure's status.route or status.proxyRoutes.
func routeReferencedByExposure(route *gatewayv1.Route, exposure *apiv1.ApiExposure) bool {
	if exposure.Status.Route != nil &&
		exposure.Status.Route.Name == route.Name &&
		exposure.Status.Route.Namespace == route.Namespace {
		return true
	}
	for i := range exposure.Status.ProxyRoutes {
		ref := &exposure.Status.ProxyRoutes[i]
		if ref.Name == route.Name && ref.Namespace == route.Namespace {
			return true
		}
	}
	return false
}
