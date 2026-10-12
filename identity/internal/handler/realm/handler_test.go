// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/providerca"
	common "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/mocks/keycloakservice"
	"github.com/telekom/controlplane/identity/test/tlsutil"
	secrets "github.com/telekom/controlplane/secret-manager/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	testEnv_      = "test"
	adminURLA     = "https://keycloak-a.example.com/auth/admin/realms/"
	tokenURLA     = "https://keycloak-a.example.com/auth/realms/master/protocol/openid-connect/token"
	adminURLB     = "https://keycloak-b.example.com/auth/admin/realms/"
	secretRef     = "$<secret-id>"
	literalSecret = "literal-password"
)

var nsCounter int

func newNamespace() string {
	nsCounter++
	name := fmt.Sprintf("realm-handler-%d-%d", GinkgoParallelProcess(), nsCounter)
	Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
	return name
}

func envLabels() map[string]string { return map[string]string{cconfig.EnvironmentLabelKey: testEnv_} }

func handlerCtx() context.Context {
	ctx := context.Background()
	ctx = client.WithClient(ctx, client.NewJanitorClient(client.NewScopedClient(k8sClient, testEnv_)))
	return contextutil.WithRecorder(ctx, record.NewFakeRecorder(10))
}

func createIdp(ns, adminURL, password string, ref *common.CABundleConfigMapRef) *identityv1.IdentityProvider {
	idp := &identityv1.IdentityProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "idp", Namespace: ns, Labels: envLabels()},
		Spec: identityv1.IdentityProviderSpec{
			AdminUrl: adminURL, AdminClientId: "admin-cli", AdminUserName: "admin", AdminPassword: password,
		},
	}
	if ref != nil {
		idp.Spec.TLS = &common.ProviderTLS{CABundleConfigMapRef: ref}
	}
	Expect(k8sClient.Create(context.Background(), idp)).To(Succeed())
	markIdpCurrent(idp)
	return idp
}

// markIdpCurrent mirrors a successful IdentityProvider reconcile of its current generation.
func markIdpCurrent(idp *identityv1.IdentityProvider) {
	idp.Status.AdminUrl = idp.Spec.AdminUrl
	idp.Status.AdminTokenUrl = keycloak.DetermineAdminTokenUrlFrom(idp.Spec.AdminUrl, keycloak.MasterRealm)
	idp.SetCondition(condition.NewReadyCondition("Ready", "ready"))
	idp.SetCondition(identityv1.NewTrustReadyCondition("resolved"))
	for i := range idp.Status.Conditions {
		idp.Status.Conditions[i].ObservedGeneration = idp.Generation
	}
	Expect(k8sClient.Status().Update(context.Background(), idp)).To(Succeed())
}

// updateIdpSpec changes the spec only; status and conditions still describe the old generation.
func updateIdpSpec(idp *identityv1.IdentityProvider, adminURL string, ref *common.CABundleConfigMapRef) {
	ctx := context.Background()
	Expect(k8sClient.Get(ctx, pkgclient.ObjectKeyFromObject(idp), idp)).To(Succeed())
	idp.Spec.AdminUrl = adminURL
	idp.Spec.TLS = nil
	if ref != nil {
		idp.Spec.TLS = &common.ProviderTLS{CABundleConfigMapRef: ref}
	}
	Expect(k8sClient.Update(ctx, idp)).To(Succeed())
}

func updateIdp(idp *identityv1.IdentityProvider, adminURL string, ref *common.CABundleConfigMapRef) {
	updateIdpSpec(idp, adminURL, ref)
	markIdpCurrent(idp)
}

func createRealm(ns string, status *identityv1.RealmStatus) *identityv1.Realm {
	ctx := context.Background()
	realm := &identityv1.Realm{
		ObjectMeta: metav1.ObjectMeta{Name: "test-realm", Namespace: ns, Labels: envLabels()},
		Spec:       identityv1.RealmSpec{IdentityProvider: &common.ObjectRef{Namespace: ns, Name: "idp"}},
	}
	Expect(k8sClient.Create(ctx, realm)).To(Succeed())
	if status != nil {
		realm.Status = *status
		Expect(k8sClient.Status().Update(ctx, realm)).To(Succeed())
	}
	Expect(k8sClient.Get(ctx, pkgclient.ObjectKeyFromObject(realm), realm)).To(Succeed())
	return realm
}

