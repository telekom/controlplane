// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil

import (
	"context"
	"errors"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	ptls "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/providerca"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These specs map CA read failures through Validate with a reader that cannot read ConfigMaps.
var _ = Describe("CA read failure mapping", Ordered, func() {
	var testEnv *envtest.Environment

	BeforeAll(func() {
		testEnv = &envtest.Environment{}
		_, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(testEnv.Stop()).To(Succeed()) })
	})

	expectTrust := func(err error, reason string) *ctrlerrors.CtrlError {
		GinkgoHelper()
		te := AsTrustError(err)
		Expect(te).To(HaveOccurred(), "error %v", err)
		Expect(te.Reason).To(Equal(reason))
		var ce *ctrlerrors.CtrlError
		Expect(errors.As(err, &ce)).To(BeTrue())
		return ce
	}

	validate := func(reader client.Reader, tls *ctypes.ProviderTLS) error {
		ca := providerca.NewCache(reader, ptls.ParseProviderCABundle)
		gw := newGW("gw", "https://127.0.0.1:1/", "https://127.0.0.1:1/realms/x")
		gw.Spec.Admin.TLS = tls
		return NewFactory(ca, NewClientCache(ca), nil).Validate(context.Background(), gw)
	}

	It("maps a forbidden read to a blocked ConfigMapForbidden", func() {
		restricted := rest.CopyConfig(testEnv.Config)
		restricted.Impersonate = rest.ImpersonationConfig{UserName: "resolveca-no-rbac"}
		reader, err := client.New(restricted, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		err = validate(reader, ref("api-ca"))
		Expect(expectTrust(err, "ConfigMapForbidden").IsBlocked()).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("CA ConfigMap " + ns + "/api-ca is forbidden"))
	})

	It("maps a transient read failure to a retryable ConfigMapReadFailed", func() {
		reader, err := client.New(&rest.Config{Host: "http://127.0.0.1:1", Timeout: time.Second},
			client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		err = validate(reader, ref("zone-ca"))
		Expect(expectTrust(err, "ConfigMapReadFailed").IsRetryable()).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("reading CA ConfigMap " + ns + "/zone-ca: "))
	})
})
