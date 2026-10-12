// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package providerca

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// countingReader counts reads delegated to the envtest API server.
type countingReader struct {
	client.Reader
	reads  atomic.Int32
	before func(ctx context.Context)
}

func (r *countingReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	r.reads.Add(1)
	if r.before != nil {
		r.before(ctx)
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

var _ = Describe("Cache", func() {
	var (
		ctx    context.Context
		ns     string
		reader *countingReader
		cache  *Cache[string]
		now    time.Time
	)

	labeled := func(name string, data map[string]string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{types.ProviderCALabelKey: types.ProviderCALabelValue}},
			Data:       data,
		}
	}
	put := func(name string, data map[string]string) *corev1.ConfigMap {
		GinkgoHelper()
		cm := labeled(name, data)
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		return cm
	}
	remove := func(name string) {
		GinkgoHelper()
		Expect(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}})).To(Succeed())
	}
	ref := func(name, key string) *types.CABundleConfigMapRef {
		return &types.CABundleConfigMapRef{Name: name, Key: key}
	}
	advance := func(d time.Duration) { now = now.Add(d) }
	resolve := func(r *types.CABundleConfigMapRef) (*Resolved[string], error) {
		return cache.Resolve(ctx, ns, r)
	}
	expectKind := func(err error, kind FailureKind) {
		GinkgoHelper()
		re := AsResolveError(err)
		Expect(re).To(HaveOccurred(), "error %v", err)
		Expect(re.Kind).To(Equal(kind))
	}

	BeforeEach(func() {
		ctx = context.Background()
		n := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "providerca-cache-"}}
		Expect(k8sClient.Create(ctx, n)).To(Succeed())
		ns = n.Name
		reader = &countingReader{Reader: k8sClient}
		cache = NewCache[string](reader, parseTest)
		now = time.Unix(1_700_000_000, 0)
		cache.now = func() time.Time { return now }
	})

	It("panics on missing dependencies", func() {
		Expect(func() { NewCache[string](nil, parseTest) }).To(PanicWith("providerca.NewCache: reader is nil"))
		Expect(func() { NewCache[string](k8sClient, nil) }).To(PanicWith("providerca.NewCache: parse is nil"))
	})

	It("returns nil for system trust without reading", func() {
		unreachable, err := client.New(&rest.Config{Host: "http://127.0.0.1:1", Timeout: time.Second},
			client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		c := NewCache[string](unreachable, parseTest)
		r, err := c.Resolve(ctx, ns, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).To(BeNil())
	})

	It("reuses a success until the fixed TTL expires", func() {
		cm := put("ca", map[string]string{"ca.crt": "one"})
		r, err := resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Bundle).To(Equal("parsed:one"))
		Expect(r.ConfigMapUID).To(Equal(cm.UID))
		Expect(r.Ref).To(Equal(*ref("ca", "ca.crt")))
		cm.Data["ca.crt"] = "two"
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())
		advance(59 * time.Second)
		r, err = resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Bundle).To(Equal("parsed:one"))
		Expect(reader.reads.Load()).To(Equal(int32(1)))
		advance(time.Second)
		r, err = resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Bundle).To(Equal("parsed:two"))
		Expect(reader.reads.Load()).To(Equal(int32(2)))
	})

	It("observes a recreated ConfigMap by its new UID after expiry", func() {
		old := put("ca", map[string]string{"ca.crt": "one"})
		_, err := resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		remove("ca")
		recreated := put("ca", map[string]string{"ca.crt": "one"})
		advance(time.Minute)
		r, err := resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.ConfigMapUID).To(Equal(recreated.UID))
		Expect(r.ConfigMapUID).NotTo(Equal(old.UID))
	})

	It("isolates entries by namespace, name and key", func() {
		put("ca", map[string]string{"a": "one", "b": "two"})
		put("other", map[string]string{"a": "three"})
		ra, err := resolve(ref("ca", "a"))
		Expect(err).NotTo(HaveOccurred())
		rb, err := resolve(ref("ca", "b"))
		Expect(err).NotTo(HaveOccurred())
		ro, err := resolve(ref("other", "a"))
		Expect(err).NotTo(HaveOccurred())
		Expect([]string{ra.Bundle, rb.Bundle, ro.Bundle}).To(Equal([]string{"parsed:one", "parsed:two", "parsed:three"}))
		_, err = cache.Resolve(ctx, "default", ref("ca", "a"))
		expectKind(err, FailureNotFound)
	})

	It("does not fall back to an expired success after a failed refresh", func() {
		put("ca", map[string]string{"ca.crt": "one"})
		_, err := resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		remove("ca")
		advance(time.Minute)
		r, err := resolve(ref("ca", "ca.crt"))
		expectKind(err, FailureNotFound)
		Expect(r).To(BeNil())
	})

	It("caches failures for the retry interval", func() {
		_, err := resolve(ref("ca", "ca.crt"))
		expectKind(err, FailureNotFound)
		put("ca", map[string]string{"ca.crt": "one"})
		advance(4 * time.Second)
		_, err = resolve(ref("ca", "ca.crt"))
		expectKind(err, FailureNotFound)
		Expect(reader.reads.Load()).To(Equal(int32(1)))
		advance(time.Second)
		r, err := resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Bundle).To(Equal("parsed:one"))
	})

	It("passes through validation failures with their kind", func() {
		put("nokey", map[string]string{"other": "one"})
		put("bad", map[string]string{"ca.crt": "garbage"})
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "unlabeled", Namespace: ns},
			Data:       map[string]string{"ca.crt": "one"},
		})).To(Succeed())
		_, err := resolve(ref("nokey", "ca.crt"))
		expectKind(err, FailureKeyMissing)
		_, err = resolve(ref("bad", "ca.crt"))
		expectKind(err, FailureInvalidBundle)
		_, err = resolve(ref("unlabeled", "ca.crt"))
		expectKind(err, FailureLabelMissing)
		_, err = resolve(ref("Invalid_Name", "ca.crt"))
		expectKind(err, FailureInvalidReference)
	})

	It("does not cache cancellation", func() {
		put("ca", map[string]string{"ca.crt": "one"})
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		_, err := cache.Resolve(canceled, ns, ref("ca", "ca.crt"))
		Expect(err).To(MatchError(context.Canceled))
		Expect(reader.reads.Load()).To(BeZero())
		reading, cancelReading := context.WithCancel(ctx)
		reader.before = func(context.Context) { cancelReading() }
		_, err = cache.Resolve(reading, ns, ref("ca", "ca.crt"))
		Expect(err).To(MatchError(context.Canceled))
		reader.before = nil
		r, err := resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Bundle).To(Equal("parsed:one"))
		Expect(reader.reads.Load()).To(Equal(int32(2)))
	})

	It("does not reuse an expired success when its refresh is canceled", func() {
		put("ca", map[string]string{"ca.crt": "one"})
		_, err := resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		advance(time.Minute)
		remove("ca")
		put("ca", map[string]string{"ca.crt": "two"})
		refreshing, cancel := context.WithCancel(ctx)
		reader.before = func(context.Context) { cancel() }
		r, err := cache.Resolve(refreshing, ns, ref("ca", "ca.crt"))
		Expect(err).To(MatchError(context.Canceled))
		Expect(r).To(BeNil())
		reader.before = nil
		r, err = resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Bundle).To(Equal("parsed:two"))
		Expect(reader.reads.Load()).To(Equal(int32(3)))
	})

	It("bounds retained entries and re-reads evicted ones", func() {
		cache.maxEntries = 2
		for _, name := range []string{"a", "b", "c"} {
			put(name, map[string]string{"ca.crt": name})
			_, err := resolve(ref(name, "ca.crt"))
			Expect(err).NotTo(HaveOccurred())
		}
		remove("a")
		remove("c")
		_, err := resolve(ref("a", "ca.crt"))
		expectKind(err, FailureNotFound)
		r, err := resolve(ref("c", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Bundle).To(Equal("parsed:c"))
	})

	It("isolates returned metadata from the cached entry", func() {
		cm := put("ca", map[string]string{"ca.crt": "one"})
		r, err := resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		r.Ref.Name = "changed"
		r.ConfigMapUID = "changed"
		again, err := resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(again.Ref.Name).To(Equal("ca"))
		Expect(again.ConfigMapUID).To(Equal(cm.UID))
	})

	It("does not retain the caller's reference", func() {
		put("ca", map[string]string{"ca.crt": "one"})
		r := ref("ca", "ca.crt")
		_, err := resolve(r)
		Expect(err).NotTo(HaveOccurred())
		r.Name = "changed"
		again, err := resolve(ref("ca", "ca.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(again.Ref.Name).To(Equal("ca"))
	})

	It("serializes concurrent resolutions of the same key", func() {
		put("ca", map[string]string{"ca.crt": "one"})
		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				r, err := resolve(ref("ca", "ca.crt"))
				Expect(err).NotTo(HaveOccurred())
				Expect(r.Bundle).To(Equal("parsed:one"))
			}()
		}
		wg.Wait()
		Expect(reader.reads.Load()).To(Equal(int32(1)))
	})
})
