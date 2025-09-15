// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend_test

import (
	"encoding/json"
	"maps"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/test/mocks"
)

var _ = Describe("Secret", func() {
	Context("New*Secrets functions", func() {
		It("should create new environment secrets", func() {
			secrets := backend.NewEnvironmentSecrets()
			Expect(secrets).ToNot(BeNil())

			secretMap, err := secrets.GetSecrets()
			Expect(err).ToNot(HaveOccurred())
			Expect(secretMap).To(HaveLen(1))
			Expect(secretMap).To(HaveKey("zones"))
			Expect(secretMap["zones"].Value()).To(Equal("{}"))
			Expect(secretMap["zones"].AllowChange()).To(BeFalse())
		})

		It("should create new team secrets", func() {
			secrets := backend.NewTeamSecrets()
			Expect(secrets).ToNot(BeNil())

			secretMap, err := secrets.GetSecrets()
			Expect(err).ToNot(HaveOccurred())
			Expect(secretMap).To(HaveLen(2))
			Expect(secretMap).To(HaveKey("clientSecret"))
			Expect(secretMap).To(HaveKey("teamToken"))
			Expect(secretMap["clientSecret"].Value()).ToNot(BeEmpty())
			Expect(secretMap["teamToken"].Value()).ToNot(BeEmpty())
			Expect(secretMap["clientSecret"].AllowChange()).To(BeFalse())
			Expect(secretMap["teamToken"].AllowChange()).To(BeFalse())
		})

		It("should create new application secrets", func() {
			secrets := backend.NewApplicationSecrets()
			Expect(secrets).ToNot(BeNil())

			secretMap, err := secrets.GetSecrets()
			Expect(err).ToNot(HaveOccurred())
			Expect(secretMap).To(HaveLen(2))
			Expect(secretMap).To(HaveKey("clientSecret"))
			Expect(secretMap).To(HaveKey("externalSecrets"))
			Expect(secretMap["clientSecret"].Value()).ToNot(BeEmpty())
			Expect(secretMap["externalSecrets"].Value()).To(Equal("{}"))
			Expect(secretMap["clientSecret"].AllowChange()).To(BeFalse())
			Expect(secretMap["externalSecrets"].AllowChange()).To(BeFalse())
		})
	})

	Context("TrySetSecret", func() {
		It("should return false when secrets map is nil", func() {
			secrets := &backend.Secrets{}
			success := secrets.TrySetSecret("key", backend.String("value"))
			Expect(success).To(BeFalse())
		})

		It("should return false for unknown secret", func() {
			secrets := backend.NewEnvironmentSecrets()
			success := secrets.TrySetSecret("unknown", backend.String("value"))
			Expect(success).To(BeFalse())
		})

		It("should set a simple secret value", func() {
			secrets := backend.NewEnvironmentSecrets()
			success := secrets.TrySetSecret("zones", backend.String("{\"region\": \"europe\"}"))
			Expect(success).To(BeTrue())

			// Verify the value was set
			secretMap, err := secrets.GetSecrets()
			Expect(err).ToNot(HaveOccurred())
			Expect(secretMap).To(HaveKey("zones"))
			Expect(secretMap["zones"].Value()).To(Equal("{\"region\": \"europe\"}"))
		})

		It("should set sub-secrets", func() {
			secrets := backend.NewEnvironmentSecrets()
			secrets.TrySetSecret("zones", backend.Empty()) // Reset to empty first

			// Set sub-secrets
			success := secrets.TrySetSecret("zones/region", backend.String("europe"))
			Expect(success).To(BeTrue())

			success = secrets.TrySetSecret("zones/datacenter", backend.String("frankfurt"))
			Expect(success).To(BeTrue())

			// Verify through GetSecrets
			secretMap, err := secrets.GetSecrets()
			Expect(err).ToNot(HaveOccurred())
			Expect(secretMap).To(HaveKey("zones"))

			// Check JSON content
			var parsed map[string]string
			err = json.Unmarshal([]byte(secretMap["zones"].Value()), &parsed)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed).To(HaveKeyWithValue("region", "europe"))
			Expect(parsed).To(HaveKeyWithValue("datacenter", "frankfurt"))
		})
	})

	Context("GetSecrets", func() {
		It("should return nil when secrets map is nil", func() {
			secrets := &backend.Secrets{}
			secretMap, err := secrets.GetSecrets()
			Expect(err).ToNot(HaveOccurred())
			Expect(secretMap).To(BeNil())
		})

		It("should return a copy of the secrets map", func() {
			secrets := backend.NewTeamSecrets()
			secretMap1, err := secrets.GetSecrets()
			Expect(err).ToNot(HaveOccurred())

			// Get a second copy
			secretMap2, err := secrets.GetSecrets()
			Expect(err).ToNot(HaveOccurred())

			// Verify both maps have the same content but are different instances
			Expect(secretMap1).To(HaveLen(2))
			Expect(secretMap2).To(HaveLen(2))
			Expect(secretMap1).To(HaveKey("clientSecret"))
			Expect(secretMap2).To(HaveKey("clientSecret"))

			// Since map addresses can't be reliably compared, verify that modifying one map doesn't affect the other
			modifiedMap := make(map[string]backend.SecretValue)
			maps.Copy(modifiedMap, secretMap1)
			modifiedMap["newKey"] = backend.String("newValue")

			// The second map shouldn't have the new key
			Expect(secretMap2).ToNot(HaveKey("newKey"))
		})

		It("should create JSON for sub-secrets", func() {
			// Create a test secrets object - use NewApplicationSecrets which has externalSecrets
			secrets := backend.NewApplicationSecrets()

			// Set empty value for externalSecrets
			success := secrets.TrySetSecret("externalSecrets", backend.Empty())
			Expect(success).To(BeTrue())

			// Set sub-secrets
			success = secrets.TrySetSecret("externalSecrets/key1", backend.String("value1"))
			Expect(success).To(BeTrue())
			success = secrets.TrySetSecret("externalSecrets/key2", backend.String("value2"))
			Expect(success).To(BeTrue())

			secretMap, err := secrets.GetSecrets()
			Expect(err).ToNot(HaveOccurred())
			Expect(secretMap).To(HaveKey("externalSecrets"))

			// Parse the JSON to verify content
			var parsed map[string]string
			err = json.Unmarshal([]byte(secretMap["externalSecrets"].Value()), &parsed)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed).To(HaveKeyWithValue("key1", "value1"))
			Expect(parsed).To(HaveKeyWithValue("key2", "value2"))
		})

		It("should fail if sub-secrets are set for non-empty secret", func() {
			// Create a test secrets object - use NewApplicationSecrets which has externalSecrets
			secrets := backend.NewApplicationSecrets()

			// Set a non-empty value for externalSecrets
			success := secrets.TrySetSecret("externalSecrets", backend.String("{\"existing\": \"value\"}"))
			Expect(success).To(BeTrue())

			// Attempt to set a sub-secret for a non-empty secret
			success = secrets.TrySetSecret("externalSecrets/key1", backend.String("value1"))
			Expect(success).To(BeTrue())

			_, err := secrets.GetSecrets()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot set sub-secrets for non-empty secret externalSecrets"))
		})
	})

	Context("TryAddSecrets", func() {

		var newFunc backend.NewFunc[*mocks.MockSecretId] = func(env, team, app, path, checksum string) *mocks.MockSecretId {
			mock := &mocks.MockSecretId{}
			mock.On("Env").Return(env)
			mock.On("Path").Return(path)
			mock.On("SubPath").Return(backend.GetSubPath(path))

			return mock
		}

		It("should add new secrets using the provided new-function", func() {
			allowedSecrets := backend.NewApplicationSecrets()
			err := backend.TryAddSecrets(newFunc, allowedSecrets, "test", "tes-team", "test-app", map[string]backend.SecretValue{
				"clientSecret":         backend.String("app-secret"),
				"externalSecrets/key1": backend.String("value1"),
			})

			Expect(err).ToNot(HaveOccurred())

			secrets, err := allowedSecrets.GetSecrets()
			Expect(err).ToNot(HaveOccurred())
			Expect(secrets).To(HaveKey("clientSecret"))
			Expect(secrets["clientSecret"].Value()).To(Equal("app-secret"))
			Expect(secrets).To(HaveKey("externalSecrets"))
			Expect(secrets["externalSecrets"].Value()).To(MatchJSON(`{"key1":"value1"}`))
		})

		It("should return an error when trying to add unknown secrets", func() {
			allowedSecrets := backend.NewApplicationSecrets()
			err := backend.TryAddSecrets(newFunc, allowedSecrets, "test", "tes-team", "test-app", map[string]backend.SecretValue{
				"unknownSecret": backend.String("value"),
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret unknownSecret is not allowed for onboarding"))
		})

	})

	Context("MergeSecretRefs", func() {

		var newFunc backend.NewFunc[*mocks.MockSecretId] = func(env, team, app, path, checksum string) *mocks.MockSecretId {
			mock := &mocks.MockSecretId{}
			mock.On("Env").Return(env)
			mock.On("Path").Return(path)
			mock.On("SubPath").Return(backend.GetSubPath(path))

			return mock
		}

		It("should merge secret-references with secrets using the provided new-function", func() {
			secretRefs := map[string]backend.SecretRef{
				"clientSecret":    mocks.NewMockSecretId(GinkgoT()),
				"externalSecrets": mocks.NewMockSecretId(GinkgoT()),
			}
			backend.MergeSecretRefs(newFunc, secretRefs, "test", "test-team", "test-app", map[string]backend.SecretValue{
				"clientSecret":         backend.String("app-secret"),
				"externalSecrets/key1": backend.String("value1"),
			})

			Expect(secretRefs).To(HaveKey("clientSecret"))
			Expect(secretRefs).To(HaveKey("externalSecrets"))
			Expect(secretRefs).To(HaveKey("externalSecrets/key1"))
			Expect(secretRefs).To(HaveLen(3))
		})
	})
})
