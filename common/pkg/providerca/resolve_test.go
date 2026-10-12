// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package providerca

import (
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var errGarbage = errors.New("not a bundle")

func parseTest(data []byte) (string, error) {
	if string(data) == "garbage" {
		return "", errGarbage
	}
	return "parsed:" + string(data), nil
}

var _ = Describe("Resolve", func() {
	var (
		ctx context.Context
		ns  string
	)

	put := func(cm *corev1.ConfigMap) {
		GinkgoHelper()
		cm.Namespace = ns
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
	}
	labeled := func(name string, data map[string]string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{types.ProviderCALabelKey: types.ProviderCALabelValue}},
			Data:       data,
		}
	}
	ref := func(name, key string) *types.CABundleConfigMapRef {
		return &types.CABundleConfigMapRef{Name: name, Key: key}
	}
	expectKind := func(err error, kind FailureKind) *ResolveError {
		GinkgoHelper()
		re := AsResolveError(err)
		Expect(re).To(HaveOccurred(), "error %v", err)
		Expect(re.Kind).To(Equal(kind))
		return re
	}

	BeforeEach(func() {
		ctx = context.Background()
		n := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "providerca-"}}
		Expect(k8sClient.Create(ctx, n)).To(Succeed())
		ns = n.Name
	})

	It("returns nil for system trust without reading", func() {
		unreachable, err := client.New(&rest.Config{Host: "http://127.0.0.1:1", Timeout: time.Second},
			client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		r, err := Resolve(ctx, unreachable, ns, nil, parseTest)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).To(BeNil())
	})

	It("resolves a labeled source with its reference, UID and parsed bundle", func() {
		cm := labeled("ca", map[string]string{"ca.crt": "pem"})
		put(cm)
		r, err := Resolve(ctx, k8sClient, ns, ref("ca", "ca.crt"), parseTest)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Ref).To(Equal(types.CABundleConfigMapRef{Name: "ca", Key: "ca.crt"}))
		Expect(r.ConfigMapUID).To(Equal(cm.UID))
		Expect(r.Bundle).To(Equal("parsed:pem"))
	})

	It("rejects an invalid reference before reading", func() {
		_, err := Resolve(ctx, k8sClient, ns, ref("Bad_Name", "k"), parseTest)
		re := expectKind(err, FailureInvalidReference)
		Expect(re.Error()).To(ContainSubstring("invalid caBundleConfigMapRef name"))
	})

	It("classifies a missing ConfigMap", func() {
		_, err := Resolve(ctx, k8sClient, ns, ref("missing", "ca.crt"), parseTest)
		re := expectKind(err, FailureNotFound)
		Expect(apierrors.IsNotFound(re.Err)).To(BeTrue())
	})

	It("classifies a forbidden read", func() {
		restricted := rest.CopyConfig(cfg)
		restricted.Impersonate = rest.ImpersonationConfig{UserName: "no-rbac"}
		c, err := client.New(restricted, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		_, err = Resolve(ctx, c, ns, ref("ca", "ca.crt"), parseTest)
		re := expectKind(err, FailureForbidden)
		Expect(apierrors.IsForbidden(re.Err)).To(BeTrue())
	})

	It("classifies other read failures as transient", func() {
		unreachable, err := client.New(&rest.Config{Host: "http://127.0.0.1:1", Timeout: time.Second},
			client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		_, err = Resolve(ctx, unreachable, ns, ref("ca", "ca.crt"), parseTest)
		expectKind(err, FailureReadFailed)
	})

	It("requires the exact provider-ca label", func() {
		cm := labeled("ca", map[string]string{"ca.crt": "pem"})
		cm.Labels[types.ProviderCALabelKey] = "yes"
		put(cm)
		_, err := Resolve(ctx, k8sClient, ns, ref("ca", "ca.crt"), parseTest)
		re := expectKind(err, FailureLabelMissing)
		Expect(re.Error()).To(Equal("lacks label " + types.ProviderCALabelKey + "=" + types.ProviderCALabelValue))
	})

	It("rejects partial copy provenance", func() {
		cm := labeled("ca", map[string]string{"ca.crt": "pem"})
		cm.Annotations = map[string]string{AnnotationZoneUID: "zuid"}
		put(cm)
		_, err := Resolve(ctx, k8sClient, ns, ref("ca", "ca.crt"), parseTest)
		re := expectKind(err, FailureInvalidMetadata)
		Expect(re.Error()).To(ContainSubstring("origin metadata"))
	})

	It("accepts a copy with complete provenance", func() {
		o := ProviderCAOrigin{ZoneUID: "zuid", ZoneNamespace: "prod", ZoneName: "zone", ConfigMap: "src", Key: "ca.crt"}
		cm := labeled(o.CopyName(), map[string]string{ProviderCACopyKey: "pem"})
		cm.Annotations = o.Annotations()
		cm.Labels[config.EnvironmentLabelKey] = "env"
		cm.Labels[config.BuildLabelKey("zone")] = "zone"
		put(cm)
		r, err := Resolve(ctx, k8sClient, ns, ref(o.CopyName(), ProviderCACopyKey), parseTest)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Bundle).To(Equal("parsed:pem"))
	})

	It("requires the referenced key", func() {
		put(labeled("ca", map[string]string{"other": "pem"}))
		_, err := Resolve(ctx, k8sClient, ns, ref("ca", "ca.crt"), parseTest)
		re := expectKind(err, FailureKeyMissing)
		Expect(re.Error()).To(Equal(`has no key "ca.crt"`))
	})

	It("reports parse failures", func() {
		put(labeled("ca", map[string]string{"ca.crt": "garbage"}))
		_, err := Resolve(ctx, k8sClient, ns, ref("ca", "ca.crt"), parseTest)
		re := expectKind(err, FailureInvalidBundle)
		Expect(errors.Is(err, errGarbage)).To(BeTrue())
		Expect(re.Err).To(MatchError(errGarbage))
	})

	It("reads authoritatively and observes updates immediately", func() {
		cm := labeled("ca", map[string]string{"ca.crt": "one"})
		put(cm)
		r, err := Resolve(ctx, k8sClient, ns, ref("ca", "ca.crt"), parseTest)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Bundle).To(Equal("parsed:one"))
		cm.Data["ca.crt"] = "two"
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())
		r, err = Resolve(ctx, k8sClient, ns, ref("ca", "ca.crt"), parseTest)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Bundle).To(Equal("parsed:two"))
	})

	It("returns nil from AsResolveError for unrelated errors", func() {
		Expect(AsResolveError(errors.New("x"))).NotTo(HaveOccurred())
	})
})
