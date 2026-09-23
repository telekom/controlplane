// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

var _ = Describe("Rover update naming", func() {
	DescribeTable("preserves the resource name and persists a valid application label",
		func(length int) {
			name := strings.Repeat("a", length)
			resourceID := "eni--hyperion--" + name
			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				Environment: "poc", Group: "eni", Team: "hyperion",
			})
			roverStore := mocks.NewMockObjectStore[*roverv1.Rover](GinkgoT())
			persisted := &roverv1.Rover{}
			roverStore.EXPECT().CreateOrReplace(ctx, mock.Anything).
				Run(func(_ context.Context, obj *roverv1.Rover) {
					Expect(obj.Name).To(Equal(name))
					Expect(obj.Namespace).To(Equal("poc--eni--hyperion"))
					label := obj.Labels[config.BuildLabelKey("application")]
					Expect(label).NotTo(BeEmpty())
					Expect(validation.IsValidLabelValue(label)).To(BeEmpty())
					if length <= 63 {
						Expect(label).To(Equal(name))
					} else {
						Expect(label).NotTo(Equal(name))
					}
					Expect(obj.Labels).To(HaveKeyWithValue(config.EnvironmentLabelKey, "poc"))
					Expect(obj.Labels).To(HaveKeyWithValue(config.BuildLabelKey("team"), "hyperion"))
					Expect(obj.Labels).To(HaveKeyWithValue(config.BuildLabelKey("group"), "eni"))
					obj.DeepCopyInto(persisted)
				}).Return(nil).Once()
			roverStore.EXPECT().Get(ctx, "poc--eni--hyperion", name).Return(persisted, nil).Once()
			ctrl := NewRoverController(&store.Stores{RoverStore: roverStore, RoverSecretStore: roverStore})

			response, err := ctrl.Update(ctx, resourceID, api.RoverUpdateRequest{Zone: "dataplane1"})

			Expect(err).NotTo(HaveOccurred())
			Expect(response.Name).To(Equal(name))
			Expect(response.Id).To(Equal(resourceID))
		},
		Entry("at the label limit", 63),
		Entry("above the label limit", 64),
		Entry("at the API name limit", 90),
	)
})
