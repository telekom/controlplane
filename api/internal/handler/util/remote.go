// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"
	adminapi "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
)

func GetRemoteOrganization(ctx context.Context, orgRef types.ObjectRef) (*adminapi.RemoteOrganization, error) {
	c := client.ClientFromContextOrDie(ctx)
	remoteOrg := &adminapi.RemoteOrganization{}
	err := c.Get(ctx, orgRef.K8s(), remoteOrg)
	if err != nil {
		// TODO: error-handling
		return nil, errors.Wrapf(err, "failed to get remote organization %s", orgRef.Name)
	}
	return remoteOrg, nil
}
