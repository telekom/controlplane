// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package providerca

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	ktypes "k8s.io/apimachinery/pkg/types"

	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// testBundle is a parsed bundle identified by its digest.
type testBundle struct{ digest string }

func (b *testBundle) Digest() string { return b.digest }

type resolvedBundle = Resolved[*testBundle]

type fakeCA struct {
	mu        sync.Mutex
	byName    map[string]*resolvedBundle
	errs      map[string]error
	resolves  int
	onResolve func()
}

func (f *fakeCA) Resolve(_ context.Context, _ string, ref *ctypes.CABundleConfigMapRef) (*resolvedBundle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolves++
	if hook := f.onResolve; hook != nil {
		f.mu.Unlock()
		hook()
		f.mu.Lock()
	}
	if ref == nil {
		return nil, nil
	}
	if err := f.errs[ref.Name]; err != nil {
		return nil, err
	}
	r, ok := f.byName[ref.Name]
	if !ok {
		return nil, &ResolveError{Kind: FailureNotFound, Err: errors.New("not found")}
	}
	cp := *r
	return &cp, nil
}

func (f *fakeCA) set(name string, uid ktypes.UID, b *testBundle) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byName[name] = &resolvedBundle{Ref: ctypes.CABundleConfigMapRef{Name: name, Key: "ca.crt"}, ConfigMapUID: uid, Bundle: b}
}

func (f *fakeCA) fail(name string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[name] = err
}

type fakeClient struct {
	id       int
	bundle   *testBundle
	lifetime context.Context
	closed   atomic.Int32
}