func committedStatus() *identityv1.RealmStatus {
	adminURL, tokenURL := adminURLA, tokenURLA
	return &identityv1.RealmStatus{
		IssuerUrl:     keycloak.DetermineIssuerUrlFrom(adminURL, "test-realm"),
		AdminClientId: "admin-cli", AdminUserName: "admin", AdminPassword: literalSecret,
		AdminUrl: adminURL, AdminTokenUrl: tokenURL,
	}
}

func createCA(ns, name string, labeled bool) *common.CABundleConfigMapRef {
	ca, err := tlsutil.NewCA(name)
	Expect(err).NotTo(HaveOccurred())
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       map[string]string{"ca.crt": ca.PEM},
	}
	if labeled {
		cm.Labels = map[string]string{common.ProviderCALabelKey: common.ProviderCALabelValue}
	}
	Expect(k8sClient.Create(context.Background(), cm)).To(Succeed())
	return &common.CABundleConfigMapRef{Name: name, Key: "ca.crt"}
}

func readRealm(realm *identityv1.Realm) *identityv1.Realm {
	out := &identityv1.Realm{}
	Expect(k8sClient.Get(context.Background(), pkgclient.ObjectKeyFromObject(realm), out)).To(Succeed())
	return out
}

func okService() *keycloakservice.MockKeycloakService {
	svc := keycloakservice.NewMockKeycloakService(GinkgoT())
	svc.EXPECT().CreateOrReplaceRealm(mock.Anything, mock.Anything).Return(nil)
	svc.EXPECT().DeleteSecretRotationPolicy(mock.Anything, "test-realm").Return(nil)
	svc.EXPECT().ConfigureClientScopes(mock.Anything, "test-realm", mock.Anything).Return(nil)
	return svc
}

// committedFactory counts acquisitions and lets check inspect the Realm passed to the factory.
func committedFactory(svc keycloak.KeycloakService, calls *int, check func(realm *identityv1.Realm)) keycloak.ServiceFactory {
	return keycloak.ServiceFactoryFunc(func(_ context.Context, realm *identityv1.Realm) (keycloak.KeycloakService, error) {
		*calls++
		if check != nil {
			check(realm)
		}
		return svc, nil
	})
}

func noFactory() keycloak.ServiceFactory {
	return keycloak.ServiceFactoryFunc(func(context.Context, *identityv1.Realm) (keycloak.KeycloakService, error) {
		Fail("no remote request expected")
		return nil, nil
	})
}

func expectBlocked(err error) {
	Expect(err).To(HaveOccurred())
	var be ctrlerrors.BlockedError
	Expect(errors.As(err, &be)).To(BeTrue(), "expected BlockedError, got %v", err)
}

func trustCondition(realm *identityv1.Realm) *metav1.Condition {
	return meta.FindStatusCondition(realm.Status.Conditions, identityv1.TrustReadyConditionType)
}

