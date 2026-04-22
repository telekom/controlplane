// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak/util"
)

var _ = Describe("GracefulSecrets (pure functions)", func() {

	Describe("epochSecondsFromAttr", func() {

		DescribeTable("should extract or reject epoch-seconds values",
			func(attrs map[string]interface{}, key string, expected *int64) {
				got := epochSecondsFromAttr(attrs, key)
				if expected == nil {
					Expect(got).To(BeNil())
				} else {
					Expect(got).ToNot(BeNil())
					Expect(*got).To(Equal(*expected))
				}
			},
			Entry("missing key", map[string]interface{}{}, "key", nil),
			Entry("nil value", map[string]interface{}{"key": nil}, "key", nil),
			Entry("string value", map[string]interface{}{"key": "42"}, "key", ptr.To(int64(42))),
			Entry("float64 value", map[string]interface{}{"key": float64(42)}, "key", ptr.To(int64(42))),
			Entry("invalid string", map[string]interface{}{"key": "not-a-number"}, "key", nil),
			Entry("bool value (unsupported type)", map[string]interface{}{"key": true}, "key", nil),
		)
	})

	Describe("GetSecretCreationTime", func() {

		It("should return nil for nil attrs", func() {
			Expect(GetSecretCreationTime(nil)).To(BeNil())
		})

		It("should return the timestamp when present as string", func() {
			attrs := map[string]interface{}{"client.secret.creation.time": "1750075200"}
			got := GetSecretCreationTime(attrs)
			Expect(got).ToNot(BeNil())
			Expect(*got).To(Equal(int64(1750075200)))
		})

		It("should return the timestamp when present as float64", func() {
			attrs := map[string]interface{}{"client.secret.creation.time": float64(1750075200)}
			got := GetSecretCreationTime(attrs)
			Expect(got).ToNot(BeNil())
			Expect(*got).To(Equal(int64(1750075200)))
		})

		It("should return nil when attribute is missing", func() {
			attrs := map[string]interface{}{"other-attr": "value"}
			Expect(GetSecretCreationTime(attrs)).To(BeNil())
		})
	})

	Describe("NewClientSecretRotationInfo", func() {

		It("should handle nil cred and nil client", func() {
			info := NewClientSecretRotationInfo(nil, nil)
			Expect(info.RotatedSecret).To(BeEmpty())
			Expect(info.RotatedCreatedAt).To(BeNil())
			Expect(info.RotatedExpiresAt).To(BeNil())
			Expect(info.SecretCreationTime).To(BeNil())
		})

		It("should populate all fields from cred and client attributes", func() {
			cred := &api.CredentialRepresentation{Value: ptr.To("old-secret")}
			client := &api.ClientRepresentation{
				Attributes: &map[string]interface{}{
					attrRotatedCreationTime:   "1000",
					attrRotatedExpirationTime: float64(2000),
					attrSecretCreationTime:    "3000",
				},
			}
			info := NewClientSecretRotationInfo(cred, client)
			Expect(info.RotatedSecret).To(Equal("old-secret"))
			Expect(info.RotatedCreatedAt).ToNot(BeNil())
			Expect(*info.RotatedCreatedAt).To(Equal(int64(1000)))
			Expect(info.RotatedExpiresAt).ToNot(BeNil())
			Expect(*info.RotatedExpiresAt).To(Equal(int64(2000)))
			Expect(info.SecretCreationTime).ToNot(BeNil())
			Expect(*info.SecretCreationTime).To(Equal(int64(3000)))
		})
	})

	Describe("marshalPolicyAttributes", func() {

		It("should produce a JSON array of key-value pairs", func() {
			result := marshalPolicyAttributes(util.SecretRotationClientAttribute, "true")
			expected := `[{"key":"` + util.SecretRotationClientAttribute + `","value":"true"}]`
			Expect(result).To(Equal(expected))
		})

		It("should produce valid JSON for arbitrary key-value pairs", func() {
			result := marshalPolicyAttributes("my-key", "my-value")
			Expect(result).To(Equal(`[{"key":"my-key","value":"my-value"}]`))
		})
	})
})
