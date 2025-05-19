// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/internal/config"
	"github.com/telekom/controlplane/common-server/internal/crd"
	"github.com/telekom/controlplane/common-server/pkg/store"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamic "k8s.io/client-go/dynamic/fake"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Internal config Suite")
}

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

var _ = Describe("Config Test", func() {

	var gvr = schema.GroupVersionResource{
		Group:    "testgroup",
		Version:  "v1",
		Resource: "testresources",
	}

	Context("Read Config", func() {
		It("should correctly read the config from a file", func() {
			cfg, err := config.ReadConfig("config.test.yaml")
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).ToNot(BeNil())
		})
	})

	Context("Build Server from Config", func() {
		ctx := context.Background()
		log := logr.Discard()
		fakeClient := dynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			gvr: "TestObjectList",
		})

		crd.InitCrdResolverWithClient(apiextension.NewSimpleClientset(NewCRD(gvr.Group, gvr.Version, gvr.Resource)))

		It("should correctly build a server from a config", func() {

			cfg := &config.ServerConfig{
				Address:        ":8080",
				BasePath:       "/api",
				AddGroupToPath: true,
				Resources: []config.ResourceConfig{
					{
						Group:    gvr.Group,
						Version:  gvr.Version,
						Resource: gvr.Resource,
					},
				},
				Predefined: []config.PredefinedConfig{
					{
						Ref:  "testresources",
						Name: "ByName",
						Filters: []store.Filter{
							{
								Path:  "spec.name",
								Op:    store.OpEqual,
								Value: "$<name>",
							},
						},
					},
				},
			}

			server, err := cfg.BuildServer(ctx, fakeClient, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(server).ToNot(BeNil())

			expectedRoutes := map[string]bool{
				"GET /api/config":       false,
				"GET /api/openapi.json": false,
				"GET /api/testgroup/v1/testresources/:namespace/:name":    false,
				"GET /api/testgroup/v1/testresources/":                    false,
				"POST /api/testgroup/v1/testresources/":                   false,
				"PATCH /api/testgroup/v1/testresources/:namespace/:name":  false,
				"DELETE /api/testgroup/v1/testresources/:namespace/:name": false,
				"GET /api/testgroup/v1/testresources/ByName":              false,
			}

			for _, route := range server.App.GetRoutes() {
				id := fmt.Sprintf("%s %s", route.Method, route.Path)
				if _, ok := expectedRoutes[id]; ok {
					expectedRoutes[id] = true
				}
			}

			for route, found := range expectedRoutes {
				if !found {
					Fail(fmt.Sprintf("Route %s not found", route))
				}
			}
		})
	})
})
