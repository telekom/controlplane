// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

// The jumper_config JSON is a wire contract with gateway-jumper. Jumper's model
// (jumper/model/config/JumperConfig.java) declares:
//
//	private HashMap<String, RouteListener> routeListener;  // RouteListener{issue, serviceOwner}
//	private GatewayClient gatewayClient;                   // GatewayClient{id, secret, issuer}
//
// so the listener credentials belong to a TOP-LEVEL "gatewayClient" object, not
// inside each routeListener entry. The legacy gateway does exactly this in
// KongCeClient.appendOrUpdateListenerForRequestTransformerPlugin.
//
// Jumper's ObjectMapper sets FAIL_ON_UNKNOWN_PROPERTIES=false, so any field we
// put in the wrong place is silently dropped rather than rejected — which is why
// this has to be asserted on the serialized form.
var _ = Describe("JumperConfig wire format", func() {
	marshal := func(jc *plugin.JumperConfig) map[string]any {
		raw, err := json.Marshal(jc)
		Expect(err).ToNot(HaveOccurred())
		var out map[string]any
		Expect(json.Unmarshal(raw, &out)).To(Succeed())
		return out
	}

	It("emits listener credentials as a top-level gatewayClient object", func() {
		jc := plugin.NewJumperConfig()
		jc.RouteListener = map[plugin.ConsumerId]plugin.RouteListenerEntry{
			"consumer-a": {Issue: "/api/v1", ServiceOwner: "provider-a"},
		}
		jc.GatewayClient = &plugin.GatewayClient{
			Id:     "gateway-client-id",
			Secret: "gateway-client-secret",
			Issuer: "https://iris.example.com/auth/realms/test",
		}

		out := marshal(jc)

		gwc, ok := out["gatewayClient"].(map[string]any)
		Expect(ok).To(BeTrue(), `jumper reads credentials from a top-level "gatewayClient" object`)
		Expect(gwc).To(HaveKeyWithValue("id", "gateway-client-id"))
		Expect(gwc).To(HaveKeyWithValue("secret", "gateway-client-secret"))
		Expect(gwc).To(HaveKeyWithValue("issuer", "https://iris.example.com/auth/realms/test"))
	})

	It("emits only issue and serviceOwner per routeListener entry", func() {
		jc := plugin.NewJumperConfig()
		jc.RouteListener = map[plugin.ConsumerId]plugin.RouteListenerEntry{
			"consumer-a": {Issue: "/api/v1", ServiceOwner: "provider-a"},
		}

		out := marshal(jc)

		entries, ok := out["routeListener"].(map[string]any)
		Expect(ok).To(BeTrue())
		entry, ok := entries["consumer-a"].(map[string]any)
		Expect(ok).To(BeTrue())

		Expect(entry).To(HaveKeyWithValue("issue", "/api/v1"))
		Expect(entry).To(HaveKeyWithValue("serviceOwner", "provider-a"))
		// Jumper's RouteListener class has no such fields; anything else here is
		// silently discarded on deserialization.
		Expect(entry).ToNot(HaveKey("clientId"))
		Expect(entry).ToNot(HaveKey("issuer"))
		Expect(entry).ToNot(HaveKey("secret"))
		Expect(entry).To(HaveLen(2))
	})

	It("omits gatewayClient entirely when no listener is configured", func() {
		out := marshal(plugin.NewJumperConfig())
		Expect(out).ToNot(HaveKey("gatewayClient"))
		Expect(out).ToNot(HaveKey("routeListener"))
	})
})
