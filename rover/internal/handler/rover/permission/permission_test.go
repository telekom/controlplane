// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permission

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/config"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("HandlePermission", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		testScheme *runtime.Scheme
		owner      *roverv1.Rover
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)

		testScheme = runtime.NewScheme()
		_ = roverv1.AddToScheme(testScheme)
		_ = permissionv1.AddToScheme(testScheme)

		owner = &roverv1.Rover{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app",
				Namespace: "poc--eni--hyperion",
				UID:       "rover-uid-1234",
			},
			Spec: roverv1.RoverSpec{
				Zone: "dataplane1",
				Permissions: []roverv1.Permission{
					{
						Resource: "stargate:myapi:v1",
						Entries: []roverv1.PermissionEntry{
							{Role: "admin", Actions: []string{"read", "write"}},
						},
					},
				},
			},
		}
	})

	It("must create a PermissionSet with normalized permissions", func() {
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.PermissionSet"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := HandlePermission(ctx, fakeClient, owner)

		Expect(err).ToNot(HaveOccurred())
		Expect(owner.Status.PermissionSets).To(HaveLen(1))
		Expect(owner.Status.PermissionSets[0].Name).To(Equal("my-app"))
		Expect(owner.Status.PermissionSets[0].Namespace).To(Equal("poc--eni--hyperion"))
	})

	It("must set application and zone labels on the PermissionSet", func() {
		var capturedPS *permissionv1.PermissionSet

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.PermissionSet"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedPS = obj.(*permissionv1.PermissionSet)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := HandlePermission(ctx, fakeClient, owner)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedPS.Labels).To(HaveKeyWithValue(config.BuildLabelKey("application"), "my-app"))
		Expect(capturedPS.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "dataplane1"))
	})

	It("must set normalized permissions in spec", func() {
		var capturedPS *permissionv1.PermissionSet

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.PermissionSet"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedPS = obj.(*permissionv1.PermissionSet)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := HandlePermission(ctx, fakeClient, owner)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedPS.Spec.Permissions).To(HaveLen(1))
		Expect(capturedPS.Spec.Permissions[0]).To(Equal(permissionv1.Permission{
			Resource: "stargate:myapi:v1",
			Role:     "admin",
			Actions:  []string{"read", "write"},
		}))
	})

	It("must not set zone label when zone is empty", func() {
		owner.Spec.Zone = ""
		var capturedPS *permissionv1.PermissionSet

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.PermissionSet"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedPS = obj.(*permissionv1.PermissionSet)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := HandlePermission(ctx, fakeClient, owner)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedPS.Labels).ToNot(HaveKey(config.BuildLabelKey("zone")))
	})

	It("must return error when CreateOrUpdate fails", func() {
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.PermissionSet"), mock.AnythingOfType("controllerutil.MutateFn")).
			Return(controllerutil.OperationResultNone, fmt.Errorf("api server error")).Once()

		err := HandlePermission(ctx, fakeClient, owner)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create or update PermissionSet"))
	})
})

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
