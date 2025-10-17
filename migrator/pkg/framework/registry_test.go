// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Registry", func() {
	var registry *Registry

	BeforeEach(func() {
		registry = NewRegistry()
	})

	Describe("Register", func() {
		It("should register a migrator successfully", func() {
			migrator := &mockMigrator{name: "test-migrator"}

			err := registry.Register(migrator)

			Expect(err).NotTo(HaveOccurred())
			Expect(registry.Count()).To(Equal(1))
		})

		It("should return error when registering duplicate migrator", func() {
			migrator1 := &mockMigrator{name: "test-migrator"}
			migrator2 := &mockMigrator{name: "test-migrator"}

			err := registry.Register(migrator1)
			Expect(err).NotTo(HaveOccurred())

			err = registry.Register(migrator2)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already registered"))
		})

		It("should register multiple different migrators", func() {
			migrator1 := &mockMigrator{name: "migrator-1"}
			migrator2 := &mockMigrator{name: "migrator-2"}
			migrator3 := &mockMigrator{name: "migrator-3"}

			Expect(registry.Register(migrator1)).To(Succeed())
			Expect(registry.Register(migrator2)).To(Succeed())
			Expect(registry.Register(migrator3)).To(Succeed())

			Expect(registry.Count()).To(Equal(3))
		})
	})

	Describe("Get", func() {
		It("should retrieve a registered migrator", func() {
			migrator := &mockMigrator{name: "test-migrator"}
			registry.Register(migrator)

			retrieved, exists := registry.Get("test-migrator")

			Expect(exists).To(BeTrue())
			Expect(retrieved).To(Equal(migrator))
		})

		It("should return false for non-existent migrator", func() {
			_, exists := registry.Get("non-existent")

			Expect(exists).To(BeFalse())
		})
	})

	Describe("List", func() {
		It("should return empty list when no migrators registered", func() {
			list := registry.List()

			Expect(list).To(BeEmpty())
		})

		It("should return all registered migrator names", func() {
			registry.Register(&mockMigrator{name: "migrator-1"})
			registry.Register(&mockMigrator{name: "migrator-2"})
			registry.Register(&mockMigrator{name: "migrator-3"})

			list := registry.List()

			Expect(list).To(HaveLen(3))
			Expect(list).To(ContainElements("migrator-1", "migrator-2", "migrator-3"))
		})
	})

	Describe("Count", func() {
		It("should return 0 when empty", func() {
			Expect(registry.Count()).To(Equal(0))
		})

		It("should return correct count after registrations", func() {
			registry.Register(&mockMigrator{name: "migrator-1"})
			Expect(registry.Count()).To(Equal(1))

			registry.Register(&mockMigrator{name: "migrator-2"})
			Expect(registry.Count()).To(Equal(2))

			registry.Register(&mockMigrator{name: "migrator-3"})
			Expect(registry.Count()).To(Equal(3))
		})
	})
})

// Additional mock implementations for testing
type simpleMigrator struct {
	name string
}

func (m *simpleMigrator) GetName() string {
	return m.name
}

func (m *simpleMigrator) GetNewResourceType() client.Object {
	return &MockResource{}
}

func (m *simpleMigrator) GetLegacyAPIGroup() string {
	return "test.legacy.io"
}

func (m *simpleMigrator) ComputeLegacyIdentifier(ctx context.Context, obj client.Object) (string, string, bool, error) {
	return "default", "test", false, nil
}

func (m *simpleMigrator) FetchFromLegacy(ctx context.Context, remoteClient client.Client, namespace, name string) (client.Object, error) {
	return &MockResource{}, nil
}

func (m *simpleMigrator) HasChanged(ctx context.Context, current, legacy client.Object) bool {
	return false
}

func (m *simpleMigrator) ApplyMigration(ctx context.Context, current, legacy client.Object) error {
	return nil
}

func (m *simpleMigrator) GetRequeueAfter() time.Duration {
	return 30 * time.Second
}
