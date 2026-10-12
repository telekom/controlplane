// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/test/testutil"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Effective identity operator RBAC", func() {
	It("grants clusterwide ConfigMap reads independent of the provider-ca label", func() {
		providerNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "rbac-provider-identity"}}
		operatorNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "rbac-operator-identity"}}
		Expect(k8sClient.Create(ctx, providerNs)).To(Succeed())
		Expect(k8sClient.Create(ctx, operatorNs)).To(Succeed())
		for _, cm := range []*corev1.ConfigMap{
			{ObjectMeta: metav1.ObjectMeta{
				Name: "labeled", Namespace: providerNs.Name,
				Labels: map[string]string{ctypes.ProviderCALabelKey: ctypes.ProviderCALabelValue},
			}},
			{ObjectMeta: metav1.ObjectMeta{Name: "unlabeled", Namespace: providerNs.Name}},
			{ObjectMeta: metav1.ObjectMeta{Name: "lock", Namespace: operatorNs.Name}},
		} {
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		}

		rbacDir := filepath.Join("..", "..", "config", "rbac")
		c, err := testutil.RestrictedIdentity(ctx, k8sClient, cfg, "identity-operator", operatorNs.Name,
			filepath.Join(rbacDir, "role.yaml"), filepath.Join(rbacDir, "leader_election_role.yaml"))
		Expect(err).NotTo(HaveOccurred())

		testutil.ExpectProviderCAConfigMapAccess(ctx, c, providerNs.Name, "labeled", "unlabeled", false)
		By("the leader-election role grants no ConfigMap mutation in the operator namespace")
		testutil.ExpectProviderCAConfigMapAccess(ctx, c, operatorNs.Name, "lock", "lock", false)
	})
})
