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
	"sync"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Legacy plugin slot adoption over the generated client", func() {
	It("lists the plugins of the route and adopts the plugin in the slot under its ID", func() {
		tag := "ei__telekom__de--realm---bond"

		var (
			mu         sync.Mutex
			tagFilters []string
			slotReads  int
			putPath    string
			putBody    map[string]any
		)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/routes/route-id":
				_, _ = io.WriteString(w, `{"id":"route-id","name":"bond.r1"}`)
			case r.Method == http.MethodGet && r.URL.Path == "/plugins":
				tagFilters = append(tagFilters, r.URL.Query().Get("tags"))
				_, _ = io.WriteString(w, emptyPage)
			case r.Method == http.MethodGet && r.URL.Path == "/routes/route-id/plugins":
				slotReads++
				_, _ = io.WriteString(w, pageOf(nil,
					(&pluginRecord{id: "other", name: "jwt", route: "route-id", tags: []string{tag}}).json(),
					(&pluginRecord{
						id: "legacy-id", name: "acl", route: "route-id", tags: []string{tag, "acl", "bond.r1"},
						config: map[string]any{"allow": []any{"group-a"}},
					}).json()))
			case r.Method == http.MethodPut && r.URL.Path == "/routes/route-id/plugins/legacy-id":
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
		plugin := &boundTestPlugin{
			testPlugin: testPlugin{name: "acl", route: ptr("r1"), config: map[string]any{"allow": []any{"group-a"}}},
			routeId:    "route-id",
			namePrefix: "bond",
		}

		_, err = client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
		Expect(plugin.GetId()).To(Equal("legacy-id"))
		Expect(tagFilters).To(Equal([]string{"env--test,plugin--acl,route--r1,consumer--none"}))
		Expect(slotReads).To(Equal(1))
		Expect(putPath).To(Equal("/routes/route-id/plugins/legacy-id"))
		Expect(putBody["route"]).To(Equal(map[string]any{"id": "route-id"}))
		Expect(putBody["tags"]).To(ConsistOf(tag, "acl", "bond.r1", "env--test", "plugin--acl", "route--r1", "consumer--none", "realm--bond"))
	})
})
