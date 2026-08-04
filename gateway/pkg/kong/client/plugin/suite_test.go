// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

func TestPlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Plugin Suite")
}

var _ = Describe("Plugin", func() {

	Context("Util", func() {

		It("should cast the plugin", func() {
			var plugin client.CustomPlugin
			var expectedPlugin *AclPlugin

			plugin = &AclPlugin{
				Id: "123",
			}
			Expect(As(plugin, &expectedPlugin)).To(BeTrue())
			Expect(expectedPlugin.Id).To(Equal("123"))
		})

		It("should not cast the plugin", func() {
			var plugin client.CustomPlugin
			var expectedPlugin *AclPlugin

			plugin = &JwtPlugin{
				Id: "123",
			}
			Expect(As(plugin, &expectedPlugin)).To(BeFalse())
		})
	})

	Context("Jumper", func() {

		var (
			expected = &JumperConfig{
				OAuth: map[ConsumerId]OauthCredentials{
					"123": {
						ClientId:     "client-id",
						ClientSecret: "topsecret",
						Scopes:       "scope1 scope2",
					},
				},
				LoadBalancing: &LoadBalancing{
					Servers: []LoadBalancingServer{
						{
							Upstream: "http://upstream.url:8080/api/v1",
							Weight:   2,
						},
						{
							Upstream: "http://upstream2.url:8080/api/v1",
							Weight:   1,
						},
					},
				},
			}
			// This must be the base64 encoded version of the expected JumperConfig
			encodedJumperConfig = "eyJvYXV0aCI6eyIxMjMiOnsiY2xpZW50SWQiOiJjbGllbnQtaWQiLCJjbGllbnRTZWNyZXQiOiJ0b3BzZWNyZXQiLCJzY29wZXMiOiJzY29wZTEgc2NvcGUyIn19LCJsb2FkQmFsYW5jaW5nIjp7InNlcnZlcnMiOlt7InVwc3RyZWFtIjoiaHR0cDovL3Vwc3RyZWFtLnVybDo4MDgwL2FwaS92MSIsIndlaWdodCI6Mn0seyJ1cHN0cmVhbSI6Imh0dHA6Ly91cHN0cmVhbTIudXJsOjgwODAvYXBpL3YxIiwid2VpZ2h0IjoxfV19fQ=="
		)

		It("should return an empty JumperConfig", func() {

			actual := NewJumperConfig()
			Expect(actual).To(Equal(&JumperConfig{
				OAuth:     map[ConsumerId]OauthCredentials{},
				BasicAuth: map[ConsumerId]BasicAuthCredentials{},
				Claims:    map[ConsumerId][]Claim{},
				// LoadBalancing is optional and not set by default
			}))

		})

		It("should correctly base64-encode", func() {
			JumperConfig := expected
			strVal := ToBase64OrDie(JumperConfig)
			Expect(strVal).To(Equal(encodedJumperConfig))
		})

		It("should correctly base64-decode", func() {
			actual, err := FromBase64[JumperConfig](encodedJumperConfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})
	})

	Context("Encode", func() {

		It("should correctly encode a string map", func() {
			m := New()
			m.AddKV("key2", "value2")
			m.AddKV("key1", "value1")

			encoded, err := m.MarshalJSON()
			Expect(err).ToNot(HaveOccurred())
			Expect(string(encoded)).To(Equal(`["key1:value1","key2:value2"]`))
		})

		It("should encode an empty string map as an empty array", func() {
			encoded, err := New().MarshalJSON()
			Expect(err).ToNot(HaveOccurred())
			Expect(string(encoded)).To(Equal(`[]`))
		})

		// Kong is written on every reconciliation if the encoding is not stable,
		// because the config read back is compared with the config to be sent.
		It("should encode the same map identically every time", func() {
			m := New()
			for _, key := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
				m.AddKV(key, "value")
			}

			first, err := m.MarshalJSON()
			Expect(err).ToNot(HaveOccurred())
			for range 20 {
				again, err := m.MarshalJSON()
				Expect(err).ToNot(HaveOccurred())
				Expect(again).To(Equal(first))
			}
		})

		It("should escape values that are not JSON-safe", func() {
			m := New()
			m.AddKV("quote", `a"b`)

			encoded, err := m.MarshalJSON()
			Expect(err).ToNot(HaveOccurred())

			var decoded []string
			Expect(json.Unmarshal(encoded, &decoded)).To(Succeed())
			Expect(decoded).To(Equal([]string{`quote:a"b`}))
		})

		It("should correctly decode a string map", func() {
			m := New()
			err := json.Unmarshal([]byte(`["key1:value1","key2:value2"]`), m)
			Expect(err).ToNot(HaveOccurred())

			Expect(m.items).To(HaveLen(2))
			Expect(m.items).To(HaveKey("key1"))
			Expect(m.items).To(HaveKey("key2"))
			Expect(m.Get("key1")).To(Equal("value1"))
			Expect(m.Get("key2")).To(Equal("value2"))
		})
	})

})
