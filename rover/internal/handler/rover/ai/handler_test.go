// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ai_test

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/rover/ai"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	testEnvironment = "test-env"
	teamNamespace   = testEnvironment + "--eni--hyperion"
)

func createTestOwner() *roverv1.Rover {
	return &roverv1.Rover{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: teamNamespace,
			UID:       "rover-uid-1234",
		},
		Spec: roverv1.RoverSpec{
			Zone: "zone1",
		},
		Status: roverv1.RoverStatus{
			Application: &types.ObjectRef{
				Name:      "my-app",
				Namespace: teamNamespace,
			},
		},
	}
}

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = roverv1.AddToScheme(s)
	_ = agenticv1.AddToScheme(s)
	_ = organizationv1.AddToScheme(s)
	return s
}

var _ = Describe("HandleExposure", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		testScheme *runtime.Scheme
		owner      *roverv1.Rover
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctx = contextutil.WithEnv(ctx, testEnvironment)
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		testScheme = newTestScheme()
		owner = createTestOwner()
	})

	It("should create an AgenticExposure with correct spec", func() {
		exposure := &roverv1.AgenticExposure{
			BasePath:   "/mcp/weather/v1",
			Visibility: roverv1.VisibilityWorld,
			Variant:    "MCP",
			Upstreams: []roverv1.Upstream{
				{URL: "http://backend:8080", Weight: 100},
			},
			Approval: roverv1.Approval{
				Strategy: "AUTO",
			},
		}

		var capturedExposure *agenticv1.AgenticExposure

		// Mock Get for FindTeamForObject — return not found (team lookup is best-effort)
		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Team")).
			Return(errors.NewNotFound(schema.GroupResource{Group: "organization", Resource: "teams"}, "team")).
			Maybe()

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticExposure"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedExposure = obj.(*agenticv1.AgenticExposure)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleExposure(ctx, fakeClient, owner, exposure)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedExposure).ToNot(BeNil())
		Expect(capturedExposure.Spec.BasePath).To(Equal("/mcp/weather/v1"))
		Expect(capturedExposure.Spec.Visibility).To(Equal(agenticv1.Visibility("World")))
		Expect(capturedExposure.Spec.Variant).To(Equal(agenticv1.AgenticVariant("MCP")))
		Expect(capturedExposure.Spec.Upstreams).To(HaveLen(1))
		Expect(capturedExposure.Spec.Upstreams[0].Url).To(Equal("http://backend:8080"))
		Expect(capturedExposure.Spec.Upstreams[0].Weight).To(Equal(100))
		Expect(capturedExposure.Spec.Zone.Name).To(Equal("zone1"))
		Expect(capturedExposure.Spec.Zone.Namespace).To(Equal(testEnvironment))
		Expect(capturedExposure.Spec.Provider.Name).To(Equal("my-app"))
		Expect(capturedExposure.Spec.Approval.Strategy).To(Equal(agenticv1.ApprovalStrategy("AUTO")))
	})

	It("should set correct labels", func() {
		exposure := &roverv1.AgenticExposure{
			BasePath:   "/mcp/weather/v1",
			Visibility: roverv1.VisibilityWorld,
			Variant:    "MCP",
			Upstreams:  []roverv1.Upstream{{URL: "http://backend:8080", Weight: 100}},
			Approval:   roverv1.Approval{Strategy: "AUTO"},
		}

		var capturedExposure *agenticv1.AgenticExposure

		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Team")).
			Return(errors.NewNotFound(schema.GroupResource{}, "team")).
			Maybe()
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticExposure"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedExposure = obj.(*agenticv1.AgenticExposure)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleExposure(ctx, fakeClient, owner, exposure)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedExposure.Labels).To(HaveKeyWithValue(agenticv1.AgenticBasePathLabelKey, "mcp-weather-v1"))
		Expect(capturedExposure.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone1"))
		Expect(capturedExposure.Labels).To(HaveKeyWithValue(config.BuildLabelKey("application"), "my-app"))
	})

	It("should map security with M2M and ExternalIDP", func() {
		exposure := &roverv1.AgenticExposure{
			BasePath:   "/mcp/secure/v1",
			Visibility: roverv1.VisibilityWorld,
			Variant:    "MCP",
			Upstreams:  []roverv1.Upstream{{URL: "http://backend:8080", Weight: 100}},
			Approval:   roverv1.Approval{Strategy: "AUTO"},
			Security: &roverv1.Security{
				M2M: &roverv1.Machine2MachineAuthentication{
					Scopes: []string{"read", "write"},
					ExternalIDP: &roverv1.ExternalIdentityProvider{
						TokenEndpoint: "https://idp.example.com/token",
						TokenRequest:  "body",
						GrantType:     "client_credentials",
						Client: &roverv1.OAuth2ClientCredentials{
							ClientId:     "my-client",
							ClientSecret: "my-secret",
						},
					},
				},
			},
		}

		var capturedExposure *agenticv1.AgenticExposure

		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Team")).
			Return(errors.NewNotFound(schema.GroupResource{}, "team")).
			Maybe()
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticExposure"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedExposure = obj.(*agenticv1.AgenticExposure)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleExposure(ctx, fakeClient, owner, exposure)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedExposure.Spec.Security).ToNot(BeNil())
		Expect(capturedExposure.Spec.Security.M2M).ToNot(BeNil())
		Expect(capturedExposure.Spec.Security.M2M.Scopes).To(Equal([]string{"read", "write"}))
		Expect(capturedExposure.Spec.Security.M2M.ExternalIDP).ToNot(BeNil())
		Expect(capturedExposure.Spec.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://idp.example.com/token"))
		Expect(capturedExposure.Spec.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("my-client"))
	})

	It("should map traffic with rate limiting and failover", func() {
		exposure := &roverv1.AgenticExposure{
			BasePath:   "/mcp/traffic/v1",
			Visibility: roverv1.VisibilityWorld,
			Variant:    "MCP",
			Upstreams:  []roverv1.Upstream{{URL: "http://backend:8080", Weight: 100}},
			Approval:   roverv1.Approval{Strategy: "AUTO"},
			Traffic: &roverv1.Traffic{
				CircuitBreaker: &roverv1.CircuitBreaker{Enabled: true},
				RateLimit: &roverv1.RateLimit{
					Provider: &roverv1.RateLimitConfig{
						Limits: &roverv1.Limits{
							Second: 10,
							Minute: 100,
							Hour:   1000,
						},
					},
				},
				Failover: &roverv1.ProviderFailover{
					Zones: []string{"zone2", "zone3"},
				},
			},
		}

		var capturedExposure *agenticv1.AgenticExposure

		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Team")).
			Return(errors.NewNotFound(schema.GroupResource{}, "team")).
			Maybe()
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticExposure"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedExposure = obj.(*agenticv1.AgenticExposure)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleExposure(ctx, fakeClient, owner, exposure)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedExposure.Spec.Traffic.CircuitBreaker).ToNot(BeNil())
		Expect(capturedExposure.Spec.Traffic.CircuitBreaker.Enabled).To(BeTrue())
		Expect(capturedExposure.Spec.Traffic.RateLimit).ToNot(BeNil())
		Expect(capturedExposure.Spec.Traffic.RateLimit.Provider.Limits.Second).To(Equal(10))
		Expect(capturedExposure.Spec.Traffic.RateLimit.Provider.Limits.Minute).To(Equal(100))
		Expect(capturedExposure.Spec.Traffic.RateLimit.Provider.Limits.Hour).To(Equal(1000))
		Expect(capturedExposure.Spec.Traffic.Failover).ToNot(BeNil())
		Expect(capturedExposure.Spec.Traffic.Failover.Zones).To(HaveLen(2))
		Expect(capturedExposure.Spec.Traffic.Failover.Zones[0].Name).To(Equal("zone2"))
		Expect(capturedExposure.Spec.Traffic.Failover.Zones[0].Namespace).To(Equal(testEnvironment))
	})

	It("should map transformation", func() {
		exposure := &roverv1.AgenticExposure{
			BasePath:   "/mcp/transform/v1",
			Visibility: roverv1.VisibilityWorld,
			Variant:    "MCP",
			Upstreams:  []roverv1.Upstream{{URL: "http://backend:8080", Weight: 100}},
			Approval:   roverv1.Approval{Strategy: "AUTO"},
			Transformation: &roverv1.Transformation{
				Request: roverv1.RequestResponseTransformation{
					Headers: roverv1.HeaderTransformation{
						Remove: []string{"X-Internal", "X-Debug"},
					},
				},
			},
		}

		var capturedExposure *agenticv1.AgenticExposure

		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Team")).
			Return(errors.NewNotFound(schema.GroupResource{}, "team")).
			Maybe()
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticExposure"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedExposure = obj.(*agenticv1.AgenticExposure)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleExposure(ctx, fakeClient, owner, exposure)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedExposure.Spec.Transformation).ToNot(BeNil())
		Expect(capturedExposure.Spec.Transformation.Request.Headers.Remove).To(Equal([]string{"X-Internal", "X-Debug"}))
	})

	It("should resolve trusted teams and include owner team", func() {
		exposure := &roverv1.AgenticExposure{
			BasePath:   "/mcp/teams/v1",
			Visibility: roverv1.VisibilityWorld,
			Variant:    "MCP",
			Upstreams:  []roverv1.Upstream{{URL: "http://backend:8080", Weight: 100}},
			Approval: roverv1.Approval{
				Strategy: "MANUAL",
				TrustedTeams: []roverv1.TrustedTeam{
					{Group: "partner", Team: "alpha"},
				},
			},
		}

		// Mock Get for trusted team lookup
		fakeClient.EXPECT().
			Get(ctx, mock.MatchedBy(func(key client.ObjectKey) bool {
				return key.Namespace == testEnvironment && key.Name == organizationv1.TeamResourceName("partner", "alpha")
			}), mock.AnythingOfType("*v1.Team")).
			Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
				team := obj.(*organizationv1.Team)
				team.Name = organizationv1.TeamResourceName("partner", "alpha")
				team.Namespace = testEnvironment
			}).
			Return(nil).Once()

		// Mock Get for owner team lookup (FindTeamForObject)
		fakeClient.EXPECT().
			Get(ctx, mock.MatchedBy(func(key client.ObjectKey) bool {
				return key.Namespace == testEnvironment && key.Name == organizationv1.TeamResourceName("eni", "hyperion")
			}), mock.AnythingOfType("*v1.Team")).
			Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
				team := obj.(*organizationv1.Team)
				team.Name = organizationv1.TeamResourceName("eni", "hyperion")
				team.Namespace = testEnvironment
			}).
			Return(nil).Once()

		var capturedExposure *agenticv1.AgenticExposure

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticExposure"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedExposure = obj.(*agenticv1.AgenticExposure)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleExposure(ctx, fakeClient, owner, exposure)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedExposure.Spec.Approval.TrustedTeams).To(ContainElements(
			organizationv1.TeamResourceName("partner", "alpha"),
			organizationv1.TeamResourceName("eni", "hyperion"),
		))
	})

	It("should append to owner status AgenticExposures", func() {
		exposure := &roverv1.AgenticExposure{
			BasePath:   "/mcp/weather/v1",
			Visibility: roverv1.VisibilityWorld,
			Variant:    "MCP",
			Upstreams:  []roverv1.Upstream{{URL: "http://backend:8080", Weight: 100}},
			Approval:   roverv1.Approval{Strategy: "AUTO"},
		}

		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Team")).
			Return(errors.NewNotFound(schema.GroupResource{}, "team")).
			Maybe()
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticExposure"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleExposure(ctx, fakeClient, owner, exposure)

		Expect(err).ToNot(HaveOccurred())
		Expect(owner.Status.AgenticExposures).To(HaveLen(1))
		Expect(owner.Status.AgenticExposures[0].Namespace).To(Equal(teamNamespace))
	})

	It("should return error when CreateOrUpdate fails", func() {
		exposure := &roverv1.AgenticExposure{
			BasePath:   "/mcp/weather/v1",
			Visibility: roverv1.VisibilityWorld,
			Variant:    "MCP",
			Upstreams:  []roverv1.Upstream{{URL: "http://backend:8080", Weight: 100}},
			Approval:   roverv1.Approval{Strategy: "AUTO"},
		}

		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Team")).
			Return(errors.NewNotFound(schema.GroupResource{}, "team")).
			Maybe()
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticExposure"), mock.AnythingOfType("controllerutil.MutateFn")).
			Return(controllerutil.OperationResultNone, fmt.Errorf("api server error")).Once()

		err := ai.HandleExposure(ctx, fakeClient, owner, exposure)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create or update AgenticExposure"))
	})
})

