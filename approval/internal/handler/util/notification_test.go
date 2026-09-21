// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktypes "k8s.io/apimachinery/pkg/types"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
					"basePath":      "foo/bar/myapi/v1",
					"scopes":        []string{"admin:read", "admin:write"},
					"email":         "user@example.com",
					"resource_type": "API",
					"resource_name": "foo/bar/myapi/v1",
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
					"resource_type": "API",
					"resource_name": "api/v2/users",
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
					"resource_type": "event",
					"resource_name": "some-event-type",
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

	Describe("scopedNotificationBaseName", func() {
		makeOwner := func(kind, group, ns, name string, uid ktypes.UID) *approvalv1.ApprovalRequest {
			ar := &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: ns,
					UID:       uid,
				},
			}
			ar.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   group,
				Version: "v1",
				Kind:    kind,
			})
			return ar
		}

		It("produces an-v1- prefix with 48 hex chars", func() {
			owner := makeOwner("ApprovalRequest", "approval.cp.ei.telekom.de", "env--grp--team", "ar-test", "uid-1")
			name, err := scopedNotificationBaseName(owner, "provider", "approvalrequest--subscribe--created--decider")
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(HavePrefix("an-v1-"))
			Expect(name).To(HaveLen(6 + 48)) // prefix + digest
		})

		It("is deterministic — same inputs produce same output", func() {
			owner := makeOwner("ApprovalRequest", "approval.cp.ei.telekom.de", "env--grp--team", "ar-idem", "uid-2")
			name1, err := scopedNotificationBaseName(owner, "provider", "approvalrequest--subscribe--created--decider")
			Expect(err).NotTo(HaveOccurred())
			name2, err := scopedNotificationBaseName(owner, "provider", "approvalrequest--subscribe--created--decider")
			Expect(err).NotTo(HaveOccurred())
			Expect(name1).To(Equal(name2))
		})

		It("different approval keys produce different names", func() {
			owner := makeOwner("ApprovalRequest", "approval.cp.ei.telekom.de", "env--grp--team", "ar-keys", "uid-3")
			nameP, err := scopedNotificationBaseName(owner, "provider", "approvalrequest--subscribe--created--decider")
			Expect(err).NotTo(HaveOccurred())
			nameC, err := scopedNotificationBaseName(owner, "consumer", "approvalrequest--subscribe--created--decider")
			Expect(err).NotTo(HaveOccurred())
			Expect(nameP).NotTo(Equal(nameC))
		})

		It("different UIDs produce different names", func() {
			ownerA := makeOwner("ApprovalRequest", "approval.cp.ei.telekom.de", "env--grp--team", "ar-same", "uid-a")
			ownerB := makeOwner("ApprovalRequest", "approval.cp.ei.telekom.de", "env--grp--team", "ar-same", "uid-b")
			nameA, err := scopedNotificationBaseName(ownerA, "provider", "approvalrequest--subscribe--created--decider")
			Expect(err).NotTo(HaveOccurred())
			nameB, err := scopedNotificationBaseName(ownerB, "provider", "approvalrequest--subscribe--created--decider")
			Expect(err).NotTo(HaveOccurred())
			Expect(nameA).NotTo(Equal(nameB))
		})

		It("AR vs Approval source separation — different kinds produce different names", func() {
			ownerAR := makeOwner("ApprovalRequest", "approval.cp.ei.telekom.de", "env--grp--team", "obj", "uid-same")
			ownerA := makeOwner("Approval", "approval.cp.ei.telekom.de", "env--grp--team", "obj", "uid-same")
			nameAR, err := scopedNotificationBaseName(ownerAR, "provider", "approvalrequest--subscribe--created--decider")
			Expect(err).NotTo(HaveOccurred())
			nameA, err := scopedNotificationBaseName(ownerA, "provider", "approval--subscribe--updated--decider")
			Expect(err).NotTo(HaveOccurred())
			Expect(nameAR).NotTo(Equal(nameA))
		})

		It("errors when owner UID is empty", func() {
			owner := makeOwner("ApprovalRequest", "approval.cp.ei.telekom.de", "env--grp--team", "ar-no-uid", "")
			_, err := scopedNotificationBaseName(owner, "provider", "some-purpose")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("owner UID must not be empty"))
		})

		It("name fits within Kubernetes name length limit", func() {
			owner := makeOwner("ApprovalRequest", "approval.cp.ei.telekom.de", "env--grp--team", "ar-long", "uid-long")
			name, err := scopedNotificationBaseName(owner, "provider", "approvalrequest--subscribe--created--decider")
			Expect(err).NotTo(HaveOccurred())
			Expect(len(name)).To(BeNumerically("<=", 253))
		})
	})
})
