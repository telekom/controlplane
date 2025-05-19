// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package crd_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/internal/crd"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8s_testing "k8s.io/client-go/testing"
)

func NewCRD(group, version, resource string) *apiextensionsv1.CustomResourceDefinition {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.ToLower(resource) + "." + group,
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name: version,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{},
					},
				},
			},
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Singular: strings.Trim(resource, "s"),
				Kind:     strings.Trim(resource, "s"),
				Plural:   strings.ToLower(resource),
			},
		},
	}

	return crd
}

var mockClient = apiextension.NewSimpleClientset(NewCRD("testgroup", "v1", "tests"), NewCRD("testgroup", "v1", "foo"))

func TestCrd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Crd Suite")
}

var _ = Describe("CRD", func() {

	Context("Resolver", func() {

		var resolver crd.Resolver

		BeforeEach(func() {
			resolver = crd.NewResolver(mockClient)
		})

		It("should return an error when it cannot find the CRD", func() {

			crd, err := resolver.ResolveCrd(schema.GroupVersionResource{
				Group:    "testgroup",
				Version:  "v1",
				Resource: "notfound",
			})

			Expect(err).To(HaveOccurred())
			Expect(crd).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("CRD for testgroup/v1, Resource=notfound not found"))
		})

		It("should return the CRD when it can find it", func() {
			crd, err := resolver.ResolveCrd(schema.GroupVersionResource{
				Group:    "testgroup",
				Version:  "v1",
				Resource: "tests",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(crd).NotTo(BeNil())
			Expect(crd.GVK.Kind).To(Equal("test"))
		})

		It("should return multiple CRDs when it matches the pattern", func() {

			crds, err := resolver.ResolveCrds(schema.GroupVersionResource{
				Group:    "testgroup",
				Version:  "v1",
				Resource: ".*",
			}, 100)

			Expect(err).NotTo(HaveOccurred())
			Expect(crds).To(HaveLen(2))

			Expect(crds[0].GVK.Kind).To(Equal("foo"))
			Expect(crds[1].GVK.Kind).To(Equal("test"))
		})

		It("should return an error when it cannot compile the regex (resource)", func() {
			crds, err := resolver.ResolveCrds(schema.GroupVersionResource{
				Group:    "testgroup",
				Version:  "v1",
				Resource: "[",
			}, 100)

			Expect(err).To(HaveOccurred())
			Expect(crds).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to compile regexp ["))
		})

		It("should return an error when it cannot compile the regex (group)", func() {
			crds, err := resolver.ResolveCrds(schema.GroupVersionResource{
				Group:    "[",
				Version:  "v1",
				Resource: ".*",
			}, 100)

			Expect(err).To(HaveOccurred())
			Expect(crds).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to compile regexp ["))
		})

		It("should return an error when it was not initialized", func() {
			resolver = crd.NewResolver(nil)
			crds, err := resolver.ResolveCrds(schema.GroupVersionResource{
				Group:    "testgroup",
				Version:  "v1",
				Resource: ".*",
			}, 100)

			Expect(err).To(HaveOccurred())
			Expect(crds).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("CRD resolver not initialized"))
		})

		It("should return an error when it cannot list the CRDs", func() {
			mockClient.PrependReactor("list", "customresourcedefinitions", func(action k8s_testing.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, fmt.Errorf("MOCK_ERROR")
			})
			resolver = crd.NewResolver(mockClient)

			crds, err := resolver.ResolveCrds(schema.GroupVersionResource{
				Group:    "testgroup",
				Version:  "v1",
				Resource: ".*",
			}, 100)

			Expect(err).To(HaveOccurred())
			Expect(crds).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("MOCK_ERROR"))
		})

	})
})
