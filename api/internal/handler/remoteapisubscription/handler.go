// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package remoteapisubscription

import (
	"context"

	"github.com/pkg/errors"
	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/remoteapisubscription/syncer"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ handler.Handler[*apiapi.RemoteApiSubscription] = (*RemoteApiSubscriptionHandler)(nil)

type RemoteApiSubscriptionHandler struct {
	SyncerFactory syncer.SyncerClientFactory[*apiapi.RemoteApiSubscription]
}

func (h *RemoteApiSubscriptionHandler) CreateOrUpdate(ctx context.Context, obj *apiapi.RemoteApiSubscription) (err error) {
	isRemote, _, err := IshandledRemotely(ctx, obj)
	if err != nil {
		return errors.Wrapf(err, "failed to check if RemoteApiSubscription is handled remotely")
	}
	if isRemote {
		return h.handleConsumerScenario(ctx, obj)
	}
	return h.handleProviderScenario(ctx, obj)
}

func (h *RemoteApiSubscriptionHandler) Delete(ctx context.Context, obj *apiapi.RemoteApiSubscription) error {
	c := client.ClientFromContextOrDie(ctx)

	isRemote, remoteOrg, err := IshandledRemotely(ctx, obj)
	if err != nil {
		return errors.Wrapf(err, "failed to check if RemoteApiSubscription is handled remotely")
	}

	if isRemote {
		if obj.Status.Route != nil {
			route := &gatewayapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obj.Status.Route.Name,
					Namespace: obj.Status.Route.Namespace,
				},
			}
			err = c.Delete(ctx, route)
			if err != nil {
				if !apierrors.IsNotFound(err) {
					return errors.Wrapf(err, "failed to delete route %s", obj.Status.Route.Name)
				}
			}
		}

		err := h.SyncerFactory.NewClient(remoteOrg).Delete(ctx, obj)
		if err != nil {
			return errors.Wrapf(err, "failed to delete RemoteApiSubscription on remote CP")
		}
	}

	// Handled via controller-reference

	return nil
}

// IshandledRemotely checks if this RemoteApiSubscription is not handled by this CP
// but rather by another CP. It does so by checking if a RemoteOrganization exists
func IshandledRemotely(ctx context.Context, obj *apiapi.RemoteApiSubscription) (bool, *adminapi.RemoteOrganization, error) {
	log := log.FromContext(ctx)
	c := client.ClientFromContextOrDie(ctx)
	remoteOrg := &adminapi.RemoteOrganization{}
	remoteOrgRef := types.ObjectRef{Name: obj.Spec.TargetOrganization, Namespace: contextutil.EnvFromContextOrDie(ctx)}
	err := c.Get(ctx, remoteOrgRef.K8s(), remoteOrg)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("RemoteOrganization not found, assuming RemoteApiSubscription is handled locally")
			return false, nil, nil
		}
		return false, nil, errors.Wrapf(err, "failed to get remote organization %s", remoteOrgRef.Name)
	}
	log.Info("RemoteOrganization found, assuming RemoteApiSubscription is handled remotely")
	return true, remoteOrg, nil
}
