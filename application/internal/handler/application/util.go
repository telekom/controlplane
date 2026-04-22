// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	admin "github.com/telekom/controlplane/admin/api/v1"
	application "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	identity "github.com/telekom/controlplane/identity/api/v1"
)

func MakeClientName(obj *application.Application) string {
	return obj.Spec.Team + "--" + obj.Name
}

func GetZone(ctx context.Context, client client.ScopedClient, zoneRef types.ObjectRef) (*admin.Zone, error) {
	zone := &admin.Zone{}
	err := client.Get(ctx, zoneRef.K8s(), zone)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get zone %s", zoneRef.Name)
	}
	return zone, nil

}

func GetIdpClient(ctx context.Context, client client.ScopedClient, obj *application.Application, clientName string, namespace string) (*identity.Client, error) {

	clientRef := &types.ObjectRef{
		Name:      clientName,
		Namespace: namespace,
	}

	idpClient := &identity.Client{}

	err := client.Get(ctx, clientRef.K8s(), idpClient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get zone")
	}
	return idpClient, nil

}

// propagateRotationStatus copies rotation-related expiry timestamps from the
// primary identity client's status to the Application status.
// It iterates over the Application's client refs and uses the first non-failover
// client it can successfully fetch. Returns true if timestamps were propagated.
func propagateRotationStatus(ctx context.Context, app *application.Application, c client.ScopedClient) bool {
	log := logr.FromContextOrDiscard(ctx)

	for _, ref := range app.Status.Clients {
		idpClient := &identity.Client{}
		if err := c.Get(ctx, ref.K8s(), idpClient); err != nil {
			log.V(1).Info("could not fetch identity client for rotation status propagation",
				"client", ref.Name, "error", err)
			continue
		}

		// Skip failover clients — use primary for authoritative timestamps
		if idpClient.Labels != nil && idpClient.Labels[config.BuildLabelKey("failover")] == "true" {
			continue
		}

		app.Status.RotatedExpiresAt = idpClient.Status.RotatedSecretExpiresAt
		app.Status.CurrentExpiresAt = idpClient.Status.SecretExpiresAt
		return true
	}

	log.Info("could not find a primary identity client to propagate rotation timestamps")
	return false
}
