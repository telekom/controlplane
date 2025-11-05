// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNotification(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notification Util Suite")
}

var _ = Describe("Notification Utilities", func() {
	Describe("extractRequester", func() {
		Context("when requester has properties with scopes and basePath", func() {
			It("should extract all properties including scopes array and basePath", func() {
				requesterProperties := map[string]any{
					"basePath": "foo/bar/myapi/v1",
					"scopes":   []string{"admin:read", "admin:write"},
					"email":    "user@example.com",
				}

				propertiesJSON, err := json.Marshal(requesterProperties)
				Expect(err).NotTo(HaveOccurred())

				requester := &approvalv1.Requester{
					Name:       "platform--backend",
					Email:      "team@example.com",
					Reason:     "Need access",
					Properties: runtime.RawExtension{Raw: propertiesJSON},
				}

				result, err := extractRequester(requester)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveKeyWithValue("basePath", "foo/bar/myapi/v1"))
				Expect(result).To(HaveKeyWithValue("scopes", []any{"admin:read", "admin:write"}))
				Expect(result).To(HaveKeyWithValue("email", "user@example.com"))
				Expect(result).To(HaveKeyWithValue("requester_group", "platform"))
				Expect(result).To(HaveKeyWithValue("requester_team", "backend"))
			})
		})

		Context("when requester name contains group and team", func() {
			It("should extract group and team from name", func() {
				requester := &approvalv1.Requester{
					Name:  "onsite-group--enemy-team",
					Email: "team@example.com",
				}

				result, err := extractRequester(requester)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveKeyWithValue("requester_group", "onsite-group"))
				Expect(result).To(HaveKeyWithValue("requester_team", "enemy-team"))
			})
		})

		Context("when requester name does not contain separator", func() {
			It("should use name for both group and team", func() {
				requester := &approvalv1.Requester{
					Name:  "single-name",
					Email: "team@example.com",
				}

				result, err := extractRequester(requester)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveKeyWithValue("requester_group", "single-name"))
				Expect(result).To(HaveKeyWithValue("requester_team", "single-name"))
			})
		})

		Context("when requester has complex nested properties", func() {
			It("should preserve all nested structures", func() {
				requesterProperties := map[string]any{
					"basePath": "api/v2/users",
					"scopes":   []string{"read", "write", "delete"},
					"metadata": map[string]any{
						"requestedBy": "john.doe",
						"department":  "engineering",
					},
					"limits": map[string]any{
						"rateLimit": 1000,
						"quota":     50000,
					},
				}

				propertiesJSON, err := json.Marshal(requesterProperties)
				Expect(err).NotTo(HaveOccurred())

				requester := &approvalv1.Requester{
					Name:       "platform--frontend",
					Properties: runtime.RawExtension{Raw: propertiesJSON},
				}

				result, err := extractRequester(requester)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveKeyWithValue("basePath", "api/v2/users"))
				Expect(result).To(HaveKey("scopes"))
				Expect(result).To(HaveKey("metadata"))
				Expect(result).To(HaveKey("limits"))

				scopes, ok := result["scopes"].([]any)
				Expect(ok).To(BeTrue())
				Expect(scopes).To(HaveLen(3))
				Expect(scopes).To(ContainElements("read", "write", "delete"))
			})
		})

		Context("when requester has empty properties", func() {
			It("should still extract group and team", func() {
				requester := &approvalv1.Requester{
					Name:       "foo--bar",
					Properties: runtime.RawExtension{Raw: []byte("{}")},
				}

				result, err := extractRequester(requester)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveKeyWithValue("requester_group", "foo"))
				Expect(result).To(HaveKeyWithValue("requester_team", "bar"))
			})
		})
	})

	Describe("extractTarget", func() {
		Context("when target has valid structure", func() {
			It("should extract target information", func() {
				target := &types.TypedObjectRef{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ApiSubscription",
						APIVersion: "api.cp.ei.telekom.de/v1",
					},
					ObjectRef: types.ObjectRef{
						Name:      "my-app--foo-bar-v1",
						Namespace: "prod--platform--backend",
					},
				}

				result, kind, name := extractTarget(target)

				Expect(kind).To(Equal("ApiSubscription"))
				Expect(name).To(Equal("my-app--foo-bar-v1"))
				Expect(result).To(HaveKeyWithValue("target_kind", "ApiSubscription"))
				Expect(result).To(HaveKeyWithValue("target_application", "my-app"))
				Expect(result).To(HaveKeyWithValue("target_group", "platform"))
				Expect(result).To(HaveKeyWithValue("target_team", "backend"))
			})
		})

		Context("when target is nil", func() {
			It("should return empty properties", func() {
				result, kind, name := extractTarget(nil)

				Expect(kind).To(BeEmpty())
				Expect(name).To(BeEmpty())
				Expect(result).To(BeEmpty())
			})
		})

		Context("when target has simple name without basepath", func() {
			It("should use name as application", func() {
				target := &types.TypedObjectRef{
					TypeMeta: metav1.TypeMeta{
						Kind: "EventSubscription",
					},
					ObjectRef: types.ObjectRef{
						Name:      "simple-name",
						Namespace: "dev--group--team",
					},
				}

				result, kind, name := extractTarget(target)

				Expect(kind).To(Equal("EventSubscription"))
				Expect(name).To(Equal("simple-name"))
				Expect(result).To(HaveKeyWithValue("target_application", "simple-name"))
			})
		})
	})
})
