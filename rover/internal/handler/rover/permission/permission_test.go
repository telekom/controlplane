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

var _ = Describe("normalizeAuthorization", func() {

	Context("resource-oriented format", func() {
		It("must expand resource with multiple role permissions", func() {
			input := []roverv1.Authorization{
				{
					Resource: "stargate:myapi:v1",
					Permissions: []roverv1.AuthorizationPermission{
						{Role: "admin", Actions: []string{"read", "write"}},
						{Role: "viewer", Actions: []string{"read"}},
					},
				},
			}

			result := normalizeAuthorization(input)

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

		It("must handle single permission entry", func() {
			input := []roverv1.Authorization{
				{
					Resource: "myresource",
					Permissions: []roverv1.AuthorizationPermission{
						{Role: "editor", Actions: []string{"edit"}},
					},
				},
			}

			result := normalizeAuthorization(input)

			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(Equal(permissionv1.Permission{
				Resource: "myresource",
				Role:     "editor",
				Actions:  []string{"edit"},
			}))
		})
	})

	Context("role-oriented format", func() {
		It("must expand role with multiple resource permissions", func() {
			input := []roverv1.Authorization{
				{
					Role: "admin",
					Permissions: []roverv1.AuthorizationPermission{
						{Resource: "users", Actions: []string{"read", "write", "delete"}},
						{Resource: "orders", Actions: []string{"read"}},
					},
				},
			}

			result := normalizeAuthorization(input)

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
			input := []roverv1.Authorization{
				{
					Role:     "viewer",
					Resource: "dashboard",
					Actions:  []string{"read"},
				},
			}

			result := normalizeAuthorization(input)

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
			input := []roverv1.Authorization{
				// Resource-oriented
				{
					Resource: "api:users:v1",
					Permissions: []roverv1.AuthorizationPermission{
						{Role: "admin", Actions: []string{"read", "write"}},
					},
				},
				// Role-oriented
				{
					Role: "auditor",
					Permissions: []roverv1.AuthorizationPermission{
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

			result := normalizeAuthorization(input)

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
			result := normalizeAuthorization(nil)
			Expect(result).To(BeNil())
		})

		It("must return nil for empty slice", func() {
			result := normalizeAuthorization([]roverv1.Authorization{})
			Expect(result).To(BeNil())
		})

		It("must skip entries that match no format", func() {
			input := []roverv1.Authorization{
				// Resource but no permissions and no role+actions = no match
				{Resource: "orphan"},
				// Role but no permissions and no resource+actions = no match
				{Role: "lonely"},
				// Valid flat entry
				{Role: "valid", Resource: "res", Actions: []string{"act"}},
			}

			result := normalizeAuthorization(input)

			Expect(result).To(HaveLen(1))
			Expect(result[0].Role).To(Equal("valid"))
		})
	})
})
