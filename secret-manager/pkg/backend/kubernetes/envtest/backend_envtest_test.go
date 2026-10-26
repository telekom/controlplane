// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envtest_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Kubernetes Backend Get against envtest", Ordered, func() {
	const (
		namespace = "poc--my-team"
		name      = "my-app"
	)

	var k8sBackend backend.Backend[kubernetes.Id, backend.DefaultSecret[kubernetes.Id]]

	currentResourceVersion := func() string {
		obj := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, kubernetes.New("poc", "my-team", name, "clientSecret", "").ObjectKey(), obj)).To(Succeed())
		return obj.GetResourceVersion()
	}

	BeforeAll(func() {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Type:       corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"clientSecret":    []byte("synthetic-a"),
				"externalSecrets": []byte(`{"foo":"synthetic-b"}`),
			},
		})).To(Succeed())
		k8sBackend = kubernetes.NewBackend(k8sClient)
	})

	It("returns the object resourceVersion for top-level secrets without writing", func() {
		rvBefore := currentResourceVersion()
		secretId := kubernetes.New("poc", "my-team", name, "clientSecret", "stale")

		secret, err := k8sBackend.Get(ctx, secretId)
		Expect(err).NotTo(HaveOccurred())
		Expect(secret.Value()).To(Equal("synthetic-a"))
		Expect(secret.Id().String()).To(Equal("poc:my-team:my-app:clientSecret:" + rvBefore))
		Expect(secretId.String()).To(Equal("poc:my-team:my-app:clientSecret:stale"))
		Consistently(currentResourceVersion).WithTimeout(500 * time.Millisecond).WithPolling(100 * time.Millisecond).Should(Equal(rvBefore))
	})

	It("returns the value checksum for nested secrets without writing", func() {
		rvBefore := currentResourceVersion()
		secretId := kubernetes.New("poc", "my-team", name, "externalSecrets/foo", "")

		secret, err := k8sBackend.Get(ctx, secretId)
		Expect(err).NotTo(HaveOccurred())
		Expect(secret.Value()).To(Equal("synthetic-b"))
		Expect(secret.Id().String()).To(Equal("poc:my-team:my-app:externalSecrets/foo:" + backend.MakeChecksum("synthetic-b")))
		Consistently(currentResourceVersion).WithTimeout(500 * time.Millisecond).WithPolling(100 * time.Millisecond).Should(Equal(rvBefore))
	})

	It("round-trips the returned id with the default (non-strict) settings", func() {
		first, err := k8sBackend.Get(ctx, kubernetes.New("poc", "my-team", name, "externalSecrets/foo", ""))
		Expect(err).NotTo(HaveOccurred())

		secondId, err := k8sBackend.ParseSecretId(first.Id().String())
		Expect(err).NotTo(HaveOccurred())
		second, err := k8sBackend.Get(ctx, secondId)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Value()).To(Equal(first.Value()))
		Expect(second.Id().String()).To(Equal(first.Id().String()))
	})
})
