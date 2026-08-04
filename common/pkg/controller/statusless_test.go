// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"reflect"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// statuslessObject satisfies types.Object without exposing a Status field,
// mirroring pcp/v1.PermissionSet which has no status subresource.
type statuslessObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (o *statuslessObject) GetConditions() []metav1.Condition  { return nil }
func (o *statuslessObject) SetCondition(metav1.Condition) bool { return false }
func (o *statuslessObject) DeepCopyObject() runtime.Object     { return &statuslessObject{} }

var _ = Describe("status comparison", func() {
	It("treats a nil slice and an empty slice as equal", func() {
		// Status fields tagged omitempty round-trip as nil, so a handler that
		// assigns an empty slice would differ from the stored state forever.
		stored := test.TestResourceStatus{Conditions: nil}
		computed := test.TestResourceStatus{Conditions: []metav1.Condition{}}

		Expect(reflect.DeepEqual(stored, computed)).To(BeFalse(),
			"precondition: reflect.DeepEqual cannot be used here")
		Expect(apiequality.Semantic.DeepEqual(stored, computed)).To(BeTrue())
	})
})

var _ = Describe("statusValue", func() {
	It("reports no status for objects without a Status field", func() {
		value, ok := statusValue(&statuslessObject{})

		Expect(ok).To(BeFalse())
		Expect(value).To(BeNil())
	})

	It("returns the status for objects with a Status field", func() {
		obj := test.NewObject("statusless-probe", "default")
		obj.SetCondition(metav1.Condition{
			Type:   condition.ConditionTypeReady,
			Status: metav1.ConditionTrue,
			Reason: "Provisioned",
		})

		value, ok := statusValue(obj)

		Expect(ok).To(BeTrue())
		Expect(value).ToNot(BeNil())
	})
})
