// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AppendUniqueRef", func() {
	refA := ctypes.ObjectRef{Namespace: "ns", Name: "a"}
	refB := ctypes.ObjectRef{Namespace: "ns", Name: "b"}

	It("appends to a nil list", func() {
		Expect(AppendUniqueRef(nil, refA)).To(Equal([]ctypes.ObjectRef{refA}))
	})

	It("appends a ref that is not present yet", func() {
		Expect(AppendUniqueRef([]ctypes.ObjectRef{refA}, refB)).To(Equal([]ctypes.ObjectRef{refA, refB}))
	})

	It("does not append a duplicate Namespace/Name pair", func() {
		refs := []ctypes.ObjectRef{refA, refB}

		Expect(AppendUniqueRef(refs, refA)).To(Equal(refs))
		Expect(AppendUniqueRef(refs, refB)).To(Equal(refs))
	})

	It("ignores the UID when comparing", func() {
		refs := []ctypes.ObjectRef{refA}
		sameNameOtherUID := ctypes.ObjectRef{Namespace: "ns", Name: "a", UID: "other-uid"}

		Expect(AppendUniqueRef(refs, sameNameOtherUID)).To(Equal(refs))
	})

	It("treats the same name in a different namespace as distinct", func() {
		other := ctypes.ObjectRef{Namespace: "other-ns", Name: "a"}

		Expect(AppendUniqueRef([]ctypes.ObjectRef{refA}, other)).To(HaveLen(2))
	})

	It("preserves the existing order", func() {
		refs := []ctypes.ObjectRef{refB, refA}
		newRef := ctypes.ObjectRef{Namespace: "ns", Name: "c"}

		Expect(AppendUniqueRef(refs, newRef)).To(Equal([]ctypes.ObjectRef{refB, refA, newRef}))
	})
})

var _ = Describe("DedupRefs", func() {
	refA := ctypes.ObjectRef{Namespace: "ns", Name: "a"}
	refB := ctypes.ObjectRef{Namespace: "ns", Name: "b"}
	refC := ctypes.ObjectRef{Namespace: "ns", Name: "c"}

	It("returns nil for nil", func() {
		Expect(DedupRefs(nil)).To(BeNil())
	})

	It("returns an empty list for an empty list", func() {
		Expect(DedupRefs([]ctypes.ObjectRef{})).To(BeEmpty())
	})

	It("returns the input unchanged when there are no duplicates", func() {
		refs := []ctypes.ObjectRef{refA, refB, refC}

		deduped := DedupRefs(refs)

		Expect(deduped).To(Equal(refs))
		Expect(&deduped[0]).To(BeIdenticalTo(&refs[0]), "must not allocate a new slice")
	})

	It("removes duplicates, keeping the first occurrence and the order", func() {
		refs := []ctypes.ObjectRef{refA, refB, refA, refC, refB}

		Expect(DedupRefs(refs)).To(Equal([]ctypes.ObjectRef{refA, refB, refC}))
	})

	It("treats the same name in a different namespace as distinct", func() {
		other := ctypes.ObjectRef{Namespace: "other-ns", Name: "a"}
		refs := []ctypes.ObjectRef{refA, other, refA}

		Expect(DedupRefs(refs)).To(Equal([]ctypes.ObjectRef{refA, other}))
	})

	It("ignores the UID when comparing", func() {
		sameNameOtherUID := ctypes.ObjectRef{Namespace: "ns", Name: "a", UID: "other-uid"}
		refs := []ctypes.ObjectRef{refA, sameNameOtherUID}

		Expect(DedupRefs(refs)).To(Equal([]ctypes.ObjectRef{refA}))
	})

	It("is idempotent", func() {
		refs := []ctypes.ObjectRef{refA, refB, refA, refC, refB, refC}

		once := DedupRefs(refs)

		Expect(DedupRefs(once)).To(Equal(once))
	})
})
