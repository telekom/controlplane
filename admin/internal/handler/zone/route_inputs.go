// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"

	"github.com/telekom/controlplane/admin/internal/routeinputs"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

// Validate before either creator so permanent limits cannot become retryable
// CRD validation failures in the identity-route step.
func validateGatewayRouteInputs(_ context.Context, hc *HandlingContext) error {
	if errs := routeinputs.Validate(&hc.Zone.Spec); len(errs) > 0 {
		return ctrlerrors.BlockedErrorf("invalid gateway route inputs: %s", errs.ToAggregate())
	}
	return nil
}
