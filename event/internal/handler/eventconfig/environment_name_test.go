// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventconfig

import (
	"context"
	"path/filepath"

	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Horizon environment name resolution", func() {
	It("uses a nonempty overwrite verbatim", func() {
		Expect(effectiveEnvironmentName(context.Background(), " Legacy Horizon ")).To(Equal(" Legacy Horizon "))
	})

	It("falls back to the Control Plane environment for an empty overwrite", func() {
		ctx := contextutil.WithEnv(context.Background(), "staging")
		Expect(effectiveEnvironmentName(ctx, "")).To(Equal("staging"))
	})

	It("panics when fallback context is missing", func() {
		Expect(func() { effectiveEnvironmentName(context.Background(), "") }).To(PanicWith("env not found in context"))
	})

	It("keeps Control Plane scope canonical while replacing and clearing the Horizon overwrite", func() {
		testEnv := &envtest.Environment{
			CRDDirectoryPaths: []string{
				filepath.Join("..", "..", "..", "config", "crd", "bases"),
				filepath.Join("..", "..", "..", "..", "pubsub", "config", "crd", "bases"),
			},
			ErrorIfCRDPathMissing: true,
		}
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(testEnv.Stop()).To(Succeed()) })

		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(eventv1.AddToScheme(scheme)).To(Succeed())
		Expect(pubsubv1.AddToScheme(scheme)).To(Succeed())
		rawClient, err := client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(rawClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "staging"}})).To(Succeed())

		scopedClient := cclient.NewScopedClient(rawClient, "staging")
		janitorClient := cclient.NewJanitorClient(scopedClient)
		ctx := contextutil.WithEnv(context.Background(), "staging")
		ctx = cclient.WithClient(ctx, janitorClient)
		obj := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "events", Namespace: "staging"},
			Spec: eventv1.EventConfigSpec{
				Zone:                     ctypes.ObjectRef{Name: "zone-a", Namespace: "staging"},
				OverwriteEnvironmentName: "default",
				Local: &eventv1.LocalBackend{
					Admin:              eventv1.AdminConfig{Url: "https://horizon.example.com"},
					ServerSendEventUrl: "https://horizon.example.com/sse",
					PublishEventUrl:    "https://horizon.example.com/publish",
				},
			},
		}
		_, err = janitorClient.CreateOrUpdate(ctx, obj, func() error { return nil })
		Expect(err).NotTo(HaveOccurred())
		identityClient := &identityv1.Client{Spec: identityv1.ClientSpec{ClientId: "client-id", ClientSecret: "client-secret"}}

		reconcileEventStore := func(overwrite, expectedEnvironment string) {
			_, updateErr := janitorClient.CreateOrUpdate(ctx, obj, func() error {
				obj.Spec.OverwriteEnvironmentName = overwrite
				return nil
			})
			Expect(updateErr).NotTo(HaveOccurred())

			eventStore, createErr := (&EventConfigHandler{}).createEventStore(ctx, obj, identityClient, "https://issuer.example.com/token")
			Expect(createErr).NotTo(HaveOccurred())
			Expect(eventStore.Name).To(Equal("events"))
			Expect(eventStore.Namespace).To(Equal("staging"))
			Expect(eventStore.Labels).To(HaveKeyWithValue(config.EnvironmentLabelKey, "staging"))
			Expect(eventStore.Spec.OverwriteEnvironmentName).To(Equal(expectedEnvironment))
			Expect(eventStore.Spec.Url).To(Equal("https://horizon.example.com"))
			Expect(eventStore.OwnerReferences).To(ContainElement(And(
				WithTransform(func(ref metav1.OwnerReference) string { return ref.Name }, Equal("events")),
				WithTransform(func(ref metav1.OwnerReference) string { return ref.Kind }, Equal("EventConfig")),
			)))

			fromScope := &pubsubv1.EventStore{}
			Expect(scopedClient.Get(ctx, types.NamespacedName{Name: "events"}, fromScope)).To(Succeed())
			Expect(fromScope.Namespace).To(Equal("staging"))
			Expect(fromScope.Spec.OverwriteEnvironmentName).To(Equal(expectedEnvironment))
		}

		reconcileEventStore("default", "default")
		reconcileEventStore("legacy", "legacy")
		reconcileEventStore("", "staging")

		persistedConfig := &eventv1.EventConfig{}
		Expect(scopedClient.Get(ctx, types.NamespacedName{Name: "events"}, persistedConfig)).To(Succeed())
		Expect(persistedConfig.Labels).To(HaveKeyWithValue(config.EnvironmentLabelKey, "staging"))
		Expect(persistedConfig.Spec.Zone).To(Equal(ctypes.ObjectRef{Name: "zone-a", Namespace: "staging"}))
		Expect(persistedConfig.Spec.OverwriteEnvironmentName).To(BeEmpty())
	})

	DescribeTable("uses the proxy EventConfig value independently of its target",
		func(proxyOverwrite, expected string) {
			ctx := contextutil.WithEnv(context.Background(), "staging")
			fakeClient := fakeclient.NewMockJanitorClient(GinkgoT())
			ctx = cclient.WithClient(ctx, fakeClient)

			proxy := &eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "proxy-config", Namespace: "default", UID: "proxy-uid"},
				Spec: eventv1.EventConfigSpec{
					Zone:                     ctypes.ObjectRef{Name: "proxy-zone", Namespace: "default"},
					OverwriteEnvironmentName: proxyOverwrite,
					Proxy:                    &eventv1.ProxyBackend{TargetZone: ctypes.ObjectRef{Name: "target-zone", Namespace: "default"}},
				},
			}
			target := eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "target-config", Namespace: "default"},
				Spec: eventv1.EventConfigSpec{
					Zone:                     ctypes.ObjectRef{Name: "target-zone", Namespace: "default"},
					OverwriteEnvironmentName: "target-horizon",
					Local: &eventv1.LocalBackend{Admin: eventv1.AdminConfig{
						Url: "https://target.example.com",
						Client: eventv1.ClientConfig{
							Realm:        ctypes.ObjectRef{Name: "target-realm", Namespace: "default"},
							ClientId:     "target-client",
							ClientSecret: "target-secret",
						},
					}},
				},
			}
			target.SetCondition(condition.NewReadyCondition("Ready", "ready"))
			targetZone := &adminv1.Zone{ObjectMeta: metav1.ObjectMeta{Name: "target-zone", Namespace: "default"}}
			targetZone.SetCondition(condition.NewReadyCondition("Ready", "ready"))
			realm := &identityv1.Realm{Status: identityv1.RealmStatus{IssuerUrl: "https://issuer.example.com"}}

			scheme := runtime.NewScheme()
			Expect(eventv1.AddToScheme(scheme)).To(Succeed())
			Expect(pubsubv1.AddToScheme(scheme)).To(Succeed())
			fakeClient.EXPECT().Scheme().Return(scheme)
			fakeClient.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					list.(*eventv1.EventConfigList).Items = []eventv1.EventConfig{target}
				}).Return(nil)
			fakeClient.EXPECT().Get(ctx, types.NamespacedName{Name: "target-zone", Namespace: "default"}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *targetZone
				}).Return(nil)
			fakeClient.EXPECT().Get(ctx, types.NamespacedName{Name: "target-realm", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*identityv1.Realm) = *realm
				}).Return(nil)
			fakeClient.EXPECT().CreateOrUpdate(ctx, mock.AnythingOfType("*v1.EventStore"), mock.Anything).
				Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
					Expect(mutate()).To(Succeed())
				}).
				Return(controllerutil.OperationResultCreated, nil)

			eventStore, err := (&EventConfigHandler{}).createProxyEventStore(ctx, proxy)

			Expect(err).NotTo(HaveOccurred())
			Expect(eventStore.Spec).To(Equal(pubsubv1.EventStoreSpec{
				OverwriteEnvironmentName: expected,
				Url:                      "https://target.example.com",
				TokenUrl:                 "https://issuer.example.com/protocol/openid-connect/token",
				ClientId:                 "target-client",
				ClientSecret:             "target-secret",
			}))
			Expect(eventStore.Name).To(Equal("proxy-config"))
			Expect(eventStore.Namespace).To(Equal("default"))
		},
		Entry("when the proxy has its own overwrite", "proxy-horizon", "proxy-horizon"),
		Entry("when only the target has an overwrite", "", "staging"),
	)
})
