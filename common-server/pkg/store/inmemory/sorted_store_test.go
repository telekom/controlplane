package inmemory

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/common-server/pkg/store"
)

var _ = Describe("Sorted Store", func() {

	ctx := context.Background()

	Context("List sorted results", Ordered, func() {

		sortedStore := Sortable(
			&InmemoryObjectStore[*unstructured.Unstructured]{
				ctx: ctx,
				db:  newDbOrDie(logr.Discard()),
				log: logr.Discard(),
			},
			StoreOpts{
				AllowedSorts: []string{"metadata.name", "metadata.labels.app", "spec.replicas", "spec.timeout"},
			},
		).(*SortableStore[*unstructured.Unstructured])

		BeforeAll(func() {
			for _, u := range GenerateUnstructured(1000) {
				Expect(sortedStore.OnUpdate(ctx, u)).To(Succeed())
			}
		})

		It("Should order asc (string)", func() {
			orderedList, err := sortedStore.List(ctx, store.ListOpts{
				Limit: 100,
				Sorters: []store.Sorter{
					{
						Path:  "metadata.name",
						Order: store.SortOrderAsc,
					},
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(orderedList.Items).To(HaveLen(100))

			for i := 1; i < len(orderedList.Items); i++ {
				prevName, _, _ := unstructured.NestedString(orderedList.Items[i-1].Object, "metadata", "name")
				currName, _, _ := unstructured.NestedString(orderedList.Items[i].Object, "metadata", "name")
				Expect(prevName <= currName).To(BeTrue(), fmt.Sprintf("List is not sorted: %s > %s", prevName, currName))
			}
		})

		It("Should order desc (string)", func() {
			orderedList, err := sortedStore.List(ctx, store.ListOpts{
				Limit: 100,
				Sorters: []store.Sorter{
					{
						Path:  "metadata.labels.app",
						Order: store.SortOrderDesc,
					},
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(orderedList.Items).To(HaveLen(100))

			for i := 1; i < len(orderedList.Items); i++ {
				prevApp, _, _ := unstructured.NestedString(orderedList.Items[i-1].Object, "metadata", "labels", "app")
				currApp, _, _ := unstructured.NestedString(orderedList.Items[i].Object, "metadata", "labels", "app")
				Expect(prevApp >= currApp).To(BeTrue(), fmt.Sprintf("List is not sorted: %s < %s", prevApp, currApp))
			}

		})

		It("Should order asc (int)", func() {
			orderedList, err := sortedStore.List(ctx, store.ListOpts{
				Limit: 100,
				Sorters: []store.Sorter{
					{
						Path:  "spec.replicas",
						Order: store.SortOrderDesc,
					},
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(orderedList.Items).To(HaveLen(100))

			for i := 1; i < len(orderedList.Items); i++ {
				prevReplicas, _, _ := unstructured.NestedInt64(orderedList.Items[i-1].Object, "spec", "replicas")
				currReplicas, _, _ := unstructured.NestedInt64(orderedList.Items[i].Object, "spec", "replicas")
				Expect(prevReplicas >= currReplicas).To(BeTrue(), fmt.Sprintf("List is not sorted: %d > %d", prevReplicas, currReplicas))
			}

		})

		It("Should order asc (float)", func() {
			orderedList, err := sortedStore.List(ctx, store.ListOpts{
				Limit: 100,
				Sorters: []store.Sorter{
					{
						Path:  "spec.timeout",
						Order: store.SortOrderDesc,
					},
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(orderedList.Items).To(HaveLen(100))

			for i := 1; i < len(orderedList.Items); i++ {
				prevTimeout, _, _ := unstructured.NestedFloat64(orderedList.Items[i-1].Object, "spec", "timeout")
				currTimeout, _, _ := unstructured.NestedFloat64(orderedList.Items[i].Object, "spec", "timeout")
				Expect(prevTimeout >= currTimeout).To(BeTrue(), fmt.Sprintf("List is not sorted: %f < %f", prevTimeout, currTimeout))
			}

		})

	})
})

func BenchmarkStore(b *testing.B) {
	ctx := context.Background()

	s := Sortable(
		&InmemoryObjectStore[*unstructured.Unstructured]{
			ctx: ctx,
			db:  newDbOrDie(logr.Discard()),
			log: logr.Discard(),
		},
		StoreOpts{
			AllowedSorts: []string{"metadata.name", "metadata.labels.app"},
		},
	).(*SortableStore[*unstructured.Unstructured])

	for i := 0; i < b.N; i++ {
		_ = s.OnUpdate(ctx, NewUnstructured(fmt.Sprintf("item-%d", i)))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = s.List(ctx, store.ListOpts{
			Sorters: []store.Sorter{
				{
					Path:  "metadata.name",
					Order: store.SortOrderAsc,
				},
			},
		})
	}

}
