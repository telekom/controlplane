// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package remoteapisubscription

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

func CalculateRemoteOrgZone(remoteOrg *adminapi.RemoteOrganization) types.ObjectRef {
	return types.ObjectRef{
		Name:      fmt.Sprintf("%s-%s", remoteOrg.Spec.Id, remoteOrg.Spec.Zone.Name),
		Namespace: remoteOrg.Spec.Zone.Namespace,
	}
}

func fillApprovalInfo(ctx context.Context, obj *apiapi.RemoteApiSubscription, apiSubscription *apiapi.ApiSubscription) (err error) {
	if apiSubscription.Status.Approval == nil {
		return nil
	}

	c := client.ClientFromContextOrDie(ctx)

	approval := &approvalapi.Approval{}
	err = c.Get(ctx, apiSubscription.Status.Approval.K8s(), approval)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get approval %s", apiSubscription.Status.Approval.Name)
		}
		return nil
	}
	obj.Status.Approval = &apiapi.ApprovalInfo{
		ApprovalState: approval.Spec.State.String(),
		Message:       "", // todo - resolve later, should be taken from decisions
	}
	return err
}

func fillApprovalRequestInfo(ctx context.Context, obj *apiapi.RemoteApiSubscription, apiSubscription *apiapi.ApiSubscription) (err error) {
	if apiSubscription.Status.ApprovalRequest == nil {
		return nil
	}

	c := client.ClientFromContextOrDie(ctx)

	approvalRequest := &approvalapi.ApprovalRequest{}
	err = c.Get(ctx, apiSubscription.Status.ApprovalRequest.K8s(), approvalRequest)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get approval request %s", apiSubscription.Status.ApprovalRequest.Name)
		}
		return nil
	}
	obj.Status.ApprovalRequest = &apiapi.ApprovalInfo{
		ApprovalState: approvalRequest.Spec.State.String(),
		Message:       "", // todo - resolve later, should be taken from decisions
	}
	return err
}

func fillRouteInfo(ctx context.Context, obj *apiapi.RemoteApiSubscription, apiSubscription *apiapi.ApiSubscription) (err error) {
	if apiSubscription.Status.Route == nil {
		return nil
	}

	c := client.ClientFromContextOrDie(ctx)
	downstreamRoute := &gatewayapi.Route{}
	err = c.Get(ctx, apiSubscription.Status.Route.K8s(), downstreamRoute)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get route %s", apiSubscription.Status.Route.Name)
		}
		return nil
	}
	// Derive the gateway URL from the route's hostnames and paths
	if len(downstreamRoute.Spec.Hostnames) > 0 && len(downstreamRoute.Spec.Paths) > 0 {
		obj.Status.GatewayUrl = "https://" + downstreamRoute.Spec.Hostnames[0] + downstreamRoute.Spec.Paths[0]
	}
	return nil
}
