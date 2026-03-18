// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"encoding/json"
	"testing"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
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
					TeamName:   "platform--backend",
					TeamEmail:  "team@example.com",
					Reason:     "Need access",
					Properties: runtime.RawExtension{Raw: propertiesJSON},
					ApplicationRef: &ctypes.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "application.cp.ei.telekom.de/v1",
							APIVersion: "Application",
						},
						ObjectRef: ctypes.ObjectRef{
							Name:      "requester-app-name",
							Namespace: "default",
						},
					},
				}

				result, err := extractRequester(requester)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveKeyWithValue("basePath", "foo/bar/myapi/v1"))
				Expect(result).To(HaveKeyWithValue("scopes", []any{"admin:read", "admin:write"}))
				Expect(result).To(HaveKeyWithValue("email", "user@example.com"))
				Expect(result).To(HaveKeyWithValue("requester_group", "platform"))
				Expect(result).To(HaveKeyWithValue("requester_team", "backend"))
				Expect(result).To(HaveKeyWithValue("resource_name", "foo/bar/myapi/v1"))
				Expect(result).To(HaveKeyWithValue("resource_type", "API"))
			})
		})

		Context("when requester name contains group and team", func() {
			It("should extract group and team from name", func() {
				requester := &approvalv1.Requester{
					TeamName:  "onsite-group--enemy-team",
					TeamEmail: "team@example.com",
					ApplicationRef: &ctypes.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "application.cp.ei.telekom.de/v1",
							APIVersion: "Application",
						},
						ObjectRef: ctypes.ObjectRef{
							Name:      "requester-app-name",
							Namespace: "default",
						},
					},
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
					TeamName:  "single-name",
					TeamEmail: "team@example.com",
					ApplicationRef: &ctypes.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "application.cp.ei.telekom.de/v1",
							APIVersion: "Application",
						},
						ObjectRef: ctypes.ObjectRef{
							Name:      "requester-app-name",
							Namespace: "default",
						},
					},
				}

				_, err := extractRequester(requester)

				Expect(err).To(HaveOccurred())
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
					TeamName:   "platform--frontend",
					Properties: runtime.RawExtension{Raw: propertiesJSON},
					ApplicationRef: &ctypes.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "application.cp.ei.telekom.de/v1",
							APIVersion: "Application",
						},
						ObjectRef: ctypes.ObjectRef{
							Name:      "requester-app-name",
							Namespace: "default",
						},
					},
				}

				result, err := extractRequester(requester)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveKeyWithValue("basePath", "api/v2/users"))
				Expect(result).To(HaveKey("scopes"))
				Expect(result).To(HaveKey("metadata"))
				Expect(result).To(HaveKey("limits"))
				Expect(result).To(HaveKeyWithValue("resource_name", "api/v2/users"))
				Expect(result).To(HaveKeyWithValue("resource_type", "API"))

				scopes, ok := result["scopes"].([]any)
				Expect(ok).To(BeTrue())
				Expect(scopes).To(HaveLen(3))
				Expect(scopes).To(ContainElements("read", "write", "delete"))
			})
		})

		Context("when requester has empty properties", func() {
			It("should still extract group and team", func() {
				requester := &approvalv1.Requester{
					TeamName:   "foo--bar",
					Properties: runtime.RawExtension{Raw: []byte("{}")},
					ApplicationRef: &ctypes.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "application.cp.ei.telekom.de/v1",
							APIVersion: "Application",
						},
						ObjectRef: ctypes.ObjectRef{
							Name:      "requester-app-name",
							Namespace: "default",
						},
					},
				}

				result, err := extractRequester(requester)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveKeyWithValue("requester_group", "foo"))
				Expect(result).To(HaveKeyWithValue("requester_team", "bar"))
			})
		})

		Context("when requester has event subscription", func() {
			It("should still extract group and team", func() {

				requesterProperties := map[string]any{
					"eventType": "some-event-type",
					"scopes":    []string{"read", "write", "delete"},
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
					TeamName:   "foo--bar",
					Properties: runtime.RawExtension{Raw: propertiesJSON},
					ApplicationRef: &ctypes.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "application.cp.ei.telekom.de/v1",
							APIVersion: "Application",
						},
						ObjectRef: ctypes.ObjectRef{
							Name:      "requester-app-name",
							Namespace: "default",
						},
					},
				}

				result, err := extractRequester(requester)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveKeyWithValue("requester_group", "foo"))
				Expect(result).To(HaveKeyWithValue("requester_team", "bar"))
				Expect(result).To(HaveKeyWithValue("resource_name", "some-event-type"))
				Expect(result).To(HaveKeyWithValue("resource_type", "event"))
			})
		})
	})

	DescribeTable("should normalize subscription kinds",
		func(kind string, want string) {
			got := extractTargetKind(kind)
			Expect(got).To(Equal(want))
		},
		Entry("api subscription lower case", "apisubscription", "subscription"),
		Entry("api subscription mixed case", "ApiSubscription", "subscription"),
		Entry("event subscription lower case", "eventsubscription", "subscription"),
		Entry("event subscription upper case", "EVENTSUBSCRIPTION", "subscription"),
		Entry("other kind unchanged", "SomeOtherKind", "SomeOtherKind"),
	)
})
