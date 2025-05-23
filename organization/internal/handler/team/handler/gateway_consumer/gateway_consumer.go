// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_consumer

import (
	"context"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler"
	"github.com/telekom/controlplane/organization/internal/handler/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GatewayConsumerHandler struct {
}

var _ handler.ObjectHandler = &GatewayConsumerHandler{}

func (g GatewayConsumerHandler) CreateOrUpdate(ctx context.Context, owner *organisationv1.Team) error {
	var err error

	gatewayConsumerObj := buildGatewayConsumerObj(owner)
	k8sClient := cclient.ClientFromContextOrDie(ctx)
	zoneObj, err := util.GetZoneObjWithTeamInfo(ctx)
	if err != nil {
		return err
	}

	mutate := func() error {
		gatewayConsumerObj.Spec.Name = gatewayConsumerObj.GetName()

		gatewayConsumerObj.Spec = gatewayv1.ConsumerSpec{
			Realm: *zoneObj.Status.TeamApiGatewayRealm,
			Name:  gatewayConsumerObj.GetName(),
		}

		gatewayConsumerObj.SetLabels(owner.GetLabels())
		return nil
	}

	if _, err = k8sClient.CreateOrUpdate(ctx, gatewayConsumerObj, mutate); err != nil {
		return err
	}

	owner.Status.GatewayConsumerRef = types.ObjectRefFromObject(gatewayConsumerObj)
	return nil
}

func (g GatewayConsumerHandler) Delete(ctx context.Context, owner *organisationv1.Team) error {
	var err error
	k8sClient := cclient.ClientFromContextOrDie(ctx)
	if owner.Status.GatewayConsumerRef != nil {
		err = k8sClient.Delete(ctx, &gatewayv1.Consumer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      owner.Status.GatewayConsumerRef.GetName(),
				Namespace: owner.Status.GatewayConsumerRef.GetNamespace(),
			},
		})
	}
	return err
}

func (g GatewayConsumerHandler) Identifier() string {
	return "gateway consumer"
}
