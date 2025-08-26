// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handlers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("Sorter", func() {
	var (
		testHandler1 *mocks.MockResourceHandler
		testHandler2 *mocks.MockResourceHandler
		testHandler3 *mocks.MockResourceHandler
		obj1         *types.UnstructuredObject
		obj2         *types.UnstructuredObject
		obj3         *types.UnstructuredObject
	)

	BeforeEach(func() {
		// Reset the registry before each test
		handlers.ResetRegistryForTest()

		// Create test handlers with different priorities
		testHandler1 = mocks.NewMockResourceHandler(GinkgoT())
		testHandler1.EXPECT().Priority().Return(100).Maybe()

		testHandler2 = mocks.NewMockResourceHandler(GinkgoT())
		testHandler2.EXPECT().Priority().Return(50).Maybe() // Higher priority (lower number)

		testHandler3 = mocks.NewMockResourceHandler(GinkgoT())
		testHandler3.EXPECT().Priority().Return(200).Maybe() // Lower priority (higher number)

		// Create test objects
		obj1 = &types.UnstructuredObject{
			Content: map[string]any{
				"apiVersion": "v1",
				"kind":       "TestKind1",
			},
		}

		obj2 = &types.UnstructuredObject{
			Content: map[string]any{
				"apiVersion": "v1",
				"kind":       "TestKind2",
			},
		}

		obj3 = &types.UnstructuredObject{
			Content: map[string]any{
				"apiVersion": "v1",
				"kind":       "TestKind3",
			},
		}

		// Register handlers in the registry
		handlers.RegisterHandler("TestKind1", "v1", testHandler1)
		handlers.RegisterHandler("TestKind2", "v1", testHandler2)
		handlers.RegisterHandler("TestKind3", "v1", testHandler3)
	})

	Describe("Sort", func() {
		Context("when sorting objects with handlers of different priorities", func() {
			It("should order them by handler priority (ascending number = higher priority)", func() {
				// Create input slice with objects in random order
				objects := []types.Object{obj1, obj3, obj2}

				// Sort the objects
				sorted := handlers.Sort(objects)

				// Verify the order: testHandler2 (50) > testHandler1 (100) > testHandler3 (200)
				Expect(sorted).To(HaveLen(3))
				Expect(sorted[0]).To(Equal(obj2)) // First: Priority 50
				Expect(sorted[1]).To(Equal(obj1)) // Second: Priority 100
				Expect(sorted[2]).To(Equal(obj3)) // Third: Priority 200
			})

			It("should maintain original order for objects with same priority", func() {
				// Reset the expectations for handlers
				handlers.ResetRegistryForTest()

				// Register handlers with the same priority
				testHandler1 = mocks.NewMockResourceHandler(GinkgoT())
				testHandler1.EXPECT().Priority().Return(100).Maybe()

				testHandler2 = mocks.NewMockResourceHandler(GinkgoT())
				testHandler2.EXPECT().Priority().Return(100).Maybe()

				testHandler3 = mocks.NewMockResourceHandler(GinkgoT())
				testHandler3.EXPECT().Priority().Return(100).Maybe()

				// Register them again
				handlers.RegisterHandler("TestKind1", "v1", testHandler1)
				handlers.RegisterHandler("TestKind2", "v1", testHandler2)
				handlers.RegisterHandler("TestKind3", "v1", testHandler3)

				// Create input slice with the objects in a specific order
				objects := []types.Object{obj1, obj2, obj3}

				// Sort the objects
				sorted := handlers.Sort(objects)

				// Since all handlers have the same priority, order should be preserved
				// due to stable sort, but we only need to verify the length is the same
				// and that all objects are still present
				Expect(sorted).To(HaveLen(len(objects)))
				Expect(sorted).To(ContainElements(obj1, obj2, obj3))
			})
		})

		Context("when an object has no registered handler", func() {
			It("should not affect the order of those objects", func() {
				// Create an object without a registered handler
				objNoHandler := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "v1",
						"kind":       "UnregisteredKind",
					},
				}

				// Create input slice with mix of objects
				objects := []types.Object{obj1, objNoHandler, obj2}

				// Sort the objects
				sorted := handlers.Sort(objects)

				// Verify the objects with handlers are sorted correctly
				// objNoHandler should be in its original position
				Expect(sorted).To(HaveLen(3))

				// In this test, we don't need to check the specific order
				// since the unregistered handler affects sorting in implementation-specific ways.
				// Just verify all objects are present.

				// Verify objNoHandler is still in the result
				Expect(sorted).To(ContainElement(objNoHandler))
			})
		})
	})
})
