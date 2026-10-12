// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type recordingRT struct {
	next http.RoundTripper
	mu   sync.Mutex
	urls []string
	// expireWatches makes that many configmap watch requests fail with 410 Gone, which forces
	// the reflector to restart and relist.
	expireWatches int
}

func (r *recordingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, "/configmaps") {
		r.mu.Lock()
		r.urls = append(r.urls, req.URL.String())
		expire := r.expireWatches > 0 && req.URL.Query().Get("watch") == "true"
		if expire {
			r.expireWatches--
		}
		r.mu.Unlock()
		if expire {
			body := `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Expired","code":410,"message":"too old resource version"}`
			return &http.Response{
				StatusCode: http.StatusGone, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(body)), Request: req,
			}, nil
		}
	}
	return r.next.RoundTrip(req)
}

func (r *recordingRT) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.urls...)
}

var _ = Describe("WithProviderCAConfigMapCache", func() {
	It("preserves other per-GVK selectors and replaces only the ConfigMap entry", func() {
		other := labels.SelectorFromSet(labels.Set{"domain": "admin"})
		opts := ProviderCACacheOptions(map[client.Object]cache.ByObject{
			&corev1.Secret{}:    {Label: other},
			&corev1.ConfigMap{}: {Label: other},
		})
		Expect(opts.DefaultLabelSelector).To(BeNil())
		Expect(opts.ByObject).To(HaveLen(2))
		for obj, e := range opts.ByObject {
			switch obj.(type) {
			case *corev1.Secret:
				Expect(e.Label.String()).To(Equal("domain=admin"))
			case *corev1.ConfigMap:
				Expect(e.Label.String()).To(Equal(types.ProviderCALabelKey + "=true"))
			}
		}
	})

	It("sends the selector server-side and keeps unrelated ConfigMaps out of the cache", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rec := &recordingRT{}
		c := rest.CopyConfig(cfg)
		c.Wrap(func(rt http.RoundTripper) http.RoundTripper { rec.next = rt; return rec })

		ns := "providerca-cache"
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}))).To(Succeed())
		labeled := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: "labeled", Namespace: ns,
			Labels: map[string]string{types.ProviderCALabelKey: "true"},
		}}
		unlabeled := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "unlabeled", Namespace: ns}}
		mismatched := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: "mismatched", Namespace: ns,
			Labels: map[string]string{types.ProviderCALabelKey: "True"},
		}}
		for _, cm := range []*corev1.ConfigMap{labeled, unlabeled, mismatched} {
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		}

		opts := ProviderCACacheOptions(nil)
		opts.Scheme = scheme.Scheme
		informers, err := cache.New(c, opts)
		Expect(err).ToNot(HaveOccurred())
		go func() { _ = informers.Start(ctx) }()
		_, err = informers.GetInformer(ctx, &corev1.ConfigMap{})
		Expect(err).ToNot(HaveOccurred())
		Expect(informers.WaitForCacheSync(ctx)).To(BeTrue())

		Eventually(func(g Gomega) {
			list := &corev1.ConfigMapList{}
			g.Expect(informers.List(ctx, list, client.InNamespace(ns))).To(Succeed())
			names := make([]string, 0, len(list.Items))
			for _, i := range list.Items {
				names = append(names, i.Name)
			}
			g.Expect(names).To(ConsistOf("labeled"))
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

		Eventually(rec.recorded, 10*time.Second, 100*time.Millisecond).Should(ContainElement(
			And(ContainSubstring("watch=true"), ContainSubstring("labelSelector=cp.ei.telekom.de%2Fprovider-ca%3Dtrue"))))
		for _, u := range rec.recorded() {
			Expect(u).To(ContainSubstring("labelSelector=cp.ei.telekom.de%2Fprovider-ca%3Dtrue"))
		}

		By("dropping the label removes the object from the cache")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(labeled), labeled)).To(Succeed())
		labeled.Labels = nil
		Expect(k8sClient.Update(ctx, labeled)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(informers.Get(ctx, client.ObjectKeyFromObject(labeled), &corev1.ConfigMap{}))
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("an uncached reader still retrieves nonmatching objects")
		direct := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(unlabeled), direct)).To(Succeed())
		Expect(types.HasProviderCALabel(direct.Labels)).To(BeFalse())
	})

	It("keeps the server-side selector across watch restarts and relists", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rec := &recordingRT{expireWatches: 2}
		c := rest.CopyConfig(cfg)
		c.Wrap(func(rt http.RoundTripper) http.RoundTripper { rec.next = rt; return rec })

		ns := "providerca-relist"
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}))).To(Succeed())

		opts := ProviderCACacheOptions(nil)
		opts.Scheme = scheme.Scheme
		informers, err := cache.New(c, opts)
		Expect(err).ToNot(HaveOccurred())
		go func() { _ = informers.Start(ctx) }()
		_, err = informers.GetInformer(ctx, &corev1.ConfigMap{})
		Expect(err).ToNot(HaveOccurred())
		Expect(informers.WaitForCacheSync(ctx)).To(BeTrue())

		By("observing a labeled object created after the expired watches")
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: ns}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: "late", Namespace: ns, Labels: map[string]string{types.ProviderCALabelKey: "true"},
		}})).To(Succeed())
		Eventually(func(g Gomega) {
			list := &corev1.ConfigMapList{}
			g.Expect(informers.List(ctx, list, client.InNamespace(ns))).To(Succeed())
			g.Expect(list.Items).To(HaveLen(1))
			g.Expect(list.Items[0].Name).To(Equal("late"))
		}, 20*time.Second, 100*time.Millisecond).Should(Succeed())

		rec.mu.Lock()
		remaining := rec.expireWatches
		rec.mu.Unlock()
		Expect(remaining).To(BeZero(), "both injected watch expirations were consumed")
		urls := rec.recorded()
		Expect(len(urls)).To(BeNumerically(">=", 3), "initial list/watch plus restarted requests: %v", urls)
		for _, u := range urls {
			Expect(u).To(ContainSubstring("labelSelector=cp.ei.telekom.de%2Fprovider-ca%3Dtrue"))
		}
	})
})
