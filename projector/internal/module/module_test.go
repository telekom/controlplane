// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package module

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type parentProcessor struct{ err error }

func (p parentProcessor) Upsert(context.Context, *corev1.ConfigMap) error { return p.err }
func (p parentProcessor) Delete(context.Context, types.NamespacedName, *corev1.ConfigMap) error {
	return p.err
}

// Compile-time check: TypedModule implements Module interface.
var _ Module = &TypedModule[*corev1.ConfigMap, any, string]{}

var _ = Describe("TypedModule", func() {
	Describe("Name", func() {
		It("returns the configured module name", func() {
			m := &TypedModule[*corev1.ConfigMap, any, string]{
				ModuleName: "test-resource",
			}
			Expect(m.Name()).To(Equal("test-resource"))
		})
	})
	It("publishes only completed parent projections and skips unavailable delete objects", func() {
		events := make(chan event.GenericEvent, 3)
		obj := &corev1.ConfigMap{}
		obj.Name = "parent"
		processor := notifyingProcessor[*corev1.ConfigMap]{SyncProcessor: parentProcessor{}, events: events}
		Expect(processor.Upsert(context.Background(), obj)).To(Succeed())
		Expect((<-events).Object.GetName()).To(Equal("parent"))
		Expect(processor.Delete(context.Background(), types.NamespacedName{Name: obj.Name}, obj)).To(Succeed())
		Expect((<-events).Object.GetName()).To(Equal("parent"))
		Expect(processor.Delete(context.Background(), types.NamespacedName{}, nil)).To(Succeed())
		Expect(events).To(BeEmpty())
		processor.SyncProcessor = parentProcessor{err: errors.New("projection failed")}
		Expect(processor.Upsert(context.Background(), obj)).To(MatchError("projection failed"))
		Expect(events).To(BeEmpty())
	})
})
