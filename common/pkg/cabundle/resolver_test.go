// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cabundle_test

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/telekom/controlplane/common/pkg/cabundle"
	"github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Resolver", Ordered, func() {
	ctx := context.Background()
	var caA, caB []byte

	createNS := func(name string) {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
	}
	createCM := func(ns, name string, data map[string]string, binary map[string][]byte) {
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
			Data:       data,
			BinaryData: binary,
		})).To(Succeed())
	}

	BeforeAll(func() {
		caA, caB = newCA("a"), newCA("b")
		createNS("ns-a")
		createNS("ns-b")
		createCM("ns-a", "custom-ca", map[string]string{"ca.pem": string(caA), "junk.pem": "junk"}, map[string][]byte{"bin.pem": caA})
		createCM("ns-a", "other-ca", map[string]string{"ca.pem": string(caB)}, nil)
		createCM("ns-b", "only-b", map[string]string{"ca.pem": string(caB)}, nil)
	})

	It("resolves a single reference", func() {
		out, err := cabundle.NewResolver(k8sClient, "ns-a", types.ConfigMapKeyRef{Name: "custom-ca", Key: "ca.pem"}).Resolve(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(caA))
	})

	It("resolves the union of multiple references", func() {
		out, err := cabundle.ResolveRefs(ctx, k8sClient, "ns-a",
			types.ConfigMapKeyRef{Name: "custom-ca", Key: "ca.pem"},
			types.ConfigMapKeyRef{Name: "other-ca", Key: "ca.pem"},
			types.ConfigMapKeyRef{Name: "custom-ca", Key: "ca.pem"},
		)
		Expect(err).NotTo(HaveOccurred())
		expected, err := cabundle.Canonicalize(caA, caB)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(expected))
	})

	It("returns nil without references", func() {
		out, err := (&cabundle.Resolver{Reader: k8sClient, Namespace: "ns-a"}).Resolve(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil())
	})

	It("fails without reader", func() {
		_, err := (&cabundle.Resolver{}).Resolve(ctx)
		Expect(err).To(HaveOccurred())
	})

	It("isolates namespaces", func() {
		_, err := cabundle.NewResolver(k8sClient, "ns-a", types.ConfigMapKeyRef{Name: "only-b", Key: "ca.pem"}).Resolve(ctx)
		Expect(err).To(MatchError(cabundle.ErrSourceNotFound))
		var sErr *cabundle.SourceError
		Expect(errors.As(err, &sErr)).To(BeTrue())
		Expect(sErr.Namespace).To(Equal("ns-a"))
		Expect(sErr.Name).To(Equal("only-b"))
	})

	It("reports a missing key", func() {
		_, err := cabundle.ResolveRefs(ctx, k8sClient, "ns-a", types.ConfigMapKeyRef{Name: "custom-ca", Key: "missing"})
		Expect(err).To(MatchError(cabundle.ErrSourceNotFound))
	})

	It("ignores binaryData", func() {
		_, err := cabundle.ResolveRefs(ctx, k8sClient, "ns-a", types.ConfigMapKeyRef{Name: "custom-ca", Key: "bin.pem"})
		Expect(err).To(MatchError(cabundle.ErrSourceNotFound))
	})

	It("rejects invalid content", func() {
		_, err := cabundle.ResolveRefs(ctx, k8sClient, "ns-a",
			types.ConfigMapKeyRef{Name: "other-ca", Key: "ca.pem"},
			types.ConfigMapKeyRef{Name: "custom-ca", Key: "junk.pem"})
		Expect(err).To(MatchError(cabundle.ErrInvalidBundle))
		var sErr *cabundle.SourceError
		Expect(errors.As(err, &sErr)).To(BeTrue())
		Expect(sErr.Key).To(Equal("junk.pem"))
	})

	It("rejects an incomplete reference", func() {
		_, err := cabundle.ResolveRefs(ctx, k8sClient, "ns-a", types.ConfigMapKeyRef{Name: "custom-ca"})
		Expect(err).To(MatchError(cabundle.ErrInvalidBundle))
	})

	It("reports forbidden access", func() {
		user, err := testEnv.AddUser(envtest.User{Name: "unprivileged"}, nil)
		Expect(err).NotTo(HaveOccurred())
		limited, err := client.New(user.Config(), client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())

		_, err = cabundle.ResolveRefs(ctx, limited, "ns-a", types.ConfigMapKeyRef{Name: "custom-ca", Key: "ca.pem"})
		Expect(err).To(MatchError(cabundle.ErrSourceForbidden))
	})

	It("passes transient errors through", func() {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		_, err := cabundle.ResolveRefs(canceled, k8sClient, "ns-a", types.ConfigMapKeyRef{Name: "custom-ca", Key: "ca.pem"})
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, cabundle.ErrSourceNotFound)).To(BeFalse())
		Expect(errors.Is(err, cabundle.ErrSourceForbidden)).To(BeFalse())
		Expect(errors.Is(err, cabundle.ErrInvalidBundle)).To(BeFalse())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})
})
