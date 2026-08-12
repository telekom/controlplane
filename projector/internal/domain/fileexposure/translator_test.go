// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fileexposure_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	commontypes "github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/fileexposure"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

var _ = Describe("FileExposure Translator", func() {
	var t fileexposure.Translator

	Describe("ShouldSkip", func() {
		It("should never skip", func() {
			skip, reason := t.ShouldSkip(&filev1.FileExposure{})
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should map provider, zone, approval, sftp keys and status", func() {
			obj := &filev1.FileExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exp-a",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "prod",
						"cp.ei.telekom.de/application": "label-app",
					},
				},
				Spec: filev1.FileExposureSpec{
					Provider: "provider-app",
					FileType: "invoice",
					Zone: &commontypes.ObjectRef{
						Name:      "caas",
						Namespace: "zone-ns",
					},
					SFTP:       &filev1.FileSFTP{PublicKeys: []filev1.SSHPublicKeySpec{{Key: "ssh-rsa AAA"}, {Key: "ssh-rsa BBB"}}},
					Visibility: filev1.VisibilityEnterprise,
					Approval: filev1.Approval{
						Strategy:     filev1.ApprovalStrategyFourEyes,
						TrustedTeams: []string{"team-a"},
					},
				},
				Status: filev1.FileExposureStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "ok"}}},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.TargetFileType).To(Equal("invoice"))
			Expect(data.Provider).NotTo(BeNil())
			Expect(*data.Provider).To(Equal("provider-app"))
			Expect(data.AppName).To(Equal("provider-app"))
			Expect(data.TeamName).To(Equal("platform--narvi"))
			Expect(data.Visibility).To(Equal("ENTERPRISE"))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.StatusMessage).To(Equal("ok"))
			Expect(data.Meta.Environment).To(Equal("prod"))
			Expect(data.ZoneName).To(Equal("caas"))
			Expect(data.ZoneNamespace).NotTo(BeNil())
			Expect(*data.ZoneNamespace).To(Equal("zone-ns"))
			Expect(data.SFTPPublicKeys).To(Equal([]string{"ssh-rsa AAA", "ssh-rsa BBB"}))
			Expect(data.ApprovalConfig.Strategy).To(Equal("FOUR_EYES"))
			Expect(data.ApprovalConfig.TrustedTeams).To(Equal([]string{"team-a"}))
			Expect(data.Active).To(BeTrue())
		})

		It("should fallback owner app from labels and mark inactive on already-exists reason", func() {
			obj := &filev1.FileExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exp-b",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "label-app"},
				},
				Spec: filev1.FileExposureSpec{
					FileType:   "orders",
					Zone:       &commontypes.ObjectRef{Name: "caas"},
					Visibility: filev1.VisibilityWorld,
				},
				Status: filev1.FileExposureStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionFalse, Reason: "FileExposureAlreadyExists", Message: "duplicate"}}},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.AppName).To(Equal("label-app"))
			Expect(data.Provider).To(BeNil())
			Expect(data.Active).To(BeFalse())
			Expect(data.Visibility).To(Equal("WORLD"))
		})
	})

	Describe("KeyFromObject", func() {
		It("should derive key from object metadata/spec", func() {
			obj := &filev1.FileExposure{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "my-app"},
				},
				Spec: filev1.FileExposureSpec{FileType: "invoice"},
			}
			key := t.KeyFromObject(obj)
			Expect(key.FileType).To(Equal("invoice"))
			Expect(key.AppName).To(Equal("my-app"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should prefer lastKnown", func() {
			req := k8stypes.NamespacedName{Namespace: "prod--platform--narvi", Name: "exp"}
			lastKnown := &filev1.FileExposure{
				ObjectMeta: metav1.ObjectMeta{Namespace: "prod--platform--narvi", Labels: map[string]string{"cp.ei.telekom.de/application": "my-app"}},
				Spec:       filev1.FileExposureSpec{FileType: "invoice"},
			}
			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.FileType).To(Equal("invoice"))
			Expect(key.AppName).To(Equal("my-app"))
		})

		It("should fallback to request values when lastKnown is nil", func() {
			req := k8stypes.NamespacedName{Namespace: "prod--platform--narvi", Name: "exp-name"}
			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.FileType).To(Equal("exp-name"))
			Expect(key.AppName).To(Equal("exp-name"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})
	})
})
