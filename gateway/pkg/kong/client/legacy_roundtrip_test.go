// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Legacy plugin discovery over the generated client", func() {
	It("sends the legacy tags as one literal filter and adopts the plugin under its ID", func() {
		tag := clientpkg.LegacyPluginTag
		legacyFilter := tag + ",acl,c1"

		var (
			mu         sync.Mutex
			tagFilters []string
			putPath    string
			putBody    map[string]any
		)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/consumers/c1":
				_, _ = io.WriteString(w, `{"id":"consumer-id","username":"c1"}`)
			case r.Method == http.MethodGet && r.URL.Path == "/plugins":
				filter := r.URL.Query().Get("tags")
				tagFilters = append(tagFilters, filter)
				if filter == legacyFilter {
					_, _ = io.WriteString(w, pageOf(nil, (&pluginRecord{
						id: "legacy-id", name: "acl", consumer: "consumer-id", tags: []string{tag, "acl", "c1"},
						config: map[string]any{"allow": []any{"group-a"}},
					}).json()))
					return
				}
				_, _ = io.WriteString(w, emptyPage)
			case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/consumers/c1/plugins/"):
				putPath = r.URL.Path
				body, err := io.ReadAll(r.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(json.Unmarshal(body, &putBody)).To(Succeed())
				_, _ = io.WriteString(w, `{"id":"legacy-id","name":"acl"}`)
			default:
				w.WriteHeader(http.StatusTeapot)
			}
		}))
		DeferCleanup(server.Close)

		api, err := kong.NewClientWithResponses(server.URL)
		Expect(err).NotTo(HaveOccurred())
		client := clientpkg.NewKongClient(api)
		ctx := contextutil.WithEnv(context.Background(), "test")
		plugin := &testPlugin{name: "acl", consumer: ptr("c1"), config: map[string]any{"allow": []any{"group-a"}}}

		_, err = client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
		Expect(plugin.GetId()).To(Equal("legacy-id"))
		Expect(tagFilters).To(Equal([]string{"env--test,plugin--acl,consumer--c1", legacyFilter}))
		Expect(putPath).To(Equal("/consumers/c1/plugins/legacy-id"))
		Expect(putBody["tags"]).To(ConsistOf(tag, "acl", "c1", "env--test", "plugin--acl", "consumer--c1"))
	})
})