var _ = Describe("ClientCache", func() {
	var (
		ctx              context.Context
		ca               *fakeCA
		cache            *ClientCache[*fakeClient, *testBundle]
		builds           atomic.Int32
		build            Builder[*fakeClient, *testBundle]
		owner            ktypes.NamespacedName
		bundleA, bundleB *testBundle
		built            []*fakeClient
		builtMu          sync.Mutex
	)
	ref := func(name string) *ctypes.CABundleConfigMapRef {
		return &ctypes.CABundleConfigMapRef{Name: name, Key: "ca.crt"}
	}
	request := func(parts []string, r *ctypes.CABundleConfigMapRef) ClientRequest {
		return ClientRequest{Owner: owner, UID: "uid-1", Parts: parts, Ref: r}
	}
	acquire := func(r ClientRequest) (*fakeClient, error) { return cache.Acquire(ctx, r, build) }
	mustAcquire := func(r ClientRequest) *fakeClient {
		GinkgoHelper()
		c, err := acquire(r)
		Expect(err).NotTo(HaveOccurred())
		return c
	}
	BeforeEach(func() {
		ctx = context.Background()
		bundleA = &testBundle{digest: "digest-a"}
		bundleB = &testBundle{digest: "digest-b"}
		ca = &fakeCA{byName: map[string]*resolvedBundle{}, errs: map[string]error{}}
		ca.set("a", "uid-a", bundleA)
		ca.set("b", "uid-b", bundleB)
		cache = NewClientCache[*fakeClient, *testBundle](ca)
		DeferCleanup(cache.Close)
		owner = ktypes.NamespacedName{Namespace: "ns", Name: "owner"}
		builds.Store(0)
		built = nil
		build = func(_, lifetime context.Context, bundle *testBundle) (Built[*fakeClient], error) {
			c := &fakeClient{id: int(builds.Add(1)), bundle: bundle, lifetime: lifetime}
			builtMu.Lock()
			built = append(built, c)
			builtMu.Unlock()
			return Built[*fakeClient]{Value: c, CloseIdle: func() { c.closed.Add(1) }}, nil
		}
	})
	It("panics without a CA resolver", func() {
		Expect(func() { NewClientCache[int, *testBundle](nil) }).To(PanicWith("providerca.NewClientCache: CA resolver is nil"))
	})
	It("rejects a nil builder", func() {
		_, err := cache.Acquire(ctx, request(nil, nil), nil)
		Expect(err).To(MatchError(ContainSubstring("builder is nil")))
	})
	It("reuses a client while its inputs are unchanged", func() {
		c1 := mustAcquire(request([]string{"url", "secret"}, ref("a")))
		c2 := mustAcquire(request([]string{"url", "secret"}, ref("a")))
		Expect(c2).To(BeIdenticalTo(c1))
		Expect(builds.Load()).To(Equal(int32(1)))
		Expect(ca.resolves).To(Equal(2))
	})
	It("passes the resolved bundle, or nil for system trust, without resolving nil", func() {
		Expect(mustAcquire(request(nil, ref("a"))).bundle).To(BeIdenticalTo(bundleA))
		Expect(mustAcquire(request(nil, nil)).bundle).To(BeNil())
		Expect(ca.resolves).To(Equal(1))
	})
	It("does not confuse part boundaries", func() {
		mustAcquire(request([]string{"ab", "c"}, nil))
		mustAcquire(request([]string{"a", "bc"}, nil))
		Expect(builds.Load()).To(Equal(int32(2)))
	})
	It("reuses a client for an equivalent canonical CA digest", func() {
		c1 := mustAcquire(request(nil, ref("a")))
		ca.set("a", "uid-a", &testBundle{digest: bundleA.digest})
		Expect(mustAcquire(request(nil, ref("a")))).To(BeIdenticalTo(c1))
	})
	DescribeTable("replaces the client when an identity input changes", func(change func(*ClientRequest)) {
		r := request([]string{"url", "secret"}, ref("a"))
		old := mustAcquire(r)
		change(&r)
		Expect(mustAcquire(r)).NotTo(BeIdenticalTo(old))
		Expect(old.closed.Load()).To(Equal(int32(1)))
	},
		Entry("endpoint", func(r *ClientRequest) { r.Parts[0] = "other-url" }),
		Entry("credential", func(r *ClientRequest) { r.Parts[1] = "rotated" }),
		Entry("owner UID", func(r *ClientRequest) { r.UID = "uid-2" }),
		Entry("CA reference", func(r *ClientRequest) { r.Ref = ref("b") }),
		Entry("system trust", func(r *ClientRequest) { r.Ref = nil }),
		Entry("ConfigMap UID", func(r *ClientRequest) { ca.set("a", "uid-recreated", bundleA) }),
		Entry("CA digest", func(r *ClientRequest) { ca.set("a", "uid-a", bundleB) }),
	)
	It("returns the CA failure without building or returning the old client", func() {
		mustAcquire(request(nil, ref("a")))
		cause := &ResolveError{Kind: FailureInvalidBundle, Err: errors.New("bad")}
		ca.fail("a", cause)
		c, err := acquire(request(nil, ref("a")))
		Expect(c).To(BeNil())
		Expect(AsResolveError(err)).To(BeIdenticalTo(cause))
		Expect(builds.Load()).To(Equal(int32(1)))
	})
	It("fails closed when an explicit reference resolves to system trust", func() {
		c := NewClientCache[*fakeClient, *testBundle](resolverFunc(func() (*resolvedBundle, error) { return nil, nil }))
		defer c.Close()
		_, err := c.Acquire(ctx, request(nil, ref("a")), build)
		Expect(err).To(MatchError(ContainSubstring("resolved to system trust")))
		Expect(builds.Load()).To(BeZero())
	})
	It("returns a build failure instead of the old client and recovers later", func() {
		old := mustAcquire(request([]string{"v1"}, nil))
		failing := func(context.Context, context.Context, *testBundle) (Built[*fakeClient], error) {
			return Built[*fakeClient]{}, errors.New("boom")
		}
		c, err := cache.Acquire(ctx, request([]string{"v2"}, nil), failing)
		Expect(err).To(MatchError("boom"))
		Expect(c).To(BeNil())
		Expect(mustAcquire(request([]string{"v2"}, nil))).NotTo(BeIdenticalTo(old))
	})
	It("evicts the least recently used owner and closes its idle connections", func() {
		cache.maxEntries = 2
		clients := map[string]*fakeClient{}
		for _, name := range []string{"o1", "o2"} {
			owner = ktypes.NamespacedName{Namespace: "ns", Name: name}
			clients[name] = mustAcquire(request(nil, nil))
		}
		owner = ktypes.NamespacedName{Namespace: "ns", Name: "o1"}
		mustAcquire(request(nil, nil))
		owner = ktypes.NamespacedName{Namespace: "ns", Name: "o3"}
		mustAcquire(request(nil, nil))
		Expect(clients["o2"].closed.Load()).To(Equal(int32(1)))
		Expect(clients["o1"].closed.Load()).To(BeZero())
		owner = ktypes.NamespacedName{Namespace: "ns", Name: "o2"}
		Expect(mustAcquire(request(nil, nil))).NotTo(BeIdenticalTo(clients["o2"]))
	})
	It("gives builders a lifetime context independent of the call", func() {
		call, cancel := context.WithCancel(ctx)
		c, err := cache.Acquire(call, request(nil, nil), build)
		Expect(err).NotTo(HaveOccurred())
		cancel()
		Expect(c.lifetime.Err()).NotTo(HaveOccurred())
		cache.Close()
		Expect(c.lifetime.Err()).To(MatchError(context.Canceled))
	})
	It("closes retained clients once and fails later acquisitions", func() {
		c := mustAcquire(request(nil, nil))
		cache.Close()
		cache.Close()
		Expect(c.closed.Load()).To(Equal(int32(1)))
		_, err := acquire(request(nil, nil))
		Expect(err).To(MatchError(ErrClientCacheClosed))
	})
	It("closes when the manager stops", func() {
		Expect(cache.NeedLeaderElection()).To(BeFalse())
		run, stop := context.WithCancel(ctx)
		done := make(chan error)
		go func() { done <- cache.Start(run) }()
		stop()
		Eventually(done).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(Receive(BeNil()))
		_, err := acquire(request(nil, nil))
		Expect(err).To(MatchError(ErrClientCacheClosed))
	})
	It("does not publish a client built while closing and disposes of it", func() {
		var candidate *fakeClient
		closing := func(call, lifetime context.Context, b *testBundle) (Built[*fakeClient], error) {
			out, err := build(call, lifetime, b)
			candidate = out.Value
			cache.Close()
			return out, err
		}
		_, err := cache.Acquire(ctx, request(nil, nil), closing)
		Expect(err).To(MatchError(ErrClientCacheClosed))
		Expect(candidate.closed.Load()).To(Equal(int32(1)))
	})
	It("does not return a client built for a canceled call", func() {
		call, cancel := context.WithCancel(ctx)
		canceling := func(call, lifetime context.Context, b *testBundle) (Built[*fakeClient], error) {
			cancel()
			return build(call, lifetime, b)
		}
		_, err := cache.Acquire(call, request(nil, nil), canceling)
		Expect(err).To(MatchError(context.Canceled))
		Expect(built[0].closed.Load()).To(Equal(int32(1)))
		mustAcquire(request(nil, nil))
		Expect(builds.Load()).To(Equal(int32(2)))
	})
	Context("concurrency", func() {
		blockingBuild := func(entered chan<- struct{}, release <-chan struct{}) Builder[*fakeClient, *testBundle] {
			return func(call, lifetime context.Context, b *testBundle) (Built[*fakeClient], error) {
				entered <- struct{}{}
				<-release
				return build(call, lifetime, b)
			}
		}
		It("serializes same-owner acquisitions without duplicate builds", func() {
			var wg sync.WaitGroup
			results := make([]*fakeClient, 8)
			for i := range results {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					results[i] = mustAcquire(request([]string{"same"}, nil))
				}()
			}
			wg.Wait()
			Expect(builds.Load()).To(Equal(int32(1)))
			for _, r := range results {
				Expect(r).To(BeIdenticalTo(results[0]))
			}
		})
		It("lets other stripes progress during a blocked build", func() {
			entered, release := make(chan struct{}, 1), make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer GinkgoRecover()
				defer close(done)
				_, err := cache.Acquire(ctx, request(nil, nil), blockingBuild(entered, release))
				Expect(err).NotTo(HaveOccurred())
			}()
			Eventually(entered).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(Receive())
			stripeOf := func(o ktypes.NamespacedName) uint32 { return stripeIndex(o.Namespace, o.Name) % clientCacheStripes }
			blocked := stripeOf(owner)
			for i := range 100 {
				other := ktypes.NamespacedName{Namespace: "ns", Name: "other-" + string(rune('a'+i%26)) + string(rune('a'+i/26))}
				if stripeOf(other) == blocked {
					continue
				}
				_, err := cache.Acquire(ctx, ClientRequest{Owner: other}, build)
				Expect(err).NotTo(HaveOccurred())
				break
			}
			close(release)
			Eventually(done).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())
		})
		It("does not let a canceled waiter build or receive a client", func() {
			entered, release := make(chan struct{}, 1), make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer GinkgoRecover()
				defer close(done)
				_, err := cache.Acquire(ctx, request(nil, nil), blockingBuild(entered, release))
				Expect(err).NotTo(HaveOccurred())
			}()
			Eventually(entered).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(Receive())
			wait, cancel := context.WithCancel(ctx)
			resolved := make(chan struct{})
			ca.onResolve = func() { close(resolved) }
			waiterErr := make(chan error, 1)
			go func() { _, err := cache.Acquire(wait, request(nil, ref("a")), build); waiterErr <- err }()
			Eventually(resolved).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())
			cancel()
			close(release)
			Eventually(waiterErr).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(Receive(MatchError(context.Canceled)))
			Eventually(done).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())
			Expect(builds.Load()).To(Equal(int32(1)))
		})
	})
})

