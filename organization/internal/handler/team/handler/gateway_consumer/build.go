// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_consumer

import (
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const TeamNameSuffix = "team-user"

func buildGatewayConsumerObj(owner *organisationv1.Team) *gatewayv1.Consumer {
	name := owner.Spec.Group + handler.Separator + owner.Spec.Name + handler.Separator + TeamNameSuffix
	return &gatewayv1.Consumer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.Status.Namespace,
		},
	}
}
