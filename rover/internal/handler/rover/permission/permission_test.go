// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permission

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("normalizePermissions", func() {

	Context("resource-oriented format", func() {
		It("must expand resource with multiple role entries", func() {
			input := []roverv1.Permission{
				{
					Resource: "stargate:myapi:v1",
					Entries: []roverv1.PermissionEntry{
						{Role: "admin", Actions: []string{"read", "write"}},
						{Role: "viewer", Actions: []string{"read"}},
					},
				},
			}

			result := normalizePermissions(input)

			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(Equal(permissionv1.Permission{
				Resource: "stargate:myapi:v1",
				Role:     "admin",
				Actions:  []string{"read", "write"},
			}))
			Expect(result[1]).To(Equal(permissionv1.Permission{
				Resource: "stargate:myapi:v1",
				Role:     "viewer",
				Actions:  []string{"read"},
			}))
		})

		It("must handle single entry", func() {
			input := []roverv1.Permission{
				{
					Resource: "myresource",
					Entries: []roverv1.PermissionEntry{
						{Role: "editor", Actions: []string{"edit"}},
					},
				},
			}

			result := normalizePermissions(input)

			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(Equal(permissionv1.Permission{
				Resource: "myresource",
				Role:     "editor",
				Actions:  []string{"edit"},
			}))
		})
	})

	Context("role-oriented format", func() {
		It("must expand role with multiple resource entries", func() {
			input := []roverv1.Permission{
				{
					Role: "admin",
					Entries: []roverv1.PermissionEntry{
						{Resource: "users", Actions: []string{"read", "write", "delete"}},
						{Resource: "orders", Actions: []string{"read"}},
					},
				},
			}

			result := normalizePermissions(input)

			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(Equal(permissionv1.Permission{
				Role:     "admin",
				Resource: "users",
				Actions:  []string{"read", "write", "delete"},
			}))
			Expect(result[1]).To(Equal(permissionv1.Permission{
				Role:     "admin",
				Resource: "orders",
				Actions:  []string{"read"},
			}))
		})
	})

	Context("flat format", func() {
		It("must pass through role + resource + actions directly", func() {
			input := []roverv1.Permission{
				{
					Role:     "viewer",
					Resource: "dashboard",
					Actions:  []string{"read"},
				},
			}

			result := normalizePermissions(input)

			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(Equal(permissionv1.Permission{
				Role:     "viewer",
				Resource: "dashboard",
				Actions:  []string{"read"},
			}))
		})
	})

	Context("mixed formats", func() {
		It("must handle all three formats in a single list", func() {
			input := []roverv1.Permission{
				// Resource-oriented
				{
					Resource: "api:users:v1",
					Entries: []roverv1.PermissionEntry{
						{Role: "admin", Actions: []string{"read", "write"}},
					},
				},
				// Role-oriented
				{
					Role: "auditor",
					Entries: []roverv1.PermissionEntry{
						{Resource: "logs", Actions: []string{"read"}},
					},
				},
				// Flat
				{
					Role:     "operator",
					Resource: "cluster",
					Actions:  []string{"restart", "scale"},
				},
			}

			result := normalizePermissions(input)

			Expect(result).To(HaveLen(3))
			Expect(result[0]).To(Equal(permissionv1.Permission{
				Resource: "api:users:v1",
				Role:     "admin",
				Actions:  []string{"read", "write"},
			}))
			Expect(result[1]).To(Equal(permissionv1.Permission{
				Role:     "auditor",
				Resource: "logs",
				Actions:  []string{"read"},
			}))
			Expect(result[2]).To(Equal(permissionv1.Permission{
				Role:     "operator",
				Resource: "cluster",
				Actions:  []string{"restart", "scale"},
			}))
		})
	})

	Context("edge cases", func() {
		It("must return nil for empty input", func() {
			result := normalizePermissions(nil)
			Expect(result).To(BeNil())
		})

		It("must return nil for empty slice", func() {
			result := normalizePermissions([]roverv1.Permission{})
			Expect(result).To(BeNil())
		})

		It("must skip entries that match no format", func() {
			input := []roverv1.Permission{
				// Resource but no entries and no role+actions = no match
				{Resource: "orphan"},
				// Role but no entries and no resource+actions = no match
				{Role: "lonely"},
				// Valid flat entry
				{Role: "valid", Resource: "res", Actions: []string{"act"}},
			}

			result := normalizePermissions(input)

			Expect(result).To(HaveLen(1))
			Expect(result[0].Role).To(Equal("valid"))
		})
	})
})
