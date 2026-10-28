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
		Entry("foreign origin", func() map[string]infoPage {
			return map[string]infoPage{"": {nextURL: "http://foreign.invalid/rovers/info?cursor=p2"}}
		}, 1),
		Entry("different endpoint", func() map[string]infoPage {
			return map[string]infoPage{"": {nextURL: server.URL + "/other/info?cursor=p2"}}
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
