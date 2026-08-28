// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	commontypes "github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/filesubscription"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

var _ = Describe("FileSubscription Translator", func() {
	var t filesubscription.Translator

	Describe("ShouldSkip", func() {
		It("should never skip", func() {
			skip, reason := t.ShouldSkip(&filev1.FileSubscription{})
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should map all relevant fields", func() {
			obj := &filev1.FileSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sub-a",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "prod",
						"cp.ei.telekom.de/application": "consumer-app",
					},
				},
				Spec: filev1.FileSubscriptionSpec{
					FileType: "invoice",
					Zone: &commontypes.ObjectRef{
						Name:      "caas",
						Namespace: "zone-ns",
					},
					SFTP: &filev1.FileSFTP{PublicKeys: []filev1.SSHPublicKeySpec{{Key: "ssh-ed25519 AAA"}}},
				},
				Status: filev1.FileSubscriptionStatus{
					Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "ok"}},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.TargetFileType).To(Equal("invoice"))
			Expect(data.OwnerAppName).To(Equal("consumer-app"))
			Expect(data.OwnerTeamName).To(Equal("platform--narvi"))
			Expect(data.Zone).To(Equal("caas"))
			Expect(data.SFTPPublicKeys).To(Equal([]string{"ssh-ed25519 AAA"}))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.StatusMessage).To(Equal("ok"))
			Expect(data.Meta.Environment).To(Equal("prod"))
		})

		It("should handle nil zone and nil sftp", func() {
			obj := &filev1.FileSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sub-b",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "consumer-app"},
				},
				Spec: filev1.FileSubscriptionSpec{FileType: "orders"},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Zone).To(BeEmpty())
			Expect(data.SFTPPublicKeys).To(BeEmpty())
			Expect(data.StatusPhase).To(Equal("UNKNOWN"))
		})
	})

	Describe("KeyFromObject", func() {
		It("should derive key from object metadata/spec", func() {
			obj := &filev1.FileSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sub-a",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "consumer-app"},
				},
				Spec: filev1.FileSubscriptionSpec{FileType: "invoice"},
			}
			key := t.KeyFromObject(obj)
			Expect(key.FileType).To(Equal("invoice"))
			Expect(key.OwnerAppName).To(Equal("consumer-app"))
			Expect(key.OwnerTeamName).To(Equal("platform--narvi"))
			Expect(key.Namespace).To(Equal("prod--platform--narvi"))
			Expect(key.Name).To(Equal("sub-a"))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should prefer lastKnown", func() {
			req := k8stypes.NamespacedName{Namespace: "prod--platform--narvi", Name: "sub-a"}
			lastKnown := &filev1.FileSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-a", Namespace: "prod--platform--narvi", Labels: map[string]string{"cp.ei.telekom.de/application": "consumer-app"}},
				Spec:       filev1.FileSubscriptionSpec{FileType: "invoice"},
			}
			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.FileType).To(Equal("invoice"))
			Expect(key.OwnerAppName).To(Equal("consumer-app"))
		})

		It("should fallback to request values when lastKnown is nil", func() {
			req := k8stypes.NamespacedName{Namespace: "prod--platform--narvi", Name: "sub-a"}
			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.FileType).To(Equal("sub-a"))
			Expect(key.OwnerAppName).To(Equal("sub-a"))
			Expect(key.OwnerTeamName).To(Equal("platform--narvi"))
		})
	})
})
