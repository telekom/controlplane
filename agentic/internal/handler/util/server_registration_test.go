// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	cconfig "github.com/telekom/controlplane/common/pkg/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FindOldestPeerRegistration", func() {
	It("scopes peer registration lists to the source environment", func() {
		ctx := context.Background()
		fakeClient := fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		source := &agenticv1.McpServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mcp",
				Namespace: "default",
				Labels: map[string]string{
					cconfig.EnvironmentLabelKey: "test",
				},
			},
			Spec: agenticv1.McpServerSpec{BasePath: "/shared/path"},
		}

		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.MatchedBy(func(option client.ListOption) bool {
				matchingLabels, ok := option.(client.MatchingLabels)
				return ok && matchingLabels[cconfig.EnvironmentLabelKey] == "test"
			})).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{}
			}).
			Return(nil)

		peer, err := util.FindOldestPeerRegistration(ctx, source)

		Expect(err).NotTo(HaveOccurred())
		Expect(peer).To(BeNil())
	})
})
