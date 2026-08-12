// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	commontypes "github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/filetype"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

var _ = Describe("FileType Translator", func() {
	var t filetype.Translator

	Describe("ShouldSkip", func() {
		It("should never skip", func() {
			skip, reason := t.ShouldSkip(&filev1.FileType{})
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should map all status/spec fields", func() {
			obj := &filev1.FileType{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invoice",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/environment": "prod"},
				},
				Spec: filev1.FileTypeSpec{Description: "Invoice files"},
				Status: filev1.FileTypeStatus{
					FileExposureRef: &commontypes.ObjectRef{Name: "exp-invoice"},
					SFTPInstance:    &commontypes.ObjectRef{Name: "sftp-a", Namespace: "ns-a"},
					Conditions:      []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "ok"}},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.FileType).To(Equal("invoice"))
			Expect(data.Description).To(Equal("Invoice files"))
			Expect(data.TeamName).To(Equal("platform--narvi"))
			Expect(data.Meta.Environment).To(Equal("prod"))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.StatusMessage).To(Equal("ok"))
			Expect(data.Active).To(BeTrue())
			Expect(data.SFTPInstanceName).NotTo(BeNil())
			Expect(*data.SFTPInstanceName).To(Equal("sftp-a"))
			Expect(data.SFTPInstanceNamespace).NotTo(BeNil())
			Expect(*data.SFTPInstanceNamespace).To(Equal("ns-a"))
			Expect(data.Variant).To(BeNil())
		})

		It("should set active=false when no exposure ref", func() {
			obj := &filev1.FileType{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "prod--platform--narvi"},
				Spec:       filev1.FileTypeSpec{Description: "Orders"},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Active).To(BeFalse())
			Expect(data.SFTPInstanceName).To(BeNil())
			Expect(data.SFTPInstanceNamespace).To(BeNil())
			Expect(data.StatusPhase).To(Equal("UNKNOWN"))
		})
	})

	Describe("KeyFromObject", func() {
		It("should derive key from object name", func() {
			key := t.KeyFromObject(&filev1.FileType{ObjectMeta: metav1.ObjectMeta{Name: "invoice"}})
			Expect(key.FileType).To(Equal("invoice"))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should use lastKnown when available", func() {
			req := k8stypes.NamespacedName{Name: "req-name"}
			lastKnown := &filev1.FileType{ObjectMeta: metav1.ObjectMeta{Name: "last-known"}}
			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.FileType).To(Equal("last-known"))
		})

		It("should fallback to request name when lastKnown is nil", func() {
			key, err := t.KeyFromDelete(k8stypes.NamespacedName{Name: "req-name"}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.FileType).To(Equal("req-name"))
		})
	})
})