var _ = Describe("HandlerRealm", func() {
	var ns string

	BeforeEach(func() {
		ns = newNamespace()
		original := secrets.Get
		secrets.Get = func(_ context.Context, ref string) (string, error) {
			if ref == secretRef {
				return "resolved-password", nil
			}
			return ref, nil
		}
		DeferCleanup(func() { secrets.Get = original })
	})

	Context("CreateOrUpdate", func() {
		It("returns an error when the realm is nil", func() {
			Expect(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(context.Background(), nil)).
				To(MatchError(ContainSubstring("realm is nil")))
		})

		It("blocks when the IdentityProvider is not found", func() {
			realm := createRealm(ns, nil)
			expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
		})

		It("blocks when the IdentityProvider is not ready", func() {
			idp := &identityv1.IdentityProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "idp", Namespace: ns, Labels: envLabels()},
				Spec:       identityv1.IdentityProviderSpec{AdminUrl: adminURLA, AdminClientId: "c", AdminUserName: "u", AdminPassword: "p"},
			}
			Expect(k8sClient.Create(context.Background(), idp)).To(Succeed())
			realm := createRealm(ns, nil)
			realm.SetCondition(identityv1.NewTrustReadyCondition("stale"))
			expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
			Expect(trustCondition(realm).Status).To(Equal(metav1.ConditionFalse), "a previous True must not remain stale")
		})

		It("reports the committed trust when the IdentityProvider is not ready", func() {
			idp := &identityv1.IdentityProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "idp", Namespace: ns, Labels: envLabels()},
				Spec:       identityv1.IdentityProviderSpec{AdminUrl: adminURLA, AdminClientId: "c", AdminUserName: "u", AdminPassword: "p"},
			}
			Expect(k8sClient.Create(context.Background(), idp)).To(Succeed())
			status := committedStatus()
			status.TLS = &common.ProviderTLS{CABundleConfigMapRef: createCA(ns, "committed-ca", true)}
			realm := createRealm(ns, status)
			expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
			Expect(trustCondition(realm).Status).To(Equal(metav1.ConditionTrue))

			By("removing the committed CA, trust becomes not ready without fallback")
			Expect(k8sClient.Delete(context.Background(), &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "committed-ca", Namespace: ns}})).To(Succeed())
			expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
			Expect(trustCondition(realm).Status).To(Equal(metav1.ConditionFalse))
			Expect(trustCondition(realm).Message).To(ContainSubstring("committed-ca"))
		})

		Context("with an IdentityProvider whose observed state is stale", func() {
			failSecrets := func() {
				original := secrets.Get
				secrets.Get = func(context.Context, string) (string, error) {
					Fail("no secret resolution expected")
					return "", nil
				}
				DeferCleanup(func() { secrets.Get = original })
			}

			It("does not record spec B with status A and an old-generation Ready", func() {
				idp := createIdp(ns, adminURLA, secretRef, nil)
				realm := createRealm(ns, nil)
				updateIdpSpec(idp, adminURLB, nil)
				Expect(idp.Status.AdminUrl).To(Equal(adminURLA))
				failSecrets()

				expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
				Expect(identityv1.ClassifyRealmSnapshot(&realm.Status)).To(Equal(identityv1.RealmSnapshotEmpty))
				Expect(trustCondition(realm).Status).To(Equal(metav1.ConditionFalse))
			})

			It("does not record when current-generation conditions describe stale status URLs", func() {
				idp := createIdp(ns, adminURLA, literalSecret, nil)
				realm := createRealm(ns, nil)
				updateIdpSpec(idp, adminURLB, nil)
				for i := range idp.Status.Conditions {
					idp.Status.Conditions[i].ObservedGeneration = idp.Generation
				}
				Expect(k8sClient.Status().Update(context.Background(), idp)).To(Succeed())
				failSecrets()

				expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
				Expect(identityv1.ClassifyRealmSnapshot(&realm.Status)).To(Equal(identityv1.RealmSnapshotEmpty))
			})

			It("does not record changed credentials or trust before the IdentityProvider observed them", func() {
				ref := createCA(ns, "idp-ca", true)
				idp := createIdp(ns, adminURLA, literalSecret, nil)
				realm := createRealm(ns, committedStatus())
				Expect(k8sClient.Get(context.Background(), pkgclient.ObjectKeyFromObject(idp), idp)).To(Succeed())
				idp.Spec.AdminPassword = "new-password"
				idp.Spec.TLS = &common.ProviderTLS{CABundleConfigMapRef: ref}
				Expect(k8sClient.Update(context.Background(), idp)).To(Succeed())
				failSecrets()

				expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
				Expect(realm.Status.AdminPassword).To(Equal(literalSecret))
				Expect(realm.Status.TLS).To(BeNil())
			})

			It("blocks when TrustReady was not observed as true for the current generation", func() {
				idp := createIdp(ns, adminURLA, literalSecret, nil)
				idp.SetCondition(identityv1.NewTrustNotReadyCondition("broken"))
				for i := range idp.Status.Conditions {
					idp.Status.Conditions[i].ObservedGeneration = idp.Generation
				}
				Expect(k8sClient.Status().Update(context.Background(), idp)).To(Succeed())
				realm := createRealm(ns, nil)
				failSecrets()
				expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
			})
		})

		It("records the complete connection of a new realm in status before the first remote request", func() {
			createIdp(ns, adminURLA, secretRef, nil)
			realm := createRealm(ns, nil)
			calls := 0
			factory := committedFactory(okService(), &calls, func(r *identityv1.Realm) {
				Expect(identityv1.ClassifyRealmSnapshot(&r.Status)).To(Equal(identityv1.RealmSnapshotComplete))
				Expect(r.Status.AdminPassword).To(Equal(secretRef), "reference must not be replaced by its resolved value")
				Expect(r.Status.TLS).To(BeNil())
			})

			Expect(NewHandlerRealm(factory, providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm)).To(Succeed())
			Expect(calls).To(Equal(1))

			By("persisting it with the normal status update")
			Expect(k8sClient.Status().Update(context.Background(), realm)).To(Succeed())
			persisted := readRealm(realm)
			Expect(persisted.Status.AdminPassword).To(Equal(secretRef))
			Expect(meta.IsStatusConditionTrue(persisted.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			Expect(trustCondition(persisted).Status).To(Equal(metav1.ConditionTrue))
		})

		It("preserves literal credential inputs", func() {
			createIdp(ns, adminURLA, literalSecret, nil)
			realm := createRealm(ns, nil)
			calls := 0
			Expect(NewHandlerRealm(committedFactory(okService(), &calls, nil), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).
				CreateOrUpdate(handlerCtx(), realm)).To(Succeed())
			Expect(realm.Status.AdminPassword).To(Equal(literalSecret))
		})

		It("follows a changed IdentityProvider target without migrating the old one", func() {
			createIdp(ns, adminURLB, literalSecret, nil)
			realm := createRealm(ns, committedStatus())
			calls := 0
			factory := committedFactory(okService(), &calls, func(r *identityv1.Realm) {
				Expect(r.Status.AdminUrl).To(Equal(adminURLB))
				Expect(r.Status.IssuerUrl).To(Equal(keycloak.DetermineIssuerUrlFrom(adminURLB, "test-realm")))
			})
			Expect(NewHandlerRealm(factory, providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm)).To(Succeed())
			Expect(calls).To(Equal(1))
			Expect(realm.Status.AdminUrl).To(Equal(adminURLB))
		})

		It("records changed credential references", func() {
			idp := createIdp(ns, adminURLA, literalSecret, nil)
			realm := createRealm(ns, committedStatus())
			Expect(k8sClient.Get(context.Background(), pkgclient.ObjectKeyFromObject(idp), idp)).To(Succeed())
			idp.Spec.AdminPassword = secretRef
			Expect(k8sClient.Update(context.Background(), idp)).To(Succeed())
			markIdpCurrent(idp)

			calls := 0
			Expect(NewHandlerRealm(committedFactory(okService(), &calls, nil), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).
				CreateOrUpdate(handlerCtx(), realm)).To(Succeed())
			Expect(realm.Status.AdminPassword).To(Equal(secretRef))
		})

		It("completes a partial connection status from the IdentityProvider", func() {
			createIdp(ns, adminURLA, literalSecret, nil)
			realm := createRealm(ns, &identityv1.RealmStatus{AdminUrl: adminURLA})
			calls := 0
			Expect(NewHandlerRealm(committedFactory(okService(), &calls, nil), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).
				CreateOrUpdate(handlerCtx(), realm)).To(Succeed())
			Expect(identityv1.ClassifyRealmSnapshot(&realm.Status)).To(Equal(identityv1.RealmSnapshotComplete))
		})

		It("adds the validated trust reference to an existing connection", func() {
			ref := createCA(ns, "idp-ca", true)
			createIdp(ns, adminURLA, literalSecret, ref)
			realm := createRealm(ns, committedStatus())
			calls := 0
			factory := committedFactory(okService(), &calls, func(r *identityv1.Realm) {
				Expect(r.Status.TLS.Ref()).To(Equal(ref))
			})
			Expect(NewHandlerRealm(factory, providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm)).To(Succeed())
			Expect(calls).To(Equal(1))
		})

		It("keeps the prior connection when the new trust is invalid", func() {
			ref := createCA(ns, "unlabeled-ca", false)
			createIdp(ns, adminURLA, literalSecret, ref)
			realm := createRealm(ns, committedStatus())

			expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
			Expect(trustCondition(realm).Status).To(Equal(metav1.ConditionFalse))
			Expect(realm.Status.TLS).To(BeNil())
		})

		It("blocks a missing CA ConfigMap", func() {
			createIdp(ns, adminURLA, literalSecret, &common.CABundleConfigMapRef{Name: "missing", Key: "ca.crt"})
			realm := createRealm(ns, nil)
			expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
			Expect(identityv1.ClassifyRealmSnapshot(&realm.Status)).To(Equal(identityv1.RealmSnapshotEmpty))
		})

		It("rejects cross-namespace inheritance of a CA reference", func() {
			other := newNamespace()
			ref := createCA(other, "idp-ca", true)
			idp := createIdp(other, adminURLA, literalSecret, ref)
			realm := createRealm(ns, nil)
			realm.Spec.IdentityProvider = common.ObjectRefFromObject(idp)
			Expect(k8sClient.Update(context.Background(), realm)).To(Succeed())

			expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
			Expect(trustCondition(realm).Status).To(Equal(metav1.ConditionFalse))
		})

		It("requires HTTPS with a custom CA", func() {
			ref := createCA(ns, "idp-ca", true)
			createIdp(ns, "http://keycloak-a.example.com/auth/admin/realms/", literalSecret, ref)
			realm := createRealm(ns, nil)
			expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm))
		})

		It("records a trust change even when authentication fails", func() {
			ref := createCA(ns, "idp-ca", true)
			idp := createIdp(ns, adminURLA, literalSecret, nil)
			realm := createRealm(ns, committedStatus())
			updateIdp(idp, adminURLA, ref)

			factory := keycloak.ServiceFactoryFunc(func(context.Context, *identityv1.Realm) (keycloak.KeycloakService, error) {
				return nil, fmt.Errorf("failed to retrieve token")
			})
			Expect(NewHandlerRealm(factory, providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).CreateOrUpdate(handlerCtx(), realm)).To(HaveOccurred())
			Expect(realm.Status.TLS.Ref()).To(Equal(ref))
		})

		It("configures the secret rotation policy when requested", func() {
			createIdp(ns, adminURLA, literalSecret, nil)
			realm := createRealm(ns, nil)
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				GracePeriod:             metav1.Duration{Duration: 5 * time.Minute},
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}
			svc := keycloakservice.NewMockKeycloakService(GinkgoT())
			svc.EXPECT().CreateOrReplaceRealm(mock.Anything, mock.Anything).Return(nil)
			svc.EXPECT().ConfigureSecretRotationPolicy(mock.Anything, "test-realm", realm.Spec.SecretRotation).Return(nil)
			svc.EXPECT().ConfigureClientScopes(mock.Anything, "test-realm", mock.Anything).Return(nil)
			calls := 0
			Expect(NewHandlerRealm(committedFactory(svc, &calls, nil), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).
				CreateOrUpdate(handlerCtx(), realm)).To(Succeed())
		})

		It("returns remote errors", func() {
			createIdp(ns, adminURLA, literalSecret, nil)
			realm := createRealm(ns, nil)
			svc := keycloakservice.NewMockKeycloakService(GinkgoT())
			svc.EXPECT().CreateOrReplaceRealm(mock.Anything, mock.Anything).Return(fmt.Errorf("keycloak 500"))
			calls := 0
			Expect(NewHandlerRealm(committedFactory(svc, &calls, nil), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).
				CreateOrUpdate(handlerCtx(), realm)).To(MatchError(ContainSubstring("failed to create or update realm")))
		})
	})

	Context("Delete", func() {
		It("deletes using the historic snapshot without a live IdentityProvider", func() {
			realm := createRealm(ns, committedStatus())
			svc := keycloakservice.NewMockKeycloakService(GinkgoT())
			svc.EXPECT().DeleteRealm(mock.Anything, "test-realm").Return(nil)
			calls := 0
			Expect(NewHandlerRealm(committedFactory(svc, &calls, nil), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).Delete(handlerCtx(), realm)).To(Succeed())
			Expect(calls).To(Equal(1))
			Expect(realm.Status.AdminPassword).To(Equal(literalSecret))
		})

		It("skips remote deletion without a committed snapshot", func() {
			realm := createRealm(ns, nil)
			Expect(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).Delete(handlerCtx(), realm)).To(Succeed())
		})

		It("blocks on a partial snapshot", func() {
			realm := createRealm(ns, &identityv1.RealmStatus{AdminUrl: adminURLA})
			expectBlocked(NewHandlerRealm(noFactory(), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).Delete(handlerCtx(), realm))
		})

		It("returns trust/auth errors so the finalizer is retained", func() {
			realm := createRealm(ns, committedStatus())
			factory := keycloak.ServiceFactoryFunc(func(context.Context, *identityv1.Realm) (keycloak.KeycloakService, error) {
				return nil, ctrlerrors.BlockedErrorf("CA bundle invalid")
			})
			expectBlocked(NewHandlerRealm(factory, providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).Delete(handlerCtx(), realm))
		})

		It("returns DeleteRealm errors", func() {
			realm := createRealm(ns, committedStatus())
			svc := keycloakservice.NewMockKeycloakService(GinkgoT())
			svc.EXPECT().DeleteRealm(mock.Anything, "test-realm").Return(fmt.Errorf("boom"))
			calls := 0
			Expect(NewHandlerRealm(committedFactory(svc, &calls, nil), providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle)).Delete(handlerCtx(), realm)).
				To(MatchError(ContainSubstring("failed to delete realm")))
		})
	})
})
