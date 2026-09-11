// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure_test

import (
	"sync"

	"github.com/telekom/controlplane/projector/internal/infrastructure"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("DeleteCache", func() {
	var cache *infrastructure.DeleteCache

	BeforeEach(func() {
		cache = &infrastructure.DeleteCache{}
	})

	It("stores and loads an object", func() {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cm-1", Namespace: "ns-1"},
		}
		cache.Store(obj)

		key := types.NamespacedName{Name: "cm-1", Namespace: "ns-1"}
		loaded := cache.LoadAndDelete(key)
		Expect(loaded).NotTo(BeNil())
		Expect(loaded.GetName()).To(Equal("cm-1"))
		Expect(loaded.GetNamespace()).To(Equal("ns-1"))
	})

	It("returns nil on miss", func() {
		key := types.NamespacedName{Name: "nonexistent", Namespace: "ns"}
		loaded := cache.LoadAndDelete(key)
		Expect(loaded).To(BeNil())
	})

	It("removes entry after LoadAndDelete", func() {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cm-2", Namespace: "ns"},
		}
		cache.Store(obj)

		key := types.NamespacedName{Name: "cm-2", Namespace: "ns"}
		first := cache.LoadAndDelete(key)
		Expect(first).NotTo(BeNil())

		second := cache.LoadAndDelete(key)
		Expect(second).To(BeNil())
	})

	It("handles concurrent access without race conditions", func() {
		const goroutines = 100
		var wg sync.WaitGroup
		wg.Add(goroutines * 2) // half store, half load

		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				obj := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "concurrent",
						Namespace: "ns",
					},
				}
				cache.Store(obj)
			}()

			go func() {
				defer wg.Done()
				key := types.NamespacedName{Name: "concurrent", Namespace: "ns"}
				cache.LoadAndDelete(key) // may or may not find an entry
			}()
		}

		wg.Wait()
		// No panic = success. The test validates race-safety.
	})

	It("stores objects with different keys independently", func() {
		obj1 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		}
		obj2 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"},
		}
		cache.Store(obj1)
		cache.Store(obj2)

		loaded1 := cache.LoadAndDelete(types.NamespacedName{Name: "a", Namespace: "ns"})
		loaded2 := cache.LoadAndDelete(types.NamespacedName{Name: "b", Namespace: "ns"})
		Expect(loaded1).NotTo(BeNil())
		Expect(loaded1.GetName()).To(Equal("a"))
		Expect(loaded2).NotTo(BeNil())
		Expect(loaded2.GetName()).To(Equal("b"))
	})
})
