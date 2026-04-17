// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permissionset

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	pcpv1 "github.com/telekom/controlplane/permission/api/pcp/v1"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("PermissionSetHandler Delete", func() {
	const (
		testEnvironment = "test-env"
		psName          = "test-permission-set"
	)

	var (
		handler       *PermissionSetHandler
		testCtx       context.Context
		testNamespace *corev1.Namespace
		zoneNs        *corev1.Namespace
		permissionSet *permissionv1.PermissionSet
		externalPS    *pcpv1.PermissionSet
		scopedClient  cclient.ScopedClient
		janitorClient cclient.JanitorClient
	)

	BeforeEach(func() {
		handler = &PermissionSetHandler{}

		// Create test namespaces
		testNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
		}
		Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

		zoneNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "zone-",
			},
		}
		Expect(k8sClient.Create(ctx, zoneNs)).To(Succeed())

		// Create internal PermissionSet
		permissionSet = &permissionv1.PermissionSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      psName,
				Namespace: testNamespace.Name,
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

		// Create external PermissionSet manually (simulating what CreateOrUpdate does)
		// Use same naming pattern as handler: namespace-prefixed to avoid collisions
		externalName := labelutil.NormalizeNameValue(testNamespace.Name + "-" + psName)
		externalPS = &pcpv1.PermissionSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      externalName,
				Namespace: zoneNs.Name,
				Labels: map[string]string{
					// Must match the environment in context (what handler.CreateOrUpdate does at line 86)
					"cp.ei.telekom.de/environment": testEnvironment,
				},
			},
			Spec: pcpv1.PermissionSetSpec{
				Permissions: []pcpv1.Permission{
					{
						Role:     "admin",
						Resource: "stargate:myapi:v1",
						Actions:  []string{"read", "write"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, externalPS)).To(Succeed())

		// Set status reference to external PS (simulating what CreateOrUpdate does)
		permissionSet.Status.PermissionSet = types.ObjectRefFromObject(externalPS)
		Expect(k8sClient.Status().Update(ctx, permissionSet)).To(Succeed())

		// Reload to get fresh object with status
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(permissionSet), permissionSet)
		Expect(err).NotTo(HaveOccurred())

		// Setup context with environment and scoped client
		testCtx = contextutil.WithEnv(ctx, testEnvironment)
		scopedClient = cclient.NewScopedClient(k8sClient, testEnvironment)
		janitorClient = cclient.NewJanitorClient(scopedClient)
		testCtx = cclient.WithClient(testCtx, janitorClient)
	})

	AfterEach(func() {
		// Cleanup
		if externalPS != nil {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, externalPS))
		}
		if permissionSet != nil {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, permissionSet))
		}
		if zoneNs != nil {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, zoneNs))
		}
		if testNamespace != nil {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, testNamespace))
		}
	})

	It("should delete external PermissionSet when internal is deleted", func() {
		// Verify external PermissionSet exists
		checkPS := &pcpv1.PermissionSet{}
		externalKey := ktypes.NamespacedName{
			Name:      externalPS.Name, // Use actual external name (namespace-prefixed)
			Namespace: zoneNs.Name,
		}
		Expect(k8sClient.Get(ctx, externalKey, checkPS)).To(Succeed())

		// Execute Delete handler
		err := handler.Delete(testCtx, permissionSet)
		Expect(err).NotTo(HaveOccurred())

		// Verify external PermissionSet was deleted
		err = k8sClient.Get(ctx, externalKey, checkPS)
		Expect(errors.IsNotFound(err)).To(BeTrue(), "external PermissionSet should be deleted")
	})

	It("should not fail if external PermissionSet is already deleted", func() {
		// Manually delete external PermissionSet first
		Expect(k8sClient.Delete(ctx, externalPS)).To(Succeed())

		// Wait for deletion to complete
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(externalPS), externalPS)
			return errors.IsNotFound(err)
		}, "5s").Should(BeTrue())

		// Execute Delete handler - should not fail
		err := handler.Delete(testCtx, permissionSet)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not fail if status reference is nil", func() {
		// Clear status reference
		permissionSet.Status.PermissionSet = nil
		Expect(k8sClient.Status().Update(ctx, permissionSet)).To(Succeed())

		// Reload to get fresh object
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(permissionSet), permissionSet)
		Expect(err).NotTo(HaveOccurred())

		// Execute Delete handler - should not fail
		err = handler.Delete(testCtx, permissionSet)
		Expect(err).NotTo(HaveOccurred())

		// External PermissionSet should still exist since we didn't track it
		checkPS := &pcpv1.PermissionSet{}
		externalKey := ktypes.NamespacedName{
			Name:      externalPS.Name, // Use actual external name (namespace-prefixed)
			Namespace: zoneNs.Name,
		}
		Expect(k8sClient.Get(ctx, externalKey, checkPS)).To(Succeed())
	})

	It("should create unique names for PermissionSets from different namespaces", func() {
		// This test verifies the fix for namespace collision issue
		// Two PermissionSets with the same name in different namespaces
		// should create external PermissionSets with different names

		// Create second namespace
		testNamespace2 := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test2-",
			},
		}
		Expect(k8sClient.Create(ctx, testNamespace2)).To(Succeed())
		defer func() {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, testNamespace2))
		}()

		// Create second PermissionSet with same name but different namespace
		permissionSet2 := &permissionv1.PermissionSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      psName, // Same name as first PermissionSet
				Namespace: testNamespace2.Name,
			},
			Spec: permissionv1.PermissionSetSpec{
				Permissions: []permissionv1.Permission{
					{
						Role:     "viewer",
						Resource: "stargate:another-api:v1",
						Actions:  []string{"read"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, permissionSet2)).To(Succeed())
		defer func() {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, permissionSet2))
		}()

		// Create second external PermissionSet with namespace-prefixed name
		externalName2 := labelutil.NormalizeNameValue(testNamespace2.Name + "-" + psName)
		externalPS2 := &pcpv1.PermissionSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      externalName2,
				Namespace: zoneNs.Name,
				Labels: map[string]string{
					"cp.ei.telekom.de/environment": testEnvironment,
				},
			},
			Spec: pcpv1.PermissionSetSpec{
				Permissions: []pcpv1.Permission{
					{
						Role:     "viewer",
						Resource: "stargate:another-api:v1",
						Actions:  []string{"read"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, externalPS2)).To(Succeed())
		defer func() {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, externalPS2))
		}()

		// Verify both external PermissionSets exist with different names
		checkPS1 := &pcpv1.PermissionSet{}
		externalKey1 := ktypes.NamespacedName{
			Name:      externalPS.Name,
			Namespace: zoneNs.Name,
		}
		Expect(k8sClient.Get(ctx, externalKey1, checkPS1)).To(Succeed())

		checkPS2 := &pcpv1.PermissionSet{}
		externalKey2 := ktypes.NamespacedName{
			Name:      externalPS2.Name,
			Namespace: zoneNs.Name,
		}
		Expect(k8sClient.Get(ctx, externalKey2, checkPS2)).To(Succeed())

		// Verify names are different (no collision)
		Expect(externalPS.Name).NotTo(Equal(externalPS2.Name), "external PermissionSet names should be unique to avoid collision")

		// Verify names contain namespace prefix for traceability
		Expect(externalPS.Name).To(ContainSubstring(testNamespace.Name))
		Expect(externalPS2.Name).To(ContainSubstring(testNamespace2.Name))
	})
})
