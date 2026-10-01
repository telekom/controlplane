// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"
	"sync"

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
	var sm *memorySecretManager
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
			provision(h, zone)

			roverClient := getRoverClient(zone)
			Expect(roverClient.Spec.ClientId).To(Equal("rover"))
			Expect(secretsapi.IsRef(roverClient.Spec.ClientSecret)).To(BeTrue())
			Expect(sm.publishedValues()).To(HaveLen(1))
			Expect(sm.valueOf(roverClient.Spec.ClientSecret)).To(Equal(sm.publishedValues()[0]))
			Expect(gatewaysOf(zone)).To(HaveLen(2))
			expectGatewaysUse(zone, roverClient)

			clients := &identityapi.ClientList{}
			Expect(k8sClient.List(ctx, clients, client.InNamespace(zoneNamespace(zone)),
				client.MatchingLabels{config.DomainLabelKey: domainName})).To(Succeed())
			Expect(clients.Items).To(HaveLen(1))
		})

		It("stores a new secret under the zone-scoped rover path of the environment", func() {
			zone := uniqueZone("rover-path")
			provision(h, zone)

			id, ok := secretsapi.FromRef(getRoverClient(zone).Spec.ClientSecret)
			Expect(ok).To(BeTrue())
			Expect(id).To(HavePrefix(testEnvironment + ":::zones/" + zone.Name + "/admin/rover/clientSecret:"))
		})

		It("keeps the credentials stable across repeated reconciliation and gateway add, reorder and removal", func() {
			zone := uniqueZone("rover-stable")
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
			Expect(sm.publishedValues()).To(HaveLen(1))
			for _, gw := range zone.Status.Gateways {
				Expect(gw.AdminClient.Name).To(Equal(current.Name))
			}
		})

		It("reconciles non-secret settings of an existing client without touching its secret", func() {
			zone := uniqueZone("rover-drift")
			provision(h, zone)
			initial := getRoverClient(zone)

			drifted := initial.DeepCopy()
			drifted.Spec.ClientId = "drifted"
			Expect(k8sClient.Update(ctx, drifted)).To(Succeed())

			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			current := getRoverClient(zone)
			Expect(current.Spec.ClientId).To(Equal("rover"))
			Expect(current.Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
			Expect(sm.publishedValues()).To(HaveLen(1))
		})

		It("reuses the credentials of an existing client instead of generating new ones", func() {
			zone := uniqueZone("rover-existing")
			provision(h, zone)

			existing := getRoverClient(zone)
			existing.Spec.ClientSecret = "$<existing-ref>"
			Expect(k8sClient.Update(ctx, existing)).To(Succeed())

			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			Expect(getRoverClient(zone).Spec.ClientSecret).To(Equal("$<existing-ref>"))
			Expect(sm.publishedValues()).To(HaveLen(1))
			for _, gw := range gatewaysOf(zone) {
				Expect(gw.Spec.Admin.ClientSecret).To(Equal("$<existing-ref>"))
			}
		})

		It("isolates clients and secrets between zones and environments", func() {
			ensureEnvironment("other")
			zoneA := uniqueZone("rover-iso")
			zoneB := uniqueZone("rover-iso")
			zoneC := uniqueZone("rover-iso", "other")

			for _, z := range []*adminv1.Zone{zoneA, zoneB, zoneC} {
				provision(h, z)
			}

			refs := map[string]struct{}{}
			values := map[string]struct{}{}
			for _, z := range []*adminv1.Zone{zoneA, zoneB, zoneC} {
				c := getRoverClient(z)
				expectGatewaysUse(z, c)
				refs[c.Spec.ClientSecret] = struct{}{}
				values[sm.valueOf(c.Spec.ClientSecret)] = struct{}{}
			}
			Expect(refs).To(HaveLen(3))
			Expect(values).To(HaveLen(3))
		})

		It("does not overwrite the stored secret when a stale cache reports an existing client as missing", func() {
			zone := uniqueZone("rover-stale")
			provision(h, zone)
			initial := getRoverClient(zone)
			storedValue := sm.valueOf(initial.Spec.ClientSecret)

			stale := newFaultyClient(zone)
			stale.staleGets = -1
			Expect(h.CreateOrUpdate(newTestContextWithClient(zone, stale), zone)).NotTo(Succeed())

			Expect(sm.publishedValues()).To(HaveLen(1))
			Expect(sm.valueOf(initial.Spec.ClientSecret)).To(Equal(storedValue))
			current := getRoverClient(zone)
			Expect(current.Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
			Expect(current.ResourceVersion).To(Equal(initial.ResourceVersion))
			expectGatewaysUse(zone, initial)
		})

		It("keeps the existing client's secret when the cache catches up during the reconciliation", func() {
			zone := uniqueZone("rover-catchup")
			provision(h, zone)
			initial := getRoverClient(zone)

			stale := newFaultyClient(zone)
			stale.staleGets = 1
			Expect(h.CreateOrUpdate(newTestContextWithClient(zone, stale), zone)).To(Succeed())

			Expect(sm.publishedValues()).To(HaveLen(1))
			Expect(getRoverClient(zone).Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
			expectGatewaysUse(zone, initial)
		})

		It("reuses the stored secret on retry after the client creation failed", func() {
			zone := uniqueZone("rover-createfail")
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			markSubResourcesReady(zone)

			failing := newFaultyClient(zone)
			failing.createErr = apierrors.NewServiceUnavailable("api server unavailable")
			Expect(h.CreateOrUpdate(newTestContextWithClient(zone, failing), zone)).NotTo(Succeed())
			expectNoRoverClient(zone)
			Expect(gatewaysOf(zone)).To(BeEmpty())
			Expect(sm.publishedValues()).To(HaveLen(1))

			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			roverClient := getRoverClient(zone)
			Expect(sm.publishedValues()).To(HaveLen(1))
			Expect(sm.valueOf(roverClient.Spec.ClientSecret)).To(Equal(sm.publishedValues()[0]))
			expectGatewaysUse(zone, roverClient)
		})

		It("does not generate a secret when the secret-manager lookup fails", func() {
			zone := uniqueZone("rover-smread")
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			markSubResourcesReady(zone)

			sm.getErr = fmt.Errorf("secret-manager unavailable")
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).NotTo(Succeed())
			Expect(sm.publishedValues()).To(BeEmpty())
			expectNoRoverClient(zone)
			Expect(gatewaysOf(zone)).To(BeEmpty())

			sm.getErr = nil
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			Expect(sm.publishedValues()).To(HaveLen(1))
			expectGatewaysUse(zone, getRoverClient(zone))
		})

		It("returns an error and publishes nothing when storing the secret fails, then converges", func() {
			zone := uniqueZone("rover-smfail")
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			markSubResourcesReady(zone)

			sm.failNext = 1
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).NotTo(Succeed())
			expectNoRoverClient(zone)
			Expect(gatewaysOf(zone)).To(BeEmpty())

			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			roverClient := getRoverClient(zone)
			Expect(sm.publishedValues()).To(HaveLen(1))
			expectGatewaysUse(zone, roverClient)
		})

		It("does not treat client read errors as a missing client", func() {
			zone := uniqueZone("rover-readerr")
			provision(h, zone)
			initial := getRoverClient(zone)

			failing := newFaultyClient(zone)
			failing.getErr = apierrors.NewServiceUnavailable("api server unavailable")
			Expect(h.CreateOrUpdate(newTestContextWithClient(zone, failing), zone)).NotTo(Succeed())
			Expect(sm.publishedValues()).To(HaveLen(1))
			Expect(getRoverClient(zone).Spec.ClientSecret).To(Equal(initial.Spec.ClientSecret))
		})

		It("never changes existing credentials when reconciled concurrently", func() {
			zone := uniqueZone("rover-concurrent")
			provision(h, zone)
			initial := getRoverClient(zone)

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
			Expect(sm.publishedValues()).To(HaveLen(1))
		})

		It("keeps the client when the zone is deleted", func() {
			zone := uniqueZone("rover-delete")
			provision(h, zone)
			initial := getRoverClient(zone)

			Expect(h.Delete(newTestContext(zone), zone)).To(Succeed())
			Expect(getRoverClient(zone).UID).To(Equal(initial.UID))
		})
	})

	Context("with the secret-manager disabled", func() {
		BeforeEach(func() {
			DeferCleanup(setSecretManagerFeature(false))
			sm = useSecretManager()
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
			Expect(sm.publishedValues()).To(BeEmpty())
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