var _ = Describe("HandleSubscription", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		testScheme *runtime.Scheme
		owner      *roverv1.Rover
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctx = contextutil.WithEnv(ctx, testEnvironment)
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		testScheme = newTestScheme()
		owner = createTestOwner()
	})

	It("should create an AgenticSubscription with correct spec", func() {
		subscription := &roverv1.AgenticSubscription{
			BasePath: "/mcp/weather/v1",
		}

		var capturedSub *agenticv1.AgenticSubscription

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticSubscription"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedSub = obj.(*agenticv1.AgenticSubscription)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleSubscription(ctx, fakeClient, owner, subscription)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedSub).ToNot(BeNil())
		Expect(capturedSub.Spec.BasePath).To(Equal("/mcp/weather/v1"))
		Expect(capturedSub.Spec.Zone.Name).To(Equal("zone1"))
		Expect(capturedSub.Spec.Zone.Namespace).To(Equal(testEnvironment))
		Expect(capturedSub.Spec.Requestor.Application.Name).To(Equal("my-app"))
		Expect(capturedSub.Spec.Requestor.Application.Namespace).To(Equal(teamNamespace))
	})

	It("should set correct labels", func() {
		subscription := &roverv1.AgenticSubscription{
			BasePath: "/mcp/weather/v1",
		}

		var capturedSub *agenticv1.AgenticSubscription

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticSubscription"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedSub = obj.(*agenticv1.AgenticSubscription)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleSubscription(ctx, fakeClient, owner, subscription)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedSub.Labels).To(HaveKeyWithValue(agenticv1.AgenticBasePathLabelKey, "mcp-weather-v1"))
		Expect(capturedSub.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone1"))
		Expect(capturedSub.Labels).To(HaveKeyWithValue(config.BuildLabelKey("application"), "my-app"))
	})

	It("should map subscriber security", func() {
		subscription := &roverv1.AgenticSubscription{
			BasePath: "/mcp/secure/v1",
			Security: &roverv1.SubscriberSecurity{
				M2M: &roverv1.SubscriberMachine2MachineAuthentication{
					Scopes: []string{"read"},
					Client: &roverv1.OAuth2ClientCredentials{
						ClientId:     "sub-client",
						ClientSecret: "sub-secret",
						ClientKey:    "sub-key",
					},
				},
			},
		}

		var capturedSub *agenticv1.AgenticSubscription

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticSubscription"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedSub = obj.(*agenticv1.AgenticSubscription)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleSubscription(ctx, fakeClient, owner, subscription)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedSub.Spec.Security).ToNot(BeNil())
		Expect(capturedSub.Spec.Security.M2M).ToNot(BeNil())
		Expect(capturedSub.Spec.Security.M2M.Scopes).To(Equal([]string{"read"}))
		Expect(capturedSub.Spec.Security.M2M.Client).ToNot(BeNil())
		Expect(capturedSub.Spec.Security.M2M.Client.ClientId).To(Equal("sub-client"))
		Expect(capturedSub.Spec.Security.M2M.Client.ClientSecret).To(Equal("sub-secret"))
		Expect(capturedSub.Spec.Security.M2M.Client.ClientKey).To(Equal("sub-key"))
	})

	It("should map subscriber traffic with failover", func() {
		subscription := &roverv1.AgenticSubscription{
			BasePath: "/mcp/failover/v1",
			Traffic: roverv1.SubscriberTraffic{
				Failover: &roverv1.SubscriberFailover{
					Enabled: true,
				},
			},
		}

		var capturedSub *agenticv1.AgenticSubscription

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticSubscription"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedSub = obj.(*agenticv1.AgenticSubscription)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleSubscription(ctx, fakeClient, owner, subscription)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedSub.Spec.Traffic.Failover).ToNot(BeNil())
		Expect(capturedSub.Spec.Traffic.Failover.Enabled).To(BeTrue())
	})

	It("should append to owner status AgenticSubscriptions", func() {
		subscription := &roverv1.AgenticSubscription{
			BasePath: "/mcp/weather/v1",
		}

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticSubscription"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := ai.HandleSubscription(ctx, fakeClient, owner, subscription)

		Expect(err).ToNot(HaveOccurred())
		Expect(owner.Status.AgenticSubscriptions).To(HaveLen(1))
		Expect(owner.Status.AgenticSubscriptions[0].Namespace).To(Equal(teamNamespace))
	})

	It("should return error when CreateOrUpdate fails", func() {
		subscription := &roverv1.AgenticSubscription{
			BasePath: "/mcp/weather/v1",
		}

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgenticSubscription"), mock.AnythingOfType("controllerutil.MutateFn")).
			Return(controllerutil.OperationResultNone, fmt.Errorf("api server error")).Once()

		err := ai.HandleSubscription(ctx, fakeClient, owner, subscription)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create or update AgenticSubscription"))
	})
})

var _ = Describe("MakeName", func() {
	It("should combine owner name and basePath", func() {
		name := ai.MakeName("my-app", "/mcp/weather/v1")
		Expect(name).To(Equal("my-app--mcp-weather-v1"))
	})
})
