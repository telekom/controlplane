// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ConsumeRoute_test

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	handler "github.com/telekom/controlplane/gateway/internal/handler/consumeroute"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConsumeRouteHandler with consumer username/password and scopes", func() {
	idp := func(grantType gatewayv1.GrantType) *gatewayv1.ExternalIdentityProvider {
		return &gatewayv1.ExternalIdentityProvider{
			TokenEndpoint: "https://idp.example.com/token",
			TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
			GrantType:     grantType,
		}
	}
	primaryIDP := func(grantType gatewayv1.GrantType) func(*gatewayv1.Route) {
		return func(r *gatewayv1.Route) {
			r.Spec.Security.M2M = &gatewayv1.Machine2MachineAuthentication{ExternalIDP: idp(grantType)}
		}
	}
	secondaryIDP := func(grantType gatewayv1.GrantType) func(*gatewayv1.Route) {
		return func(r *gatewayv1.Route) {
			r.Spec.Type = gatewayv1.RouteTypeSecondary
			r.Spec.Traffic.Failover = &gatewayv1.Failover{Security: gatewayv1.Security{
				M2M: &gatewayv1.Machine2MachineAuthentication{ExternalIDP: idp(grantType)},
			}}
		}
	}

	DescribeTable("reports the route's authentication mode",
		func(configure func(*gatewayv1.Route), blocked bool) {
			mockClient := fakeclient.NewMockJanitorClient(GinkgoT())
			ctx := cclient.WithClient(context.Background(), mockClient)
			mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ pkgclient.ObjectKey, obj pkgclient.Object, _ ...pkgclient.GetOption) {
					r := obj.(*gatewayv1.Route)
					r.Name, r.Namespace = "test-route", "test-ns"
					r.Spec.Type = gatewayv1.RouteTypePrimary
					configure(r)
					meta.SetStatusCondition(&r.Status.Conditions, metav1.Condition{
						Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
					})
				}).Return(nil)
			consumeRoute := &gatewayv1.ConsumeRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "password-consumer", Namespace: "test-ns"},
				Spec: gatewayv1.ConsumeRouteSpec{
					Route:        types.ObjectRef{Name: "test-route", Namespace: "test-ns"},
					ConsumerName: "password-consumer",
					Security: &gatewayv1.ConsumeRouteSecurity{M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
						Basic:  &gatewayv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
						Scopes: []string{"consumer:read"},
					}},
				},
			}

			err := (&handler.ConsumeRouteHandler{}).CreateOrUpdate(ctx, consumeRoute)

			ready := meta.FindStatusCondition(consumeRoute.GetConditions(), condition.ConditionTypeReady)
			processing := meta.FindStatusCondition(consumeRoute.GetConditions(), condition.ConditionTypeProcessing)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(processing).NotTo(BeNil())
			if blocked {
				blockedErr, ok := errors.AsType[ctrlerrors.BlockedError](err)
				Expect(ok).To(BeTrue())
				Expect(blockedErr.IsBlocked()).To(BeTrue())
				Expect(err).To(MatchError(`Consumer username/password with scopes requires an external IDP grant type "password"`))
				Expect(ready.Reason).To(Equal(condition.ReasonValidationFailed))
				Expect(ready.Message).To(Equal(err.Error()))
				Expect(processing.Reason).To(Equal("Blocked"))
				Expect(processing.Message).To(Equal(err.Error()))
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(ready.Reason).To(Equal("ConsumeRouteProcessing"))
		},
		Entry("primary route without an external IDP is blocked", func(r *gatewayv1.Route) {
			r.Spec.Security.M2M = &gatewayv1.Machine2MachineAuthentication{Scopes: []string{"provider:read"}}
		}, true),
		Entry("primary route with a client_credentials grant is blocked", primaryIDP(gatewayv1.GrantTypeClientCredentials), true),
		Entry("failover-secondary route with a client_credentials grant is blocked", secondaryIDP(gatewayv1.GrantTypeClientCredentials), true),
		Entry("primary route with a password grant waits for the route", primaryIDP(gatewayv1.GrantTypePassword), false),
		Entry("failover-secondary route with a password grant waits for the route", secondaryIDP(gatewayv1.GrantTypePassword), false),
		Entry("proxy route waits for the route", func(r *gatewayv1.Route) {
			r.Spec.Type = gatewayv1.RouteTypeProxy
		}, false),
	)
})
