// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"golang.org/x/time/rate"
)

// fakeListKong serves paged GET /routes and GET /consumers plus single-entity GETs.
type fakeListKong struct {
	mu        sync.Mutex
	server    *httptest.Server
	routes    []string // route names in listing order
	consumers []string // consumer IDs in listing order
	listFail  int      // status code returned for list requests if non-zero
	requests  []*http.Request
}

func newFakeListKong() *fakeListKong {
	f := &fakeListKong{}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeListKong) sizes(path string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.requests {
		if r.URL.Path == path {
			out = append(out, r.URL.Query().Get("size"))
		}
	}
	return out
}

func (f *fakeListKong) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.requests {
		out = append(out, r.URL.Path)
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeListKong) page(w http.ResponseWriter, r *http.Request, items []string, toEntity func(string) map[string]any) {
	if f.listFail != 0 {
		w.WriteHeader(f.listFail)
		return
	}
	size, err := strconv.Atoi(r.URL.Query().Get("size"))
	if err != nil || size <= 0 {
		size = 100
	}
	start, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	end := min(start+size, len(items))
	data := []map[string]any{}
	for _, it := range items[start:end] {
		data = append(data, toEntity(it))
	}
	resp := map[string]any{"data": data}
	if end < len(items) {
		resp["offset"] = strconv.Itoa(end)
	}
	writeJSON(w, resp)
}

func (f *fakeListKong) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.URL.Path == "/routes":
		f.page(w, r, f.routes, func(n string) map[string]any {
			if strings.HasPrefix(n, "id:") {
				return map[string]any{"id": strings.TrimPrefix(n, "id:")}
			}
			return map[string]any{"id": "id-" + n, "name": n}
		})
	case r.URL.Path == "/consumers":
		f.page(w, r, f.consumers, func(id string) map[string]any { return map[string]any{"id": id} })
	case len(parts) == 2 && parts[0] == "routes":
		writeJSON(w, map[string]any{"id": "id-" + parts[1], "name": parts[1], "service": map[string]any{"id": "svc"}})
	case len(parts) == 2 && parts[0] == "services":
		writeJSON(w, map[string]any{"id": parts[1]})
	case len(parts) == 3 && parts[2] == "plugins":
		writeJSON(w, map[string]any{"data": []any{}})
	case len(parts) == 2 && parts[0] == "consumers":
		writeJSON(w, map[string]any{"id": parts[1]})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func names(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s-%03d", prefix, n-i) // descending to verify sorting
	}
	return out
}

func collect(ch <-chan *snapshot.Snapshot) []string {
	var ids []string
	Eventually(func(g Gomega) {
		for {
			select {
			case s, ok := <-ch:
				if !ok {
					return
				}
				ids = append(ids, s.Id)
			default:
				g.Expect(false).To(BeTrue(), "channel not closed yet")
				return
			}
		}
	}).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(Succeed())
	return ids
}

var _ = Describe("KongSource listing", func() {
	var (
		fake *fakeListKong
		src  *KongSource
		ctx  context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		fake = newFakeListKong()
		DeferCleanup(fake.server.Close)
		client, err := kong.NewClientWithResponses(fake.server.URL)
		Expect(err).NotTo(HaveOccurred())
		src = NewKongSource(client)
		src.listRatelimiter = rate.NewLimiter(rate.Inf, 1)
	})

	It("returns fewer than one page when the limit is small", func() {
		fake.routes = names("r", 50)
		ids, err := src.listIDs(ctx, "route", 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(ids).To(Equal(fake.routes[:10]))
		Expect(fake.sizes("/routes")).To(Equal([]string{"10"}))
	})

	It("spans multiple pages and truncates to the limit", func() {
		fake.routes = names("r", 300)
		ids, err := src.listIDs(ctx, "route", 250)
		Expect(err).NotTo(HaveOccurred())
		Expect(ids).To(HaveLen(250))
		Expect(ids).To(Equal(fake.routes[:250]))
		Expect(fake.sizes("/routes")).To(Equal([]string{"100", "100", "50"}))
	})

	It("returns all resources across pages with limit 0", func() {
		fake.consumers = names("c", 230)
		ids, err := src.listIDs(ctx, "consumer", 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(ids).To(Equal(fake.consumers))
		Expect(fake.sizes("/consumers")).To(Equal([]string{"100", "100", "100"}))
	})

	It("stops when there is no further offset", func() {
		fake.routes = names("r", 120)
		ids, err := src.listIDs(ctx, "route", 500)
		Expect(err).NotTo(HaveOccurred())
		Expect(ids).To(HaveLen(120))
		Expect(fake.sizes("/routes")).To(HaveLen(2))
	})

	It("falls back to the route id when the name is missing", func() {
		fake.routes = []string{"id:abc", "named"}
		ids, err := src.listIDs(ctx, "route", 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(ids).To(Equal([]string{"abc", "named"}))
	})

	It("returns an error on non-2xx responses", func() {
		fake.listFail = http.StatusInternalServerError
		_, err := src.listIDs(ctx, "route", 10)
		Expect(err).To(MatchError(ContainSubstring("unexpected status code 500")))
		_, err = src.listIDs(ctx, "consumer", 10)
		Expect(err).To(MatchError(ContainSubstring("unexpected status code 500")))
	})

	It("passes the configured tags", func() {
		src.tags = []string{"a", "b"}
		fake.routes = names("r", 1)
		_, err := src.listIDs(ctx, "route", 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(fake.requests[0].URL.Query().Get("tags")).To(Equal("a,b"))
	})

	It("rejects unsupported resource types", func() {
		err := src.TakeGlobalSnapshot(ctx, "service", 0, make(chan *snapshot.Snapshot))
		Expect(err).To(MatchError("unsupported resource type: service"))
	})

	It("returns listing errors from TakeGlobalSnapshot", func() {
		fake.listFail = http.StatusUnauthorized
		err := src.TakeGlobalSnapshot(ctx, "route", 10, make(chan *snapshot.Snapshot))
		Expect(err).To(MatchError(ContainSubstring("unexpected status code 401")))
	})

	It("snapshots all routes in sorted order with limit 0 without deadlock", func() {
		fake.routes = names("r", 150)
		ch := make(chan *snapshot.Snapshot)
		Expect(src.TakeGlobalSnapshot(ctx, "route", 0, ch)).To(Succeed())
		ids := collect(ch)
		Expect(ids).To(HaveLen(150))
		Expect(ids[0]).To(Equal("r-001"))
		Expect(ids).To(Equal(slices.Sorted(slices.Values(ids))))
	})

	It("snapshots consumers via the consumer endpoints", func() {
		fake.consumers = []string{"c2", "c1", "c3"}
		ch := make(chan *snapshot.Snapshot, 10)
		Expect(src.TakeGlobalSnapshot(ctx, "consumer", 2, ch)).To(Succeed())
		Expect(collect(ch)).To(Equal([]string{"c1", "c2"}))
		for _, p := range fake.paths() {
			Expect(p).NotTo(HavePrefix("/routes"))
		}
		Expect(fake.paths()).To(ContainElement("/consumers/c1"))
	})

	It("stops and closes the channel when the context is cancelled", func() {
		fake.routes = names("r", 5)
		cctx, cancel := context.WithCancel(ctx)
		ch := make(chan *snapshot.Snapshot)
		Expect(src.TakeGlobalSnapshot(cctx, "route", 0, ch)).To(Succeed())
		cancel()
		Eventually(func() bool {
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return true
					}
				default:
					return false
				}
			}
		}).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(BeTrue())
	})
})
