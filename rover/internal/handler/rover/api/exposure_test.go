// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	rover "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("mapClaimsToApiClaims (exposure)", func() {
	It("returns nil for nil claims", func() {
		Expect(mapClaimsToApiClaims(nil)).To(BeNil())
	})

	It("returns nil when aud is unset", func() {
		Expect(mapClaimsToApiClaims(&rover.Claims{})).To(BeNil())
	})

	It("copies a literal value through", func() {
		got := mapClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{Value: "my-audience"}})
		Expect(got.Aud.Value).To(Equal("my-audience"))
		Expect(got.Aud.ValueFrom).To(BeEmpty())
	})

	It("forwards ProviderClientId symbolically", func() {
		got := mapClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{ValueFrom: rover.ClaimValueFromProviderClientId}})
		Expect(got.Aud.Value).To(BeEmpty())
		Expect(got.Aud.ValueFrom).To(Equal(apiapi.ClaimValueFromProviderClientId))
	})

	It("forwards BasePath symbolically", func() {
		got := mapClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{ValueFrom: rover.ClaimValueFromBasePath}})
		Expect(got.Aud.Value).To(BeEmpty())
		Expect(got.Aud.ValueFrom).To(Equal(apiapi.ClaimValueFromBasePath))
	})

	It("forwards ConsumerClientId symbolically", func() {
		got := mapClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{ValueFrom: rover.ClaimValueFromConsumerClientId}})
		Expect(got.Aud.Value).To(BeEmpty())
		Expect(got.Aud.ValueFrom).To(Equal(apiapi.ClaimValueFromConsumerClientId))
	})
})

var _ = Describe("mapSubscriberClaimsToApiClaims (subscription)", func() {
	It("returns nil for nil claims", func() {
		Expect(mapSubscriberClaimsToApiClaims(nil)).To(BeNil())
	})

	It("copies a literal value through", func() {
		got := mapSubscriberClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{Value: "consumer-audience"}})
		Expect(got.Aud.Value).To(Equal("consumer-audience"))
	})

	It("keeps ConsumerClientId symbolic", func() {
		got := mapSubscriberClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{ValueFrom: rover.ClaimValueFromConsumerClientId}})
		Expect(got.Aud.ValueFrom).To(Equal(apiapi.ClaimValueFromConsumerClientId))
	})

	It("ignores ProviderClientId (not resolvable on subscriber side)", func() {
		Expect(mapSubscriberClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{ValueFrom: rover.ClaimValueFromProviderClientId}})).To(BeNil())
	})
})

var _ = Describe("HandleExposure owner team resolution", func() {
	It("returns a BlockedError when the owner team is not found", func() {
		ctx := contextutil.WithEnv(context.Background(), "test-env")
		fakeClient := fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)

		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Team")).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "organization", Resource: "teams"}, "team")).
			Once()

		owner := &rover.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "test-env--eni--hyperion"},
		}
		exp := &rover.ApiExposure{BasePath: "/eni/foo/v1"}

		err := HandleExposure(ctx, fakeClient, owner, exp)

		Expect(err).To(HaveOccurred())
		var be ctrlerrors.BlockedError
		Expect(errors.As(err, &be)).To(BeTrue())
		Expect(be.IsBlocked()).To(BeTrue())
	})
})
