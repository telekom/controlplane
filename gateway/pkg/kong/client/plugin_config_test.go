// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"
	mockclient "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config of an existing request-transformer plugin", func() {
	var (
		ctx     context.Context
		api     *mockclient.MockKongAdminApi
		client  clientpkg.KongClient
		rt      *plugin.RequestTransformerPlugin
		written writtenBody
	)

	goTags := []string{"env--test", "plugin--request-transformer", "route--r1", "consumer--none"}

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test")
		api = mockclient.NewMockKongAdminApi(GinkgoT())
		client = clientpkg.NewKongClient(api)
		route := &v1.Route{ObjectMeta: metav1.ObjectMeta{Name: "r1"}}
		route.SetProperty("kongRequestTransformerPluginId", legacyID)
		rt = plugin.RequestTransformerPluginFromRoute(route)
		rt.Config.Append.AddHeader("x", "y")
		written = writtenBody{}
	})

	// goConfig is the config Go calculates for rt, as sent to Kong.
	goConfig := func() map[string]any {
		data, err := json.Marshal(rt.GetConfig())
		Expect(err).NotTo(HaveOccurred())
		var config map[string]any
		Expect(json.Unmarshal(data, &config)).To(Succeed())
		return config
	}

	expectCurrent := func(tags []string, config map[string]any) {
		expectRouteID(api)
		expectPluginRecord(api, legacyID, &pluginRecord{id: legacyID, name: "request-transformer", route: routeID, tags: tags, config: config})
	}

	It("replaces a differing config with the Go config under the same ID and keeps the tags", func() {
		expectCurrent([]string{legacyTag, "custom"}, map[string]any{
			"append": map[string]any{"headers": []any{"old:1", "x:y"}, "body": []any{"b:1"}},
			"remove": map[string]any{"headers": []any{"h"}},
		})
		expectRouteWrite(api, &written)

		_, err := client.CreateOrReplacePlugin(ctx, rt)
		Expect(err).NotTo(HaveOccurred())
		Expect(written.id).To(Equal(legacyID))
		Expect(*written.body.Config).To(Equal(goConfig()))
		Expect(*written.body.Tags).To(ConsistOf(append([]string{legacyTag, "custom"}, goTags...)))
	})

	It("sends the Go config on a tag-only adoption write", func() {
		expectCurrent([]string{legacyTag}, map[string]any{
			"append": map[string]any{"headers": []any{"x:y"}, "body": []any{"b:1"}},
			"rename": map[string]any{}, "replace": map[string]any{}, "remove": map[string]any{}, "add": map[string]any{},
		})
		expectRouteWrite(api, &written)

		_, err := client.CreateOrReplacePlugin(ctx, rt)
		Expect(err).NotTo(HaveOccurred())
		Expect(written.id).To(Equal(legacyID))
		Expect(*written.body.Config).To(Equal(goConfig()))
	})

	It("does not write when the Go config matches apart from Kong defaults", func() {
		expectCurrent(append([]string{legacyTag}, goTags...), map[string]any{
			"append": map[string]any{"headers": []any{"x:y"}, "body": []any{}},
			"rename": map[string]any{}, "replace": map[string]any{}, "remove": map[string]any{"headers": []any{}}, "add": map[string]any{},
		})

		_, err := client.CreateOrReplacePlugin(ctx, rt)
		Expect(err).NotTo(HaveOccurred())
		Expect(rt.GetId()).To(Equal(legacyID))
	})
})