type resolverFunc func() (*resolvedBundle, error)

func (f resolverFunc) Resolve(context.Context, string, *ctypes.CABundleConfigMapRef) (*resolvedBundle, error) {
	return f()
}

var _ = Describe("ClientCache retirement", func() {
	It("keeps a retired client usable and expires its later idle connection", func() {
		var closedConns atomic.Int32
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
			if state == http.StateClosed {
				closedConns.Add(1)
			}
		}
		srv.Start()
		defer srv.Close()
		ca := &fakeCA{byName: map[string]*resolvedBundle{}, errs: map[string]error{}}
		cache := NewClientCache[*http.Client, *testBundle](ca)
		defer cache.Close()
		build := func(context.Context, context.Context, *testBundle) (Built[*http.Client], error) {
			tr := &http.Transport{IdleConnTimeout: 200 * time.Millisecond}
			return Built[*http.Client]{Value: &http.Client{Transport: tr}, CloseIdle: tr.CloseIdleConnections}, nil
		}
		get := func(c *http.Client) {
			GinkgoHelper()
			resp, err := c.Get(srv.URL)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Body.Close()).To(Succeed())
		}
		owner := ktypes.NamespacedName{Namespace: "ns", Name: "owner"}
		borrowed, err := cache.Acquire(context.Background(), ClientRequest{Owner: owner, Parts: []string{"v1"}}, build)
		Expect(err).NotTo(HaveOccurred())
		get(borrowed)
		_, err = cache.Acquire(context.Background(), ClientRequest{Owner: owner, Parts: []string{"v2"}}, build)
		Expect(err).NotTo(HaveOccurred())
		Eventually(closedConns.Load).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(Equal(int32(1)))
		get(borrowed)
		Eventually(closedConns.Load).WithTimeout(5 * time.Second).WithPolling(20 * time.Millisecond).Should(Equal(int32(2)))
	})
})
