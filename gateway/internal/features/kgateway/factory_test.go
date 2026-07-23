// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kgateway

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var _ = Describe("GetClientFor", func() {
	var ctx context.Context
	BeforeEach(func() { ctx = context.Background() })

	local := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().WithObjects(objs...).Build()
	}

	It("returns a client for the local cluster when RemoteCluster is nil", func() {
		gw := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "ns"}}
		c, err := GetClientFor(ctx, local(), gw)
		Expect(err).NotTo(HaveOccurred())
		Expect(c).NotTo(BeNil())
	})

	It("fails when the referenced kubeconfig secret is missing", func() {
		gw := &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "ns"},
			Spec: gatewayv1.GatewaySpec{
				RemoteCluster: &gatewayv1.RemoteClusterConfig{
					SecretRef: ctypes.ObjectRef{Name: "kc", Namespace: "ns"},
				},
			},
		}
		_, err := GetClientFor(ctx, local(), gw)
		Expect(err).To(HaveOccurred())
	})

	It("fails when the secret lacks the kubeconfig key", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "kc", Namespace: "ns"},
			Data:       map[string][]byte{"wrong": []byte("x")},
		}
		gw := &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "ns"},
			Spec: gatewayv1.GatewaySpec{
				RemoteCluster: &gatewayv1.RemoteClusterConfig{
					SecretRef: ctypes.ObjectRef{Name: "kc", Namespace: "ns"},
					Key:       "kubeconfig",
				},
			},
		}
		_, err := GetClientFor(ctx, local(secret), gw)
		Expect(err).To(MatchError(ContainSubstring("no key")))
	})
})
