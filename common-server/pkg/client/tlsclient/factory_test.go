// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tlsclient

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Factory", func() {
	const (
		timeout  = 5 * time.Second
		interval = 10 * time.Millisecond
	)

	var (
		ctx     context.Context
		f       *Factory
		now     time.Time
		nowMu   sync.Mutex
		ca      *testCA
		ts      *tokenServer
		caRes   *staticCA
		spec    Spec
		profile = Profile{Name: "identity", Timeout: 2 * time.Second}
	)

	clock := func() time.Time {
		nowMu.Lock()
		defer nowMu.Unlock()
		return now
	}
	advance := func(d time.Duration) {
		nowMu.Lock()
		defer nowMu.Unlock()
		now = now.Add(d)
	}
	entryCount := func() int {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.entries)
	}
	totalRefs := func() int {
		f.mu.Lock()
		defer f.mu.Unlock()
		n := 0
		for _, e := range f.entries {
			n += e.refs
		}
		return n
	}

	BeforeEach(func() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(cancel)
		now = time.Unix(1000, 0)

		var err error
		f, err = NewFactory(ctx, []Profile{profile, {Name: "other"}}, WithClock(clock), WithIdleTTL(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(f.Close)

		ca = newTestCA("ca")
		ts = &tokenServer{}
		tokenURL := startTLSServer(ca.serverCert(""), ts).URL
		apiURL := startTLSServer(ca.serverCert(""), &apiHandler{}).URL
		caRes = &staticCA{pem: ca.pem}
		spec = Spec{
			Owner:   Owner{Kind: "identity.cp.ei.telekom.de/IdentityProvider", Namespace: "ns", Name: "idp", UID: "uid-1"},
			Profile: profile.Name,
			APIURL:  apiURL,
			Auth:    Auth{Mode: AuthPassword, TokenURL: tokenURL, ClientID: "admin-cli", Username: "u", Password: "p"},
			CA:      caRes,
		}
	})

	It("validates construction options", func() {
		_, err := NewFactory(ctx, []Profile{{Name: "a"}, {Name: "a"}})
		Expect(err).To(MatchError(ContainSubstring("duplicate")))
		_, err = NewFactory(ctx, []Profile{{}})
		Expect(err).To(HaveOccurred())
		_, err = NewFactory(ctx, nil, WithIdleTTL(0))
		Expect(err).To(HaveOccurred())
	})

	It("rejects unknown profiles, owners and specs", func() {
		s := spec
		s.Profile = "missing"
		_, err := f.Acquire(ctx, s)
		Expect(err).To(MatchError(ContainSubstring("unknown profile")))
		s = spec
		s.Owner.UID = ""
		_, err = f.Acquire(ctx, s)
		Expect(err).To(HaveOccurred())
		s = spec
		s.APIURL = ""
		_, err = f.Acquire(ctx, s)
		Expect(err).To(HaveOccurred())
	})

	It("shares one client per owner and fingerprint", func() {
		l1, err := f.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		defer l1.Release()
		l2, err := f.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		defer l2.Release()
		Expect(l1.Client()).To(BeIdenticalTo(l2.Client()))
		Expect(ts.passwordCalls.Load()).To(Equal(int32(1)))
		Expect(get(l1.Client(), spec.APIURL)).To(Equal(http.StatusOK))
	})

	It("builds concurrent acquisitions once", func() {
		var wg sync.WaitGroup
		clients := make([]*Client, 20)
		for i := range clients {
			wg.Add(1)
			go func(i int) {
				defer GinkgoRecover()
				defer wg.Done()
				l, err := f.Acquire(ctx, spec)
				Expect(err).NotTo(HaveOccurred())
				defer l.Release()
				clients[i] = l.Client()
			}(i)
		}
		wg.Wait()
		for _, c := range clients {
			Expect(c).To(BeIdenticalTo(clients[0]))
		}
		Expect(ts.passwordCalls.Load()).To(Equal(int32(1)))
		Expect(totalRefs()).To(Equal(0))
	})

	It("isolates owners, profiles, credentials and CA versions", func() {
		base, err := f.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		defer base.Release()

		variants := []func(*Spec){
			func(s *Spec) { s.Owner.UID = "uid-2" },
			func(s *Spec) { s.Owner.Namespace = "ns2" },
			func(s *Spec) { s.Owner.Kind = "gateway.cp.ei.telekom.de/Gateway" },
			func(s *Spec) { s.Profile = "other" },
			func(s *Spec) { s.Auth.Password = "rotated" },
			func(s *Spec) { s.CA = &staticCA{pem: join(ca.pem, newTestCA("rotated").pem)} },
		}
		for _, v := range variants {
			s := spec
			v(&s)
			l, err := f.Acquire(ctx, s)
			Expect(err).NotTo(HaveOccurred())
			Expect(l.Client()).NotTo(BeIdenticalTo(base.Client()))
			l.Release()
		}
		Expect(entryCount()).To(Equal(len(variants) + 1))
		Expect(ts.passwordCalls.Load()).To(Equal(int32(len(variants) + 1)))
	})

	It("keeps old and new CA versions side by side", func() {
		old, err := f.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		defer old.Release()

		caRes.set(join(ca.pem, newTestCA("next").pem), nil)
		next, err := f.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		defer next.Release()
		Expect(next.Client()).NotTo(BeIdenticalTo(old.Client()))
		Expect(get(old.Client(), spec.APIURL)).To(Equal(http.StatusOK))
		Expect(get(next.Client(), spec.APIURL)).To(Equal(http.StatusOK))
	})

	It("resolves the CA before serving a cache hit", func() {
		l, err := f.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		l.Release()

		caRes.set(nil, errors.New("invalid bundle"))
		_, err = f.Acquire(ctx, spec)
		Expect(err).To(MatchError(ContainSubstring("invalid bundle")))
		Expect(caRes.calls.Load()).To(Equal(int32(2)))
		Expect(totalRefs()).To(Equal(0))
	})

	It("removes failed builds so they can be retried", func() {
		ts.passwordStatus.Store(http.StatusUnauthorized)
		_, err := f.Acquire(ctx, spec)
		Expect(err).To(MatchError(ContainSubstring("retrieving token")))
		Expect(entryCount()).To(Equal(0))
		failedCalls := ts.passwordCalls.Load()

		ts.passwordStatus.Store(0)
		l, err := f.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		l.Release()
		Expect(ts.passwordCalls.Load()).To(Equal(failedCalls + 1))
	})

	It("honors waiter cancellation without leaking references", func() {
		blocked := &tokenServer{block: make(chan struct{})}
		spec.Auth.TokenURL = startTLSServer(ca.serverCert(""), blocked).URL

		waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		_, err := f.Acquire(waitCtx, spec)
		Expect(err).To(MatchError(context.DeadlineExceeded))
		Expect(totalRefs()).To(Equal(0))

		// the build continues with the factory lifetime
		close(blocked.block)
		Eventually(func(g Gomega) {
			f.mu.Lock()
			defer f.mu.Unlock()
			g.Expect(f.entries).To(HaveLen(1))
			for _, e := range f.entries {
				g.Expect(e.isReady()).To(BeTrue())
				g.Expect(e.err).NotTo(HaveOccurred())
			}
		}).WithTimeout(timeout).WithPolling(interval).Should(Succeed())

		l, err := f.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		l.Release()
		Expect(blocked.passwordCalls.Load()).To(Equal(int32(1)))
	})

	It("evicts only idle entries without borrowers", func() {
		held, err := f.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		s := spec
		s.Owner.UID = "uid-2"
		idle, err := f.Acquire(ctx, s)
		Expect(err).NotTo(HaveOccurred())
		idle.Release()
		idle.Release() // idempotent
		Expect(totalRefs()).To(Equal(1))

		advance(30 * time.Second)
		f.evictIdle()
		Expect(entryCount()).To(Equal(2))

		advance(time.Minute)
		f.evictIdle()
		Expect(entryCount()).To(Equal(1))
		Expect(idle.Client().closed.Load()).To(BeTrue())
		Expect(held.Client().closed.Load()).To(BeFalse())

		held.Release()
		f.evictIdle()
		Expect(entryCount()).To(Equal(0))
	})

	It("runs the eviction loop and closes on shutdown", func() {
		f2, err := NewFactory(ctx, []Profile{profile}, WithClock(clock), WithIdleTTL(time.Minute), WithCleanupInterval(interval))
		Expect(err).NotTo(HaveOccurred())
		Expect(f2.NeedLeaderElection()).To(BeFalse())
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error)
		go func() { done <- f2.Start(runCtx) }()

		l, err := f2.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		l.Release()
		advance(2 * time.Minute)
		Eventually(func() int {
			f2.mu.Lock()
			defer f2.mu.Unlock()
			return len(f2.entries)
		}).WithTimeout(timeout).WithPolling(interval).Should(Equal(0))

		held, err := f2.Acquire(ctx, spec)
		Expect(err).NotTo(HaveOccurred())
		stop()
		Eventually(done).WithTimeout(timeout).WithPolling(interval).Should(Receive(BeNil()))
		Expect(held.Client().closed.Load()).To(BeTrue())
		held.Release()

		_, err = f2.Acquire(ctx, spec)
		Expect(err).To(MatchError(ErrFactoryClosed))
	})

	It("stops the loop when the factory lifetime ends", func() {
		lifeCtx, cancel := context.WithCancel(ctx)
		f2, err := NewFactory(lifeCtx, nil)
		Expect(err).NotTo(HaveOccurred())
		done := make(chan error)
		go func() { done <- f2.Start(ctx) }()
		cancel()
		Eventually(done).WithTimeout(timeout).WithPolling(interval).Should(Receive(BeNil()))
	})

	It("fails builds completing after shutdown and unblocks waiters", func() {
		blocked := &tokenServer{block: make(chan struct{})}
		spec.Auth.TokenURL = startTLSServer(ca.serverCert(""), blocked).URL
		DeferCleanup(func() { close(blocked.block) })

		errs := make(chan error, 1)
		go func() {
			_, err := f.Acquire(ctx, spec)
			errs <- err
		}()
		Eventually(entryCount).WithTimeout(timeout).WithPolling(interval).Should(Equal(1))
		f.Close()
		Eventually(errs).WithTimeout(timeout).WithPolling(interval).Should(Receive(HaveOccurred()))
		Expect(entryCount()).To(Equal(0))
	})
})
