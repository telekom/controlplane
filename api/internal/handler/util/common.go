// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	adminapi "github.com/telekom/controlplane/admin/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetZone retrieves a Zone object based on the provided ObjectRef for a zone.
func GetZone(ctx context.Context, client cclient.ScopedClient, ref client.ObjectKey) (*adminapi.Zone, error) {
	zone := &adminapi.Zone{}
	err := client.Get(ctx, ref, zone)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to find zone '%s'", ref.String()))
	}

	return zone, nil
}

func GetApplication(ctx context.Context, ref types.ObjectRef) (*applicationapi.Application, error) {
	client := cclient.ClientFromContextOrDie(ctx)

	application := &applicationapi.Application{}
	err := client.Get(ctx, ref.K8s(), application)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to find application '%s'", ref.String()))
	}

	return application, nil
}

func GetRealm(ctx context.Context, ref client.ObjectKey) (*gatewayapi.Realm, error) {
	client := cclient.ClientFromContextOrDie(ctx)

	realm := &gatewayapi.Realm{}
	err := client.Get(ctx, ref, realm)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find realm '%s'", ref.String())
	}
	return realm, nil
}
