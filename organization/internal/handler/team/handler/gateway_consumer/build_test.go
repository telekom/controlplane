// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_consumer

import (
	"context"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GatewayConsumerHandler", func() {
	It("builds the team gateway consumer", func() {
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

		Expect(buildGatewayConsumerObj(team)).To(Equal(expected))
	})

	It("reports a missing preset status", func() {
		mockClient := fakeclient.NewMockJanitorClient(GinkgoT())
		mockClient.EXPECT().
			List(mock.Anything, mock.AnythingOfType("*v1.ZoneList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				zones := list.(*adminv1.ZoneList)
				zones.Items = []adminv1.Zone{{
					Spec: adminv1.ZoneSpec{
						ManagedRoutes: &adminv1.ManagedRoutesConfig{},
						Presets:       []adminv1.Preset{{Name: "default", Type: adminv1.GatewayTypeAPI, Default: true}},
					},
				}}
			}).
			Return(nil)

		err := (GatewayConsumerHandler{}).CreateOrUpdate(cclient.WithClient(context.Background(), mockClient), &organizationv1.Team{})

		Expect(err).To(MatchError(ContainSubstring(`preset status "default" not found`)))
	})

	It("reports a missing gateway reference", func() {
		mockClient := fakeclient.NewMockJanitorClient(GinkgoT())
		mockClient.EXPECT().
			List(mock.Anything, mock.AnythingOfType("*v1.ZoneList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				zones := list.(*adminv1.ZoneList)
				zones.Items = []adminv1.Zone{{
					Spec: adminv1.ZoneSpec{
						ManagedRoutes: &adminv1.ManagedRoutesConfig{},
						Presets:       []adminv1.Preset{{Name: "default", Type: adminv1.GatewayTypeAPI, Default: true}},
					},
					Status: adminv1.ZoneStatus{
						Presets: []adminv1.PresetStatus{{Name: "default"}},
					},
				}}
			}).
			Return(nil)

		err := (GatewayConsumerHandler{}).CreateOrUpdate(cclient.WithClient(context.Background(), mockClient), &organizationv1.Team{})

		Expect(err).To(MatchError(ContainSubstring("no gateway reference found in zone object")))
	})
})
