// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type infoPage struct {
	apps    []any
	next    string
	nextURL string
	status  int
	raw     string
}

var _ = Describe("InfoMany pagination", func() {
	var (
		mu      sync.Mutex
		queries []url.Values
		pages   map[string]infoPage
		server  *httptest.Server
		ctx     context.Context
		handler *common.BaseHandler
	)
	recorded := func() []url.Values {
		mu.Lock()
		defer mu.Unlock()
		return append([]url.Values(nil), queries...)
	}
	BeforeEach(func() {
		previous := viper.Get("access.token")
		viper.Set("access.token", "local-token")
		DeferCleanup(func() { viper.Set("access.token", previous) })
		queries = nil
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			mu.Lock()
			queries = append(queries, r.URL.Query())
			mu.Unlock()
			Expect(r.Header.Get("Authorization")).To(Equal("Bearer local-token"))
			p := pages[r.URL.Query().Get("cursor")]
			if p.raw != "" {
				_, err := w.Write([]byte(p.raw))
				Expect(err).NotTo(HaveOccurred())
				return
			}
			if p.status != 0 {
				w.WriteHeader(p.status)
				return
			}
			body := map[string]any{"environment": "env", "hub": "hub", "team": "team", "applications": p.apps}
			next := p.nextURL
			if p.next != "" {
				query := url.Values{"cursor": {p.next}, "limit": {"2"}}
				next = (&url.URL{Scheme: "http", Host: r.Host, Path: r.URL.Path, RawQuery: query.Encode()}).String()
			}
			if next != "" {
				body["_links"] = map[string]any{"next": next}
			}
			w.Header().Set("Content-Type", "application/json")
			Expect(json.NewEncoder(w).Encode(body)).To(Succeed())
		}))
		DeferCleanup(server.Close)
		ctx = config.NewContext(context.Background(), &config.Token{ServerUrl: server.URL, Group: "my-group", Team: "my-team"})
		handler = common.NewBaseHandler("v1", "Rover", "rovers", 100)
		handler.SupportsInfo = true
	})
	app := func(name string) any { return map[string]any{"name": name} }
	It("combines pages including an empty intermediate page", func() {
		pages = map[string]infoPage{"": {apps: []any{app("a")}, next: "p2"}, "p2": {next: "p3"}, "p3": {apps: []any{app("b")}}}
		result, err := handler.InfoMany(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(map[string]any{"environment": "env", "hub": "hub", "team": "team", "applications": []any{app("a"), app("b")}}))
		Expect(recorded()).To(HaveLen(3))
		Expect(recorded()[2].Get("limit")).To(Equal("2"))
	})
	It("makes one request for explicit names", func() {
		pages = map[string]infoPage{"": {apps: []any{app("a")}, next: "p2"}}
		_, err := handler.InfoMany(ctx, []string{"a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(recorded()).To(Equal([]url.Values{{"names": {"a"}}}))
	})
	It("supports old servers without links", func() {
		pages = map[string]infoPage{"": {apps: []any{app("a")}}}
		result, err := handler.InfoMany(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveKeyWithValue("applications", []any{app("a")}))
		Expect(recorded()).To(HaveLen(1))
	})
	It("follows absolute list links and still supports raw cursors", func() {
		pages = map[string]infoPage{
			"":   {raw: `{"items":[{"name":"a"}],"_links":{"next":"` + server.URL + `/rovers?cursor=p2&limit=2"}}`},
			"p2": {raw: `{"items":[{"name":"b"}],"_links":{}}`},
		}
		items, err := handler.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(Equal([]any{app("a"), app("b")}))
		Expect(recorded()).To(HaveLen(2))
		Expect(recorded()[1].Get("limit")).To(Equal("2"))
		_, err = handler.ListWithCursor(ctx, "p2")
		Expect(err).NotTo(HaveOccurred())
		Expect(recorded()[2].Get("cursor")).To(Equal("p2"))
	})
	DescribeTable("fails without partial results",
		func(build func() map[string]infoPage, calls int) {
			pages = build()
			result, err := handler.InfoMany(ctx, nil)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(recorded()).To(HaveLen(calls))
		},
		Entry("later error", func() map[string]infoPage {
			return map[string]infoPage{"": {next: "p2"}, "p2": {status: http.StatusInternalServerError}}
		}, 2),
		Entry("repeated cursor", func() map[string]infoPage {
			return map[string]infoPage{"": {next: "p2"}, "p2": {next: "p2"}}
		}, 2),
		Entry("link without cursor", func() map[string]infoPage {
			return map[string]infoPage{"": {nextURL: server.URL + "/rovers/info?limit=2"}}
		}, 1),
		Entry("null page", func() map[string]infoPage {
			return map[string]infoPage{"": {raw: `null`}}
		}, 1),
		Entry("missing items", func() map[string]infoPage {
			return map[string]infoPage{"": {next: "p2"}, "p2": {raw: `{}`}}
		}, 2),
		Entry("malformed continuation", func() map[string]infoPage {
			return map[string]infoPage{"": {next: "p2"}, "p2": {raw: `{"applications":[],"_links":{"next":123}}`}}
		}, 2),
		Entry("malformed links", func() map[string]infoPage {
			return map[string]infoPage{"": {raw: `{"applications":[],"_links":false}`}}
		}, 1),
	)
})

