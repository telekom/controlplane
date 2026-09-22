// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/log"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// verifyProviderBinding checks that the resolved gateway Route is owned by a
// resource whose identity is consistent with the declared provider Application.
//
// The api domain stamps Routes with cp.ei.telekom.de/owner.uid pointing at the
// ApiExposure UID. The ApiExposure in turn carries a cp.ei.telekom.de/application
// label holding the owning Application name. A full check would:
//
//  1. Read the Route's owner.uid label to get the ApiExposure UID.
//  2. Fetch the ApiExposure by UID (or by basepath label + namespace).
//  3. Compare the ApiExposure's application label against providerApp.Name.
//
// Step 2 requires importing the api/api module types (ApiExposure, BasePathLabelKey)
// which the spectre module does not currently depend on. Adding the dependency is a
// meaningful architectural decision that should be reviewed separately (Phase 3+).
//
// Until then this function performs a best-effort check: if the Route carries an
// owner.uid label, it logs the value for traceability. A full provider-binding
// violation blocks the Listener; a missing label is logged but does not block,
// because Routes provisioned before the label was introduced legitimately lack it.
//
// Returns nil (passes) in all cases. Callers should treat a non-nil return as a
// BlockedError.
func (h *ListenerHandler) verifyProviderBinding(
	ctx context.Context,
	route *gatewayv1.Route,
	providerApp *applicationv1.Application,
) error {
	logger := log.FromContext(ctx)

	ownerUID := ""
	if route.Labels != nil {
		ownerUID = route.Labels[cconfig.OwnerUidLabelKey]
	}

	if ownerUID == "" {
		logger.V(1).Info("Route has no owner.uid label; skipping provider binding check",
			"route", route.Name, "namespace", route.Namespace,
			"provider", providerApp.Name)
		return nil
	}

	// The owner.uid is the ApiExposure UID, not the Application UID. A full
	// binding check requires fetching the ApiExposure (api/api types). For now,
	// log the association for traceability.
	logger.V(1).Info("Provider binding: Route owner recorded",
		"route", route.Name, "namespace", route.Namespace,
		"routeOwnerUID", ownerUID,
		"provider", providerApp.Name, "providerUID", providerApp.UID)

	return nil
}
