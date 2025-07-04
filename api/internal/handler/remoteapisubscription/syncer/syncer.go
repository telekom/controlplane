// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//go:build !mock

package syncer

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/api/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiv1 "github.com/telekom/controlplane/api/api/v1"

	cpv1 "github.com/telekom/controlplane/cpapi/api/v1"
)

type Resource any

type SyncerClient[R Resource] interface {
	Send(ctx context.Context, resource R) (bool, R, error)
	SendStatus(ctx context.Context, resource R) (bool, R, error)
	Delete(ctx context.Context, resource R) error
}

type SyncerClientConfig interface {
	GetUrl() string
	GetIssuerUrl() string
	GetClientId() string
	GetClientSecret() string
}

type SyncerClientFactory[R Resource] interface {
	NewClient(cfg SyncerClientConfig) SyncerClient[R]
}

type syncerFactory[R Resource] struct {
	New func(cfg SyncerClientConfig) SyncerClient[R]
}

func (f *syncerFactory[R]) NewClient(cfg SyncerClientConfig) SyncerClient[R] {
	return f.New(cfg)
}

func NewSyncerFactory() SyncerClientFactory[*apiv1.RemoteApiSubscription] {
	return &syncerFactory[*apiv1.RemoteApiSubscription]{
		New: func(cfg SyncerClientConfig) SyncerClient[*apiv1.RemoteApiSubscription] {
			apiClient, err := cpv1.GetApiClient(context.TODO(), cfg)
			if err != nil {
				panic(err) // TODO: error handling
			}

			return &syncerClient{
				apiClient,
			}
		},
	}
}

type syncerClient struct {
	ApiClient cpv1.ClientWithResponsesInterface
}

func (c *syncerClient) Send(ctx context.Context, resource *apiv1.RemoteApiSubscription) (bool, *apiv1.RemoteApiSubscription, error) {
	log := log.FromContext(ctx)
	log.Info("Sending RemoteApiSubscription to remote CP")

	body := cpv1.RemoteSubscriptionSpec{
		ApiBasePath: resource.Spec.ApiBasePath,
		Requester: cpv1.Requester{
			Application: resource.Spec.Requester.Application,
			Team: cpv1.Team{
				Name:  resource.Spec.Requester.Team.Name,
				Email: resource.Spec.Requester.Team.Email,
			},
		},
	}
	if util.HasM2MRemote(resource) {
		body.Security = &cpv1.Security{
			Oauth2: &cpv1.SecurityOauth2{
				Scopes: &resource.Spec.Security.M2M.Scopes,
			},
		}
	}

	resourceId := MakeResourceId(resource)
	res, err := c.ApiClient.CreateOrUpdateRemoteSubscriptionWithResponse(ctx, resourceId, body)
	if err != nil {
		return false, resource, errors.Wrap(err, "failed to sent request to update resource")
	}

	if err := CheckStatusCode(res, 200, 201); err != nil {
		return false, resource, errors.Wrap(err.withBody(res.Body), "failed to update resource")
	}

	return res.JSON200.Updated, resource, nil
}

func (c *syncerClient) SendStatus(ctx context.Context, resource *apiv1.RemoteApiSubscription) (bool, *apiv1.RemoteApiSubscription, error) {
	log := log.FromContext(ctx)
	log.Info("Sending RemoteApiSubscriptionStatus to remote CP")

	body := cpv1.RemoteSubscriptionStatus{
		Conditions: MapConditions(resource.Status.Conditions),
		GatewayUrl: resource.Status.GatewayUrl,
	}

	if resource.Status.Approval != nil {
		body.Approval = &cpv1.ApprovalInfo{
			Message: resource.Status.Approval.Message,
			State:   cpv1.ApprovalInfoState(resource.Status.Approval.ApprovalState),
		}
	}
	if resource.Status.ApprovalRequest != nil {
		body.ApprovalRequest = &cpv1.ApprovalInfo{
			Message: resource.Status.ApprovalRequest.Message,
			State:   cpv1.ApprovalInfoState(resource.Status.ApprovalRequest.ApprovalState),
		}
	}

	resourceId := MakeResourceId(resource)

	res, err := c.ApiClient.UpdateRemoteSubscriptionStatusWithResponse(ctx, resourceId, body)
	if err != nil {
		return false, resource, errors.Wrap(err, "failed to sent request to update resource status")
	}

	if err := CheckStatusCode(res, 200, 201); err != nil {
		return false, resource, errors.Wrap(err.withBody(res.Body), "failed to update resource status")
	}

	return res.JSON200.Updated, resource, nil
}

func (c *syncerClient) Delete(ctx context.Context, resource *apiv1.RemoteApiSubscription) error {
	log := log.FromContext(ctx)
	log.Info("Deleting RemoteApiSubscription from remote CP")

	resourceId := MakeResourceId(resource)

	res, err := c.ApiClient.DeleteRemoteSubscriptionWithResponse(ctx, resourceId)
	if err != nil {
		return errors.Wrap(err, "failed to sent request to delete resource")
	}

	if err := CheckStatusCode(res, 200, 204, 404); err != nil {
		return errors.Wrap(err.withBody(res.Body), "failed to delete resource")
	}

	return nil
}

func MakeResourceId(ras *apiv1.RemoteApiSubscription) string {
	teamName := ras.Spec.Requester.Team.Name
	appName := ras.Spec.Requester.Application
	basePath := labelutil.NormalizeValue(ras.Spec.ApiBasePath)
	return fmt.Sprintf("%s--%s--%s", teamName, appName, basePath)
}

func MapConditions(conditions []metav1.Condition) []cpv1.StatusCondition {
	result := make([]cpv1.StatusCondition, 0, len(conditions))
	for _, c := range conditions {
		result = append(result, cpv1.StatusCondition{
			Type:               c.Type,
			Status:             cpv1.StatusConditionStatus(c.Status),
			LastTransitionTime: c.LastTransitionTime.Time,
			Reason:             c.Reason,
			Message:            c.Message,
		})
	}
	return result
}
