// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"
	"sync"

	testifymock "github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	config "github.com/telekom/controlplane/common/pkg/config"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// faultyClient wraps the envtest client and injects faults for the zone's rover Client.
type faultyClient struct {
	client.Client
	roverName string
	// staleGets reports the rover Client as missing for this many Get calls (-1: always),
	// simulating an informer cache that has not observed it yet.
	staleGets int
	// getErr fails rover Client reads with an error other than NotFound.
	getErr error
	// createErr fails rover Client creation.
	createErr error
}

func (c *faultyClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*identityapi.Client); ok && key.Name == c.roverName {
		if c.getErr != nil {
			return c.getErr
		}
		if c.staleGets != 0 {
			if c.staleGets > 0 {
				c.staleGets--
			}
			return apierrors.NewNotFound(identityapi.GroupVersion.WithResource("clients").GroupResource(), key.Name)
		}
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *faultyClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if _, ok := obj.(*identityapi.Client); ok && obj.GetName() == c.roverName && c.createErr != nil {
		return c.createErr
	}
	return c.Client.Create(ctx, obj, opts...)
}

func newFaultyClient(zone *adminv1.Zone) *faultyClient {
	return &faultyClient{Client: k8sClient, roverName: roverClientKey(zone).Name}
}

func setSecretManagerFeature(enabled bool) func() {
	previous := config.FeatureSecretManager.IsEnabled()
	config.SetFeatureEnabled(config.FeatureSecretManager, enabled)
	return func() { config.SetFeatureEnabled(config.FeatureSecretManager, previous) }
}

func withAIGateway(zone *adminv1.Zone) {
	zone.Spec.Gateways = append(zone.Spec.Gateways, adminv1.GatewayConfig{
		Name: "ai", Admin: adminv1.GatewayAdminConfig{IdentityProviderRef: "primary", Url: "https://ai.example.com/admin-api"},
	})
	zone.Spec.Presets = append(zone.Spec.Presets, adminv1.Preset{
		Name: "ai", Type: adminv1.GatewayTypeAI, GatewayRef: "ai", IdentityProviderRef: "primary",
		Urls: []adminv1.UrlConfig{{Hostname: "ai.example.com", BasePath: "/"}},
	})
}

// provision runs the handler until the gateways have been reconciled.
func provision(h *ZoneHandler, zone *adminv1.Zone) {
	GinkgoHelper()
	Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
	markSubResourcesReady(zone)
	Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
	Expect(zone.Status.Gateways).NotTo(BeEmpty())
}

func zoneNamespace(zone *adminv1.Zone) string {
	return zone.Labels[config.EnvironmentLabelKey] + "--" + zone.Name
}

func roverClientKey(zone *adminv1.Zone) client.ObjectKey {
	return client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForGatewayAdminClient(naming.ForIdentityProvider(zone, "primary"))}
}

func getRoverClient(zone *adminv1.Zone) *identityapi.Client {
	GinkgoHelper()
	c := &identityapi.Client{}
	Expect(k8sClient.Get(ctx, roverClientKey(zone), c)).To(Succeed())
	return c
}

