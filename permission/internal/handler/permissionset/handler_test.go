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
		externalPS = &pcpv1.PermissionSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      psName,
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
			Name:      psName,
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
			Name:      psName,
			Namespace: zoneNs.Name,
		}
		Expect(k8sClient.Get(ctx, externalKey, checkPS)).To(Succeed())
	})
})
