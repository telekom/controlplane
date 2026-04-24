// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	pcpv1 "github.com/telekom/controlplane/permission/api/pcp/v1"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	"github.com/telekom/controlplane/permission/internal/handler/permissionset"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	timeout  = 2 * time.Second
	interval = 100 * time.Millisecond
)

var _ = Describe("PermissionSet Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		permissionsetObj := &permissionv1.PermissionSet{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind PermissionSet")
			err := k8sClient.Get(ctx, typeNamespacedName, permissionsetObj)
			if err != nil && errors.IsNotFound(err) {
				resource := &permissionv1.PermissionSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: permissionv1.PermissionSetSpec{
						Permissions: []permissionv1.Permission{
							{
								Role:     "admin",
								Resource: "stargate:myapi:v1",
								Actions:  []string{"read", "write"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &permissionv1.PermissionSet{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance PermissionSet")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &PermissionSetReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}
			controllerReconciler.Controller = cc.NewController(&permissionset.PermissionSetHandler{}, k8sClient, recorder)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling with proper Zone setup", Ordered, func() {
		const (
			testEnvironment = "test-env"
			zoneName        = "test-zone"
			psName          = "test-permission-set"
		)

		var (
			testNamespace *corev1.Namespace
			zoneNs        *corev1.Namespace
			zone          *adminv1.Zone
			permissionSet *permissionv1.PermissionSet
		)

		BeforeAll(func() {
			// Create environment namespace (where Zone resource lives)
			envNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testEnvironment,
				},
			}
			Expect(k8sClient.Create(ctx, envNamespace)).To(Succeed())

			// Create test namespace for internal PermissionSet
			testNamespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			// Create zone namespace
			zoneNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "zone-",
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
			}
			Expect(k8sClient.Create(ctx, zoneNs)).To(Succeed())

			// Create Zone resource
			zone = &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      zoneName,
					Namespace: testEnvironment,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: adminv1.ZoneSpec{
					Visibility: adminv1.ZoneVisibilityWorld,
					IdentityProvider: adminv1.IdentityProviderConfig{
						Url: "https://idp.example.com",
						Admin: adminv1.IdentityProviderAdminConfig{
							UserName: "admin",
							Password: "password",
							ClientId: "client-id",
						},
					},
					Gateway: adminv1.GatewayConfig{
						Url:            "https://gateway.example.com",
						CircuitBreaker: false,
						Admin: adminv1.GatewayAdminConfig{
							ClientSecret: "secret",
						},
					},
					Redis: adminv1.RedisConfig{
						Host:      "redis.example.com",
						Port:      6379,
						Password:  "redis-password",
						EnableTLS: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, zone)).To(Succeed())

			// Update status in a separate step
			zone.Status.Namespace = zoneNs.Name
			zone.Status.Links = adminv1.Links{
				Url:       "https://gateway.example.com",
				Issuer:    "https://idp.example.com/auth/realms/default",
				LmsIssuer: "https://gateway.example.com/auth/realms/default",
			}
			zone.SetCondition(condition.NewReadyCondition("ZoneReady", "Zone is ready"))
			Expect(k8sClient.Status().Update(ctx, zone)).To(Succeed())

			// Create internal PermissionSet
			// The environment label is required by the common controller to set up the context
			permissionSet = &permissionv1.PermissionSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      psName,
					Namespace: testNamespace.Name,
					Labels: map[string]string{
						config.EnvironmentLabelKey:   testEnvironment,
						config.BuildLabelKey("zone"): zoneName,
					},
				},
				Spec: permissionv1.PermissionSetSpec{
					Permissions: []permissionv1.Permission{
						{
							Role:     "admin",
							Resource: "stargate:myapi:v1",
							Actions:  []string{"read", "write"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, permissionSet)).To(Succeed())
		})

		AfterAll(func() {
			By("Cleaning up the resources")
			// Cleanup external PermissionSets in zone namespace
			psList := &pcpv1.PermissionSetList{}
			if err := k8sClient.List(ctx, psList, client.InNamespace(zoneNs.Name)); err == nil {
				for i := range psList.Items {
					_ = client.IgnoreNotFound(k8sClient.Delete(ctx, &psList.Items[i]))
				}
			}
			if permissionSet != nil {
				_ = client.IgnoreNotFound(k8sClient.Delete(ctx, permissionSet))
			}
			if zone != nil {
				_ = client.IgnoreNotFound(k8sClient.Delete(ctx, zone))
			}
			if zoneNs != nil {
				_ = client.IgnoreNotFound(k8sClient.Delete(ctx, zoneNs))
			}
			if testNamespace != nil {
				_ = client.IgnoreNotFound(k8sClient.Delete(ctx, testNamespace))
			}
			envNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testEnvironment,
				},
			}
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, envNamespace))
		})

		It("should create external PermissionSet with correct labels and naming", func() {
			By("Reconciling the internal PermissionSet")
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &PermissionSetReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}
			controllerReconciler.Controller = cc.NewController(&permissionset.PermissionSetHandler{}, k8sClient, recorder)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      permissionSet.Name,
					Namespace: permissionSet.Namespace,
				},
			}

			// First reconciliation adds the finalizer and returns early
			_, err := controllerReconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Second reconciliation runs the handler
			_, err = controllerReconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying external PermissionSet was created in zone namespace")
			externalPS := &pcpv1.PermissionSet{}
			Eventually(func(g Gomega) {
				psList := &pcpv1.PermissionSetList{}
				err := k8sClient.List(ctx, psList, client.InNamespace(zoneNs.Name))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(psList.Items).To(HaveLen(1), "expected exactly one external PermissionSet in zone namespace")
				externalPS = &psList.Items[0]
			}, timeout, interval).Should(Succeed())

			By("Verifying external PermissionSet has namespace-prefixed name")
			Expect(externalPS.Name).To(HavePrefix(testNamespace.Name + "-"))

			By("Verifying external PermissionSet has both environment labels")
			Expect(externalPS.Labels).To(HaveKeyWithValue(config.EnvironmentLabelKey, testEnvironment))
			Expect(externalPS.Labels).To(HaveKeyWithValue("ei.telekom.de/environment", testEnvironment))

			By("Verifying external PermissionSet has owner UID label")
			Expect(externalPS.Labels).To(HaveKey(config.OwnerUidLabelKey))
			Expect(externalPS.Labels[config.OwnerUidLabelKey]).To(Equal(string(permissionSet.UID)))

			By("Verifying external PermissionSet has correct spec")
			Expect(externalPS.Spec.Permissions).To(HaveLen(1))
			Expect(externalPS.Spec.Permissions[0].Role).To(Equal("admin"))
			Expect(externalPS.Spec.Permissions[0].Resource).To(Equal("stargate:myapi:v1"))
			Expect(externalPS.Spec.Permissions[0].Actions).To(ConsistOf("read", "write"))

			By("Verifying internal PermissionSet status references external")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(permissionSet), permissionSet)
			Expect(err).NotTo(HaveOccurred())
			Expect(permissionSet.Status.PermissionSet).NotTo(BeNil())
			Expect(permissionSet.Status.PermissionSet.Name).To(Equal(externalPS.Name))
			Expect(permissionSet.Status.PermissionSet.Namespace).To(Equal(externalPS.Namespace))

			By("Verifying internal PermissionSet has Ready condition")
			readyCondition := meta.FindStatusCondition(permissionSet.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
		})
	})
})