func expectNoRoverClient(zone *adminv1.Zone) {
	GinkgoHelper()
	err := k8sClient.Get(ctx, roverClientKey(zone), &identityapi.Client{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected no rover client, got %v", err)
}

func gatewaysOf(zone *adminv1.Zone) []gatewayapi.Gateway {
	GinkgoHelper()
	list := &gatewayapi.GatewayList{}
	Expect(k8sClient.List(ctx, list, client.InNamespace(zoneNamespace(zone)))).To(Succeed())
	return list.Items
}

// expectGatewaysUse asserts that every gateway and status entry of the zone uses the rover client.
func expectGatewaysUse(zone *adminv1.Zone, roverClient *identityapi.Client) {
	GinkgoHelper()
	gateways := gatewaysOf(zone)
	Expect(gateways).NotTo(BeEmpty())
	for i := range gateways {
		Expect(gateways[i].Spec.Admin.ClientId).To(Equal("rover"))
		Expect(gateways[i].Spec.Admin.ClientSecret).To(Equal(roverClient.Spec.ClientSecret))
	}
	for _, gw := range zone.Status.Gateways {
		Expect(gw.AdminClient).NotTo(BeNil())
		Expect(gw.AdminClient.Name).To(Equal(roverClient.Name))
		Expect(gw.AdminClient.Namespace).To(Equal(roverClient.Namespace))
	}
}

func ensureEnvironment(name string) {
	GinkgoHelper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
	env := &adminv1.Environment{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: name, Labels: map[string]string{config.EnvironmentLabelKey: name},
	}}
	if err := k8sClient.Create(ctx, env); err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

var zoneCounter int

// canonicalRoverRef is the checksum-bearing reference of the zone's rover secret value.
func canonicalRoverRef(zone *adminv1.Zone, value string) string {
	return secretsapi.ToRef(fmt.Sprintf("%s:::%s:%s",
		zone.Labels[config.EnvironmentLabelKey], roverSecretManagerPath(zone.Name), backend.MakeChecksum(value)))
}

// expectNewRoverSecret expects one lookup of the zone's rover secret reporting it missing,
// followed by one write of a generated value under the zone's path. The returned pointer
// holds the written value once the write happened.
func expectNewRoverSecret(sm *fake.MockSecretManager, zone *adminv1.Zone) *string {
	written := new(string)
	env := zone.Labels[config.EnvironmentLabelKey]
	path := roverSecretManagerPath(zone.Name)
	sm.EXPECT().Resolve(testifymock.Anything, lookupRoverRef(zone)).
		Return(secretsapi.ResolvedSecret{}, secretsapi.ErrNotFound).Once()
	sm.EXPECT().UpsertEnvironment(testifymock.Anything, env, testifymock.Anything, roverSecretValueOption(zone)).
		RunAndReturn(func(_ context.Context, _ string, opts ...secretsapi.OnboardingOption) (map[string]string, error) {
			options := &secretsapi.OnboardingOptions{}
			for _, opt := range opts {
				opt(options)
			}
			Expect(options.SecretValues).To(HaveLen(1))
			Expect(options.SecretValues[path]).To(BeAssignableToTypeOf(""))
			*written = options.SecretValues[path].(string)
			Expect(*written).NotTo(BeEmpty())
			id, _ := secretsapi.FromRef(canonicalRoverRef(zone, *written))
			return map[string]string{path: id}, nil
		}).Once()
	return written
}

// roverSecretValueOption matches an onboarding option setting a value under the zone's rover path.
func roverSecretValueOption(zone *adminv1.Zone) any {
	path := roverSecretManagerPath(zone.Name)
	return testifymock.MatchedBy(func(opt secretsapi.OnboardingOption) bool {
		options := &secretsapi.OnboardingOptions{}
		opt(options)
		_, ok := options.SecretValues[path]
		return ok
	})
}

// expectStoredRoverSecret expects one lookup of the zone's rover secret returning value.
func expectStoredRoverSecret(sm *fake.MockSecretManager, zone *adminv1.Zone, value string) {
	sm.EXPECT().Resolve(testifymock.Anything, lookupRoverRef(zone)).
		Return(secretsapi.ResolvedSecret{Value: value, Ref: canonicalRoverRef(zone, value)}, nil).Once()
}

// lookupRoverRef is the checksum-less lookup reference of the zone's rover secret.
func lookupRoverRef(zone *adminv1.Zone) string {
	return environmentSecretRef(zone.Labels[config.EnvironmentLabelKey], roverSecretManagerPath(zone.Name))
}

// setRoverClientSecret overwrites the rover Client's secret reference and its gateways'.
func setRoverClientSecret(zone *adminv1.Zone, secret string) *identityapi.Client {
	GinkgoHelper()
	c := getRoverClient(zone)
	c.Spec.ClientSecret = secret
	Expect(k8sClient.Update(ctx, c)).To(Succeed())
	gateways := gatewaysOf(zone)
	for i := range gateways {
		gateways[i].Spec.Admin.ClientSecret = secret
		Expect(k8sClient.Update(ctx, &gateways[i])).To(Succeed())
	}
	return c
}

// uniqueZone creates a new zone in the API server so that it has a UID.
func uniqueZone(prefix string, env ...string) *adminv1.Zone {
	GinkgoHelper()
	zoneCounter++
	zone := newTestZone(fmt.Sprintf("%s-%d", prefix, zoneCounter))
	if len(env) > 0 {
		zone.Labels[config.EnvironmentLabelKey] = env[0]
	}
	Expect(k8sClient.Create(ctx, zone)).To(Succeed())
	return zone
}

var _ = Describe("Zone rover admin client", func() {
	var sm *fake.MockSecretManager
	h := &ZoneHandler{}

	It("constructs a checksum-less environment secret reference", func() {
		Expect(environmentSecretRef("env-1", roverSecretManagerPath("zone-a"))).
			To(Equal("$<env-1:::zones/zone-a/admin/rover/clientSecret:>"))
	})

	Context("with the secret-manager enabled", func() {
		BeforeEach(func() {
			DeferCleanup(setSecretManagerFeature(true))
			sm = useSecretManager()
		})

		It("shares one client and one secret-manager reference between all gateways of a zone", func() {
			zone := uniqueZone("rover-shared")
			withAIGateway(zone)
			written := expectNewRoverSecret(sm, zone)
			provision(h, zone)

			roverClient := getRoverClient(zone)
			Expect(roverClient.Spec.ClientId).To(Equal("rover"))
			Expect(roverClient.Spec.ClientSecret).To(Equal(canonicalRoverRef(zone, *written)))
			Expect(gatewaysOf(zone)).To(HaveLen(2))
			expectGatewaysUse(zone, roverClient)

			clients := &identityapi.ClientList{}
			Expect(k8sClient.List(ctx, clients, client.InNamespace(zoneNamespace(zone)),
				client.MatchingLabels{config.DomainLabelKey: domainName})).To(Succeed())
			Expect(clients.Items).To(HaveLen(1))
		})

		It("stores a new secret under the zone-scoped rover path of the environment", func() {
			zone := uniqueZone("rover-path")
			expectNewRoverSecret(sm, zone)
			provision(h, zone)

			id, ok := secretsapi.FromRef(getRoverClient(zone).Spec.ClientSecret)
			Expect(ok).To(BeTrue())
			Expect(id).To(HavePrefix(testEnvironment + ":::zones/" + zone.Name + "/admin/rover/clientSecret:"))
		})

		It("keeps the credentials stable across repeated reconciliation and gateway add, reorder and removal", func() {
			zone := uniqueZone("rover-stable")
			expectNewRoverSecret(sm, zone)
			provision(h, zone)
			initial := getRoverClient(zone)

			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

			withAIGateway(zone)
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			expectGatewaysUse(zone, initial)

			zone.Spec.Gateways[0], zone.Spec.Gateways[1] = zone.Spec.Gateways[1], zone.Spec.Gateways[0]
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

			zone.Spec.Gateways = zone.Spec.Gateways[1:]
			zone.Spec.Presets = zone.Spec.Presets[:1]
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

			current := getRoverClient(zone)
			Expect(current.UID).To(Equal(initial.UID))
			Expect(current.Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
			for _, gw := range zone.Status.Gateways {
				Expect(gw.AdminClient.Name).To(Equal(current.Name))
			}
		})

		It("reconciles non-secret settings of an existing client without touching its secret", func() {
			zone := uniqueZone("rover-drift")
			expectNewRoverSecret(sm, zone)
			provision(h, zone)
			initial := getRoverClient(zone)

			drifted := initial.DeepCopy()
			drifted.Spec.ClientId = "drifted"
			Expect(k8sClient.Update(ctx, drifted)).To(Succeed())

			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			current := getRoverClient(zone)
			Expect(current.Spec.ClientId).To(Equal("rover"))
			Expect(current.Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
		})

		It("reuses the credentials of an existing client instead of generating new ones", func() {
			zone := uniqueZone("rover-existing")
			expectNewRoverSecret(sm, zone)
			provision(h, zone)

			existing := getRoverClient(zone)
			existing.Spec.ClientSecret = "$<existing-ref>"
			Expect(k8sClient.Update(ctx, existing)).To(Succeed())

			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			Expect(getRoverClient(zone).Spec.ClientSecret).To(Equal("$<existing-ref>"))
			for _, gw := range gatewaysOf(zone) {
				Expect(gw.Spec.Admin.ClientSecret).To(Equal("$<existing-ref>"))
			}
		})

		It("isolates clients and secrets between zones and environments", func() {
			ensureEnvironment("other")
			zoneA := uniqueZone("rover-iso")
			zoneB := uniqueZone("rover-iso")
			zoneC := uniqueZone("rover-iso", "other")
			writtenA := expectNewRoverSecret(sm, zoneA)
			writtenB := expectNewRoverSecret(sm, zoneB)
			writtenC := expectNewRoverSecret(sm, zoneC)

			for _, z := range []*adminv1.Zone{zoneA, zoneB, zoneC} {
				provision(h, z)
			}

			refs := map[string]struct{}{}
			for z, written := range map[*adminv1.Zone]*string{zoneA: writtenA, zoneB: writtenB, zoneC: writtenC} {
				c := getRoverClient(z)
				Expect(c.Spec.ClientSecret).To(Equal(canonicalRoverRef(z, *written)))
				expectGatewaysUse(z, c)
				refs[c.Spec.ClientSecret] = struct{}{}
			}
			Expect(refs).To(HaveLen(3))
			Expect(*writtenA).NotTo(Equal(*writtenB))
			Expect(*writtenA).NotTo(Equal(*writtenC))
			Expect(*writtenB).NotTo(Equal(*writtenC))
		})

		It("does not overwrite the stored secret when a stale cache reports an existing client as missing", func() {
			zone := uniqueZone("rover-stale")
			written := expectNewRoverSecret(sm, zone)
			provision(h, zone)
			initial := getRoverClient(zone)

			expectStoredRoverSecret(sm, zone, *written)
			stale := newFaultyClient(zone)
			stale.staleGets = -1
			Expect(h.CreateOrUpdate(newTestContextWithClient(zone, stale), zone)).NotTo(Succeed())

			current := getRoverClient(zone)
			Expect(current.Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
			Expect(current.ResourceVersion).To(Equal(initial.ResourceVersion))
			expectGatewaysUse(zone, initial)
		})

		It("keeps the existing client's secret when the cache catches up during the reconciliation", func() {
			zone := uniqueZone("rover-catchup")
			written := expectNewRoverSecret(sm, zone)
			provision(h, zone)
			initial := getRoverClient(zone)

			expectStoredRoverSecret(sm, zone, *written)
			stale := newFaultyClient(zone)
			stale.staleGets = 1
			Expect(h.CreateOrUpdate(newTestContextWithClient(zone, stale), zone)).To(Succeed())

			Expect(getRoverClient(zone).Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
			expectGatewaysUse(zone, initial)
		})

		It("reuses the stored secret on retry after the client creation failed", func() {
			zone := uniqueZone("rover-createfail")
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			markSubResourcesReady(zone)

			written := expectNewRoverSecret(sm, zone)
			failing := newFaultyClient(zone)
			failing.createErr = apierrors.NewServiceUnavailable("api server unavailable")
			Expect(h.CreateOrUpdate(newTestContextWithClient(zone, failing), zone)).NotTo(Succeed())
			expectNoRoverClient(zone)
			Expect(gatewaysOf(zone)).To(BeEmpty())
			Expect(*written).NotTo(BeEmpty())

			expectStoredRoverSecret(sm, zone, *written)
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			roverClient := getRoverClient(zone)
			Expect(roverClient.Spec.ClientSecret).To(Equal(canonicalRoverRef(zone, *written)))
			expectGatewaysUse(zone, roverClient)
		})

		It("does not generate a secret when the secret-manager lookup fails", func() {
			zone := uniqueZone("rover-smread")
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			markSubResourcesReady(zone)

			sm.EXPECT().Resolve(testifymock.Anything, lookupRoverRef(zone)).
				Return(secretsapi.ResolvedSecret{}, fmt.Errorf("secret-manager unavailable")).Once()
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).NotTo(Succeed())
			sm.AssertNotCalled(GinkgoT(), "UpsertEnvironment", testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything)
			expectNoRoverClient(zone)
			Expect(gatewaysOf(zone)).To(BeEmpty())

			written := expectNewRoverSecret(sm, zone)
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			roverClient := getRoverClient(zone)
			Expect(roverClient.Spec.ClientSecret).To(Equal(canonicalRoverRef(zone, *written)))
			expectGatewaysUse(zone, roverClient)
		})

		It("returns an error and publishes nothing when storing the secret fails, then converges", func() {
			zone := uniqueZone("rover-smfail")
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			markSubResourcesReady(zone)

			sm.EXPECT().Resolve(testifymock.Anything, lookupRoverRef(zone)).
				Return(secretsapi.ResolvedSecret{}, secretsapi.ErrNotFound).Once()
			sm.EXPECT().UpsertEnvironment(testifymock.Anything, testEnvironment, testifymock.Anything, roverSecretValueOption(zone)).
				Return(nil, fmt.Errorf("secret-manager unavailable")).Once()
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).NotTo(Succeed())
			expectNoRoverClient(zone)
			Expect(gatewaysOf(zone)).To(BeEmpty())

			written := expectNewRoverSecret(sm, zone)
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			roverClient := getRoverClient(zone)
			Expect(roverClient.Spec.ClientSecret).To(Equal(canonicalRoverRef(zone, *written)))
			expectGatewaysUse(zone, roverClient)
		})

		It("does not treat client read errors as a missing client", func() {
			zone := uniqueZone("rover-readerr")
			expectNewRoverSecret(sm, zone)
			provision(h, zone)
			initial := getRoverClient(zone)

			failing := newFaultyClient(zone)
			failing.getErr = apierrors.NewServiceUnavailable("api server unavailable")
			Expect(h.CreateOrUpdate(newTestContextWithClient(zone, failing), zone)).NotTo(Succeed())
			Expect(getRoverClient(zone).Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
		})

		It("never changes existing credentials when reconciled concurrently", func() {
			zone := uniqueZone("rover-concurrent")
			expectNewRoverSecret(sm, zone)
			provision(h, zone)
			initial := getRoverClient(zone)

			// The client exists, so the workers must not call the secret-manager.
			const workers = 4
			var wg sync.WaitGroup
			for range workers {
				wg.Add(1)
				local := zone.DeepCopy()
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					testCtx := newTestContext(local)
					hc := newTestHandlingContext(testCtx, local)
					Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
					Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
					// Optimistic-concurrency conflicts are acceptable; secret changes are not.
					c, err := ensureRoverClient(testCtx, hc)
					if err == nil {
						Expect(c.Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
					}
				}()
			}
			wg.Wait()

			Expect(getRoverClient(zone).Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
		})

		Context("with a secret already stored in the secret-manager", func() {
			It("uses the canonical reference of the stored secret for a new client without writing", func() {
				zone := uniqueZone("rover-stored")
				withAIGateway(zone)
				expectStoredRoverSecret(sm, zone, "stored-value")

				provision(h, zone)

				roverClient := getRoverClient(zone)
				Expect(roverClient.Spec.ClientSecret).To(Equal(canonicalRoverRef(zone, "stored-value")))
				Expect(roverClient.Spec.ClientSecret).NotTo(Equal(lookupRoverRef(zone)))
				Expect(gatewaysOf(zone)).To(HaveLen(2))
				expectGatewaysUse(zone, roverClient)
			})
		})

		Context("with an existing client holding the checksum-less lookup reference", func() {
			var zone *adminv1.Zone
			var stored string

			BeforeEach(func() {
				zone = uniqueZone("rover-hashless")
				withAIGateway(zone)
				written := expectNewRoverSecret(sm, zone)
				provision(h, zone)
				stored = *written
				setRoverClientSecret(zone, lookupRoverRef(zone))
			})

			It("replaces only the reference with the canonical one and stays stable", func() {
				before := getRoverClient(zone)
				expectStoredRoverSecret(sm, zone, stored)

				Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

				repaired := getRoverClient(zone)
				Expect(repaired.UID).To(Equal(before.UID))
				Expect(repaired.Spec.ClientSecret).To(Equal(canonicalRoverRef(zone, stored)))
				Expect(repaired.Spec.ClientId).To(Equal(before.Spec.ClientId))
				Expect(repaired.Spec.Realm).To(Equal(before.Spec.Realm))
				expectGatewaysUse(zone, repaired)

				// A canonical reference needs no further lookup; unexpected mock calls fail the spec.
				Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
				Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
				current := getRoverClient(zone)
				Expect(current.ResourceVersion).To(Equal(repaired.ResourceVersion))
			})

			DescribeTable("fails without changing the client, gateways or secret when resolution fails",
				func(resolveErr error) {
					before := getRoverClient(zone)
					sm.EXPECT().Resolve(testifymock.Anything, lookupRoverRef(zone)).
						Return(secretsapi.ResolvedSecret{}, resolveErr).Once()

					Expect(h.CreateOrUpdate(newTestContext(zone), zone)).NotTo(Succeed())

					current := getRoverClient(zone)
					Expect(current.ResourceVersion).To(Equal(before.ResourceVersion))
					Expect(current.Spec.ClientSecret).To(Equal(lookupRoverRef(zone)))
					for _, gw := range gatewaysOf(zone) {
						Expect(gw.Spec.Admin.ClientSecret).To(Equal(lookupRoverRef(zone)))
					}
				},
				Entry("secret-manager unavailable", fmt.Errorf("secret-manager unavailable")),
				Entry("stored secret not found", secretsapi.ErrNotFound),
			)

			It("keeps a reference changed concurrently after the client was read", func() {
				sm.EXPECT().Resolve(testifymock.Anything, lookupRoverRef(zone)).
					RunAndReturn(func(context.Context, string) (secretsapi.ResolvedSecret, error) {
						changed := getRoverClient(zone)
						changed.Spec.ClientSecret = "$<established-ref>"
						Expect(k8sClient.Update(ctx, changed)).To(Succeed())
						return secretsapi.ResolvedSecret{Value: stored, Ref: canonicalRoverRef(zone, stored)}, nil
					}).Once()

				Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

				Expect(getRoverClient(zone).Spec.ClientSecret).To(Equal("$<established-ref>"))
			})

			It("is left unchanged when the secret-manager is disabled", func() {
				DeferCleanup(setSecretManagerFeature(false))
				before := getRoverClient(zone)

				Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

				Expect(getRoverClient(zone).ResourceVersion).To(Equal(before.ResourceVersion))
			})
		})

		It("keeps an established checksum reference when a stale cache catches up during the reconciliation", func() {
			zone := uniqueZone("rover-catchup-ref")
			written := expectNewRoverSecret(sm, zone)
			provision(h, zone)
			established := setRoverClientSecret(zone, "$<established-ref>")

			expectStoredRoverSecret(sm, zone, *written)
			stale := newFaultyClient(zone)
			stale.staleGets = 1
			Expect(h.CreateOrUpdate(newTestContextWithClient(zone, stale), zone)).To(Succeed())

			current := getRoverClient(zone)
			Expect(current.Spec.ClientSecret).To(Equal("$<established-ref>"))
			Expect(current.ResourceVersion).To(Equal(established.ResourceVersion))
			expectGatewaysUse(zone, current)
		})

		It("keeps the client when the zone is deleted", func() {
			zone := uniqueZone("rover-delete")
			expectNewRoverSecret(sm, zone)
			provision(h, zone)
			initial := getRoverClient(zone)

			Expect(h.Delete(newTestContext(zone), zone)).To(Succeed())
			Expect(getRoverClient(zone).UID).To(Equal(initial.UID))
		})
	})

	Context("with the secret-manager disabled", func() {
		BeforeEach(func() {
			DeferCleanup(setSecretManagerFeature(false))
			// No expectations: any secret-manager call fails the spec.
			useSecretManager()
		})

		It("keeps a generated secret inline and stable without using the secret-manager", func() {
			zone := uniqueZone("rover-inline")
			withAIGateway(zone)
			provision(h, zone)

			roverClient := getRoverClient(zone)
			Expect(roverClient.Spec.ClientSecret).NotTo(BeEmpty())
			Expect(secretsapi.IsRef(roverClient.Spec.ClientSecret)).To(BeFalse())
			expectGatewaysUse(zone, roverClient)

			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			Expect(getRoverClient(zone).Spec.ClientSecret).To(Equal(roverClient.Spec.ClientSecret))
		})

		It("does not overwrite the inline secret when a stale cache reports the client as missing", func() {
			zone := uniqueZone("rover-inline-stale")
			provision(h, zone)
			initial := getRoverClient(zone)

			stale := newFaultyClient(zone)
			stale.staleGets = -1
			Expect(h.CreateOrUpdate(newTestContextWithClient(zone, stale), zone)).NotTo(Succeed())

			current := getRoverClient(zone)
			Expect(current.Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
			Expect(current.ResourceVersion).To(Equal(initial.ResourceVersion))
			expectGatewaysUse(zone, initial)
		})
	})
})
