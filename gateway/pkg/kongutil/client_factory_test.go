// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil

import (
	"context"
	"errors"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common-server/pkg/client/tlsclient"
	"github.com/telekom/controlplane/common/pkg/cabundle"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// failingReader fails every read with a transient error.
type failingReader struct{ client.Reader }

func (failingReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("connection refused")
}

func doGet(c *tlsclient.Client) (int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.APIURL()+"/routes", http.NoBody)
	Expect(err).NotTo(HaveOccurred())
	resp, err := c.Doer().Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func isBlocked(err error) bool {
	var be ctrlerrors.BlockedError
	return errors.As(err, &be) && be.IsBlocked()
}

var _ = Describe("ClientFactory", func() {
	var (
		ctx       context.Context
		namespace string
		factory   *ClientFactory
		tokens    *tokenServer
		api       *apiHandler
		tokenSrv  string
		apiSrv    string
		tokenCA   *testCA
		apiCA     *testCA
	)

	newGateway := func(name string, uid k8stypes.UID, ref *types.ConfigMapKeyRef) *gatewayv1.Gateway {
		gw := &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: uid},
			Spec: gatewayv1.GatewaySpec{
				Admin: gatewayv1.AdminConfig{
					Url:          apiSrv,
					ClientId:     "client",
					ClientSecret: "secret",
					IssuerUrl:    tokenSrv + "/realms/test",
				},
			},
		}
		if ref != nil {
			gw.Spec.TLS = &types.TLSConfig{ConfigMapRef: *ref}
		}
		return gw
	}

	writeBundle := func(name, data string) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
		cm.Data = map[string]string{"ca.pem": data}
		if err == nil {
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())
			return
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
	}

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "kongutil-" + rand.String(6)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())

		tokenCA = newTestCA("token-ca")
		apiCA = newTestCA("api-ca")
		tokens = &tokenServer{}
		api = &apiHandler{}
		tokenSrv = startTLSServer(tokenCA, tokens).URL
		apiSrv = startTLSServer(apiCA, api).URL

		var err error
		factory, err = NewClientFactory(context.Background(), k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(factory.NeedLeaderElection()).To(BeFalse())
		startCtx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(done)
			Expect(factory.Start(startCtx)).To(Succeed())
		}()
		DeferCleanup(func() {
			cancel()
			Eventually(done).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())
		})
	})

	It("uses the union CA bundle for token and API requests", func() {
		union, err := cabundle.Canonicalize(tokenCA.pem, apiCA.pem)
		Expect(err).NotTo(HaveOccurred())
		writeBundle("bundle", string(union))

		gw := newGateway("gw", "uid-1", &types.ConfigMapKeyRef{Name: "bundle", Key: "ca.pem"})
		lease, err := factory.acquire(ctx, gw)
		Expect(err).NotTo(HaveOccurred())
		defer lease.Release()

		status, err := doGet(lease.Client())
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(http.StatusOK))
		Expect(api.last()).To(Equal("Bearer kong-token"))
		Expect(tokens.lastPath()).To(Equal("/realms/test" + tokenUrlPath))
	})

	It("fails TLS verification if only one of the CAs is trusted", func() {
		writeBundle("api-only", string(apiCA.pem))

		gw := newGateway("gw", "uid-1", &types.ConfigMapKeyRef{Name: "api-only", Key: "ca.pem"})
		lease, err := factory.acquire(ctx, gw)
		Expect(err).NotTo(HaveOccurred())
		defer lease.Release()

		_, err = doGet(lease.Client())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("certificate"))
		Expect(tokens.calls.Load()).To(BeZero())
	})

	It("uses system roots only without a TLS reference", func() {
		lease, err := factory.acquire(ctx, newGateway("gw", "uid-1", nil))
		Expect(err).NotTo(HaveOccurred())
		defer lease.Release()

		_, err = doGet(lease.Client())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("certificate"))
	})

	It("enforces token and API verification even with the legacy bypass environment variable", func() {
		GinkgoT().Setenv("SKIP_TLS_VERIFY", "true")
		gw := newGateway("gw", "uid-1", nil)
		lease, err := factory.acquire(ctx, gw)
		Expect(err).NotTo(HaveOccurred())
		defer lease.Release()
		_, err = doGet(lease.Client())
		Expect(err).To(MatchError(ContainSubstring("certificate")))
		Expect(tokens.calls.Load()).To(BeZero())

		writeBundle("token-only", string(tokenCA.pem))
		gw.Spec.TLS = &types.TLSConfig{ConfigMapRef: types.ConfigMapKeyRef{Name: "token-only", Key: "ca.pem"}}
		trustedToken, err := factory.acquire(ctx, gw)
		Expect(err).NotTo(HaveOccurred())
		defer trustedToken.Release()
		_, err = doGet(trustedToken.Client())
		Expect(err).To(MatchError(ContainSubstring("certificate")))
		Expect(tokens.calls.Load()).To(BeNumerically(">", 0))
		Expect(api.last()).To(BeEmpty())
	})

	It("re-validates the CA bundle before serving a cached client", func() {
		union, err := cabundle.Canonicalize(tokenCA.pem, apiCA.pem)
		Expect(err).NotTo(HaveOccurred())
		writeBundle("bundle", string(union))
		gw := newGateway("gw", "uid-1", &types.ConfigMapKeyRef{Name: "bundle", Key: "ca.pem"})

		kc, release, err := factory.ClientFor(ctx, gw)
		Expect(err).NotTo(HaveOccurred())
		Expect(kc).NotTo(BeNil())
		release()
		release()

		By("making the source invalid")
		writeBundle("bundle", "not a certificate")
		_, _, err = factory.ClientFor(ctx, gw)
		Expect(err).To(HaveOccurred())
		Expect(isBlocked(err)).To(BeTrue(), "invalid source must block: %v", err)

		By("deleting the source")
		Expect(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "bundle", Namespace: namespace}})).To(Succeed())
		_, _, err = factory.ClientFor(ctx, gw)
		Expect(isBlocked(err)).To(BeTrue(), "missing source must block: %v", err)
	})

	It("blocks if the referenced key is missing", func() {
		writeBundle("bundle", string(apiCA.pem))
		gw := newGateway("gw", "uid-1", &types.ConfigMapKeyRef{Name: "bundle", Key: "other"})
		_, _, err := factory.ClientFor(ctx, gw)
		Expect(isBlocked(err)).To(BeTrue())
	})

	It("does not block on transient read errors", func() {
		f, err := NewClientFactory(context.Background(), failingReader{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(f.clients.Close)

		gw := newGateway("gw", "uid-1", &types.ConfigMapKeyRef{Name: "bundle", Key: "ca.pem"})
		_, _, err = f.ClientFor(ctx, gw)
		Expect(err).To(HaveOccurred())
		Expect(isBlocked(err)).To(BeFalse())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("fails if a CA reference is configured but no reader is available", func() {
		f, err := NewClientFactory(context.Background(), nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(f.clients.Close)

		gw := newGateway("gw", "uid-1", &types.ConfigMapKeyRef{Name: "bundle", Key: "ca.pem"})
		_, _, err = f.ClientFor(ctx, gw)
		Expect(err).To(MatchError(ContainSubstring("no resolver is configured")))
	})

	It("isolates clients per Gateway and shares them for the same Gateway", func() {
		gwA := newGateway("gw-a", "uid-a", nil)
		gwB := newGateway("gw-b", "uid-b", nil)

		leaseA1, err := factory.acquire(ctx, gwA)
		Expect(err).NotTo(HaveOccurred())
		defer leaseA1.Release()
		leaseA2, err := factory.acquire(ctx, gwA)
		Expect(err).NotTo(HaveOccurred())
		defer leaseA2.Release()
		leaseB, err := factory.acquire(ctx, gwB)
		Expect(err).NotTo(HaveOccurred())
		defer leaseB.Release()

		Expect(leaseA1.Client()).To(BeIdenticalTo(leaseA2.Client()))
		Expect(leaseB.Client()).NotTo(BeIdenticalTo(leaseA1.Client()))

		By("changing the credentials")
		gwA.Spec.Admin.ClientSecret = "rotated"
		leaseA3, err := factory.acquire(ctx, gwA)
		Expect(err).NotTo(HaveOccurred())
		defer leaseA3.Release()
		Expect(leaseA3.Client()).NotTo(BeIdenticalTo(leaseA1.Client()))
	})

	It("rejects a nil Gateway and a missing factory", func() {
		_, _, err := factory.ClientFor(ctx, nil)
		Expect(err).To(MatchError(ContainSubstring("gateway is required")))

		_, _, err = GetClientFor(ctx, nil, newGateway("gw", "uid-1", nil))
		Expect(err).To(MatchError(ContainSubstring("not configured")))
	})

	It("delegates GetClientFor to the factory", func() {
		kc, release, err := GetClientFor(ctx, factory, newGateway("gw", "uid-1", nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(kc).NotTo(BeNil())
		release()
	})
})

var _ = Describe("NewStandaloneClient", func() {
	var (
		tokens *tokenServer
		api    *apiHandler
		cfg    GatewayAdminConfig
	)

	BeforeEach(func() {
		ca := newTestCA("untrusted")
		tokens = &tokenServer{}
		api = &apiHandler{}
		tokenSrv := startTLSServer(ca, tokens).URL
		apiSrv := startTLSServer(ca, api).URL
		cfg = NewGatewayConfig(apiSrv, "client", "secret", tokenSrv+"/realms/test")
	})

	It("verifies TLS by default", func() {
		c, closeFn, err := NewStandaloneClient(context.Background(), cfg, false)
		Expect(err).NotTo(HaveOccurred())
		defer closeFn()

		_, err = c.ListRouteWithResponse(context.Background(), &kong.ListRouteParams{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("certificate"))
		Expect(tokens.calls.Load()).To(BeZero())
	})

	It("skips TLS verification for token and API on explicit opt-in", func() {
		c, closeFn, err := NewStandaloneClient(context.Background(), cfg, true)
		Expect(err).NotTo(HaveOccurred())
		defer closeFn()

		_, err = c.ListRouteWithResponse(context.Background(), &kong.ListRouteParams{})
		Expect(err).NotTo(HaveOccurred())
		Expect(tokens.calls.Load()).To(BeNumerically(">", 0))
		Expect(tokens.lastPath()).To(Equal("/realms/test" + tokenUrlPath))
		Expect(api.last()).To(Equal("Bearer kong-token"))
	})

	It("rejects invalid URLs", func() {
		_, _, err := NewStandaloneClient(context.Background(), NewGatewayConfig("not-a-url", "c", "s", "also-not"), false)
		Expect(err).To(HaveOccurred())
	})
})
