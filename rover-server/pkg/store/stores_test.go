// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	commonStore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"k8s.io/client-go/rest"

	"github.com/telekom/controlplane/rover-server/pkg/store"
)

func TestStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Store Suite")
}

const spectrePathPrefix = "/apis/spectre.cp.ei.telekom.de/"

// fakeAPIServer answers every list with an empty list and holds every watch
// open, recording the paths the store informers request.
type fakeAPIServer struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
}

func newFakeAPIServer() *fakeAPIServer {
	s := &fakeAPIServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.paths = append(s.paths, r.URL.Path)
		s.mu.Unlock()

		if r.URL.Query().Get("watch") == "true" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"List","metadata":{"resourceVersion":"1"},"items":[]}`))
	}))
	return s
}

// requested reports whether any request path starts with prefix.
func (s *fakeAPIServer) requested(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.paths {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

var _ = Describe("NewStores", func() {
	var (
		ctx    context.Context
		server *fakeAPIServer
	)

	BeforeEach(func() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(context.Background())
		server = newFakeAPIServer()
		DeferCleanup(func() {
			cancel()
			server.CloseClientConnections()
			server.Close()
		})

		wasEnabled := cconfig.FeatureSpectre.IsEnabled()
		DeferCleanup(func() { cconfig.SetFeatureEnabled(cconfig.FeatureSpectre, wasEnabled) })
	})

	newStores := func() *store.Stores {
		return store.NewStores(ctx, &rest.Config{Host: server.URL}, inmemory.DatabaseOpts{}, inmemory.InformerOpts{})
	}

	It("uses stand-in Spectre stores that never reach the API server when the feature is disabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeatureSpectre, false)

		s := newStores()

		// The Zone store is created last, so its request shows the informers are running.
		Eventually(func() bool { return server.requested("/apis/admin.cp.ei.telekom.de/") }).Should(BeTrue())
		Consistently(func() bool { return server.requested(spectrePathPrefix) }, "500ms").Should(BeFalse())

		apps, err := s.SpectreApplicationStore.List(ctx, commonStore.NewListOpts())
		Expect(err).NotTo(HaveOccurred())
		Expect(apps.Items).To(BeEmpty())
		listeners, err := s.SpectreListenerStore.List(ctx, commonStore.NewListOpts())
		Expect(err).NotTo(HaveOccurred())
		Expect(listeners.Items).To(BeEmpty())
	})

	It("uses Spectre stores backed by the API server when the feature is enabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeatureSpectre, true)

		newStores()

		Eventually(func() bool { return server.requested(spectrePathPrefix + "v1/spectreapplications") }).Should(BeTrue())
		Eventually(func() bool { return server.requested(spectrePathPrefix + "v1/listeners") }).Should(BeTrue())
	})
})
