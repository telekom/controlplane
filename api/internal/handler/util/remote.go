// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	adminapi "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func GetRemoteOrganization(ctx context.Context, ref types.ObjectRef) (*adminapi.RemoteOrganization, error) {
	c := client.ClientFromContextOrDie(ctx)
	remoteOrg := &adminapi.RemoteOrganization{}
	err := c.Get(ctx, ref.K8s(), remoteOrg)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to find remoteorganization %q", ref.String()))
		}
		return nil, ctrlerrors.BlockedErrorf("remoteorganization %q not found", ref.String())
	}

	if err := condition.EnsureReady(remoteOrg); err != nil {
		return nil, ctrlerrors.BlockedErrorf("remoteorganization %q is not ready", ref.String())
	}

	return remoteOrg, nil
}
