// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"

	agenticconfig "github.com/telekom/controlplane/agentic/internal/config"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

// TelecontextInfo holds the resolved Telecontext Application details.
type TelecontextInfo struct {
	// ConsumerName is the consumer name derived from the Application (team--appName).
	ConsumerName string
	// Zone is the zone where the Telecontext Application lives.
	Zone types.ObjectRef
}

// ResolveTelecontextApplication looks up the Telecontext Application CR using
// the configured application ID (group--team--appName) and the environment from context.
// Returns the resolved info needed for route creation.
func ResolveTelecontextApplication(ctx context.Context, cfg *agenticconfig.AgenticConfig) (*TelecontextInfo, error) {
	group, team, appName, err := cfg.ParseTelecontextApplicationID()
	if err != nil {
		return nil, err
	}

	env := contextutil.EnvFromContextOrDie(ctx)
	namespace := env + "--" + group + "--" + team

	ref := types.ObjectRef{
		Name:      appName,
		Namespace: namespace,
	}

	c := cclient.ClientFromContextOrDie(ctx)
	application := &applicationapi.Application{}
	if err := c.Get(ctx, ref.K8s(), application); err != nil {
		return nil, errors.Wrapf(err, "failed to get Telecontext Application %q", ref.String())
	}

	if err := condition.EnsureReady(application); err != nil {
		return nil, ctrlerrors.BlockedErrorf("Telecontext Application %q is not ready", ref.String())
	}

	consumerName := team + "--" + appName

	return &TelecontextInfo{
		ConsumerName: consumerName,
		Zone:         application.Spec.Zone,
	}, nil
}
