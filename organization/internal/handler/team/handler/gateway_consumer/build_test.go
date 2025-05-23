// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_consumer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildGatewayConsumer(t *testing.T) {
	team := &organizationv1.Team{
		Spec: organizationv1.TeamSpec{
			Name:  "team",
			Group: "group",
		},
		Status: organizationv1.TeamStatus{
			Namespace: "env--group--team",
		},
	}

	expected := &gatewayv1.Consumer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "group--team--team-user",
			Namespace: "env--group--team",
		},
	}
	got := buildGatewayConsumerObj(team)
	assert.Equal(t, expected, got)
}