var _ = Describe("Pagination links", func() {
	var (
		mu          sync.Mutex
		requests    []*url.URL
		links       []string
		server      *httptest.Server
		foreignHits int
		foreign     *httptest.Server
		handler     *common.BaseHandler
	)
	expand := func(link string) string {
		return strings.NewReplacer("SERVER", server.URL, "FOREIGN", foreign.URL).Replace(link)
	}
	recorded := func() []*url.URL {
		mu.Lock()
		defer mu.Unlock()
		return append([]*url.URL(nil), requests...)
	}
	BeforeEach(func() {
		previous := viper.Get("access.token")
		viper.Set("access.token", "local-token")
		DeferCleanup(func() { viper.Set("access.token", previous) })
		requests, links, foreignHits = nil, nil, 0
		foreign = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			foreignHits++
			mu.Unlock()
		}))
		DeferCleanup(foreign.Close)
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			Expect(r.Header.Get("Authorization")).To(Equal("Bearer local-token"))
			mu.Lock()
			n := len(requests)
			requests = append(requests, r.URL)
			mu.Unlock()
			body := map[string]any{"items": []any{map[string]any{"n": n}}, "applications": []any{map[string]any{"n": n}}}
			if n < len(links) {
				body["_links"] = map[string]any{"next": expand(links[n])}
			}
			w.Header().Set("Content-Type", "application/json")
			Expect(json.NewEncoder(w).Encode(body)).To(Succeed())
		}))
		DeferCleanup(server.Close)
		handler = common.NewBaseHandler("v1", "Rover", "rovers", 100)
		handler.SupportsInfo = true
	})
	AfterEach(func() {
		mu.Lock()
		defer mu.Unlock()
		Expect(foreignHits).To(BeZero())
	})
	ctxFor := func(prefix string) context.Context {
		return config.NewContext(context.Background(), &config.Token{ServerUrl: server.URL + prefix, Group: "g", Team: "t"})
	}

	DescribeTable("rebuilds next requests from the original URL",
		func(prefix, link string) {
			ctx := ctxFor(prefix)
			links = []string{link + "?cursor=c2&limit=20&shallowInfo=false&names=x&group=evil"}
			_, err := handler.List(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorded()).To(HaveLen(2))
			Expect(recorded()[1].Path).To(Equal(prefix + "/rovers"))
			Expect(recorded()[1].Query()).To(Equal(url.Values{"cursor": {"c2"}, "limit": {"20"}}))

			requests = nil
			result, err := handler.InfoMany(common.WithShallowInfo(ctx), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("applications", HaveLen(2)))
			Expect(recorded()[1].Path).To(Equal(prefix + "/rovers/info"))
			Expect(recorded()[1].Query()).To(Equal(url.Values{"cursor": {"c2"}, "limit": {"20"}, "shallowInfo": {"true"}}))

			requests = nil
			_, err = handler.ListWithCursor(ctx, expand(link)+"?cursor=c3")
			Expect(err).NotTo(HaveOccurred())
			Expect(recorded()[0].Path).To(Equal(prefix + "/rovers"))
			Expect(recorded()[0].Query()).To(Equal(url.Values{"cursor": {"c3"}}))
		},
		Entry("/rover/api with internal link", "/rover/api", "http://internal-host/rovers"),
		Entry("/rover/v3 with stripped link", "/rover/v3", "SERVER/rovers"),
		Entry("unprefixed with foreign link", "", "FOREIGN/other/path"),
		Entry("credentials in link", "/rover/api", "http://user:pw@FOREIGN/x"),
	)

	It("keeps opaque cursors and encodes them safely", func() {
		links = []string{"FOREIGN/x?cursor=" + url.QueryEscape("a b&c=/+?#%") + "&limit=5"}
		_, err := handler.List(ctxFor("/rover/api"))
		Expect(err).NotTo(HaveOccurred())
		Expect(recorded()[1].Query()).To(Equal(url.Values{"cursor": {"a b&c=/+?#%"}, "limit": {"5"}}))
	})

	It("ends when there is no next link", func() {
		_, err := handler.List(ctxFor("/rover/api"))
		Expect(err).NotTo(HaveOccurred())
		Expect(recorded()).To(HaveLen(1))
	})

	DescribeTable("detects repeated pages across different link hosts",
		func(list bool) {
			links = []string{"SERVER/rovers?cursor=c2", "FOREIGN/x?cursor=c2"}
			var err error
			if list {
				_, err = handler.List(ctxFor("/rover/api"))
			} else {
				_, err = handler.InfoMany(ctxFor("/rover/api"), nil)
			}
			Expect(err).To(MatchError(ContainSubstring("repeated")))
			Expect(recorded()).To(HaveLen(2))
		},
		Entry("list", true),
		Entry("info", false),
	)

	DescribeTable("rejects invalid pagination links",
		func(link string) {
			links = []string{link}
			_, err := handler.List(ctxFor("/rover/api"))
			Expect(err).To(MatchError(ContainSubstring("invalid pagination link")))
			Expect(recorded()).To(HaveLen(1))
			requests = nil
			_, err = handler.InfoMany(ctxFor("/rover/api"), nil)
			Expect(err).To(MatchError(ContainSubstring("invalid pagination link")))
			Expect(recorded()).To(HaveLen(1))
		},
		Entry("missing cursor", "SERVER/rovers?limit=2"),
		Entry("empty cursor", "SERVER/rovers?cursor="),
		Entry("duplicate cursor", "SERVER/rovers?cursor=a&cursor=b"),
		Entry("duplicate limit", "SERVER/rovers?cursor=a&limit=1&limit=2"),
		Entry("non-numeric limit", "SERVER/rovers?cursor=a&limit=x"),
		Entry("zero limit", "SERVER/rovers?cursor=a&limit=0"),
		Entry("malformed query", "SERVER/rovers?cursor=%zz"),
	)

	It("rejects malformed info links and rebuilds relative ones", func() {
		links = []string{"http://[::1/rovers?cursor=a"}
		_, err := handler.InfoMany(ctxFor("/rover/api"), nil)
		Expect(err).To(MatchError(ContainSubstring("invalid pagination link")))
		Expect(recorded()).To(HaveLen(1))

		requests, links = nil, []string{"/elsewhere?cursor=c2"}
		_, err = handler.InfoMany(ctxFor("/rover/api"), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(recorded()[1].Path).To(Equal("/rover/api/rovers/info"))
		Expect(recorded()[1].Query()).To(Equal(url.Values{"cursor": {"c2"}}))
	})
})
