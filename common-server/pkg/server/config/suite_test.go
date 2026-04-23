// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ config.StoreInfo = &storeInfo{}

type storeInfo struct {
	gvr schema.GroupVersionResource
	gvk schema.GroupVersionKind
}

func (s *storeInfo) Info() (schema.GroupVersionResource, schema.GroupVersionKind) {
	return s.gvr, s.gvk
}

var _ = Describe("Config", func() {
	Context("Configs Builder", func() {
		It("should return a list of GroupedItems", func() {
			gvr := schema.GroupVersionResource{
				Group:    "group",
				Version:  "version",
				Resource: "resource",
			}
			gvk := schema.GroupVersionKind{
				Group:   "group",
				Version: "version",
				Kind:    "kind",
			}
			store := &storeInfo{
				gvr: gvr,
				gvk: gvk,
			}
			configs := config.BuildConfigs(store)
			Expect(configs).To(HaveLen(1))
			Expect(configs[0].ApiVersion).To(Equal("group/version"))
			Expect(configs[0].Items).To(HaveLen(1))
			Expect(configs[0].Items[0].Kind).To(Equal("kind"))
			Expect(configs[0].Items[0].Resource).To(Equal("resource"))
		})
	})

	Context("Config Controller", func() {
		It("should return a new ConfigController", func() {
			gvr := schema.GroupVersionResource{
				Group:    "group",
				Version:  "version",
				Resource: "resource",
			}
			gvk := schema.GroupVersionKind{
				Group:   "group",
				Version: "version",
				Kind:    "kind",
			}
			store := &storeInfo{
				gvr: gvr,
				gvk: gvk,
			}
			controller := config.NewConfigController(GinkgoLogr, store)
			Expect(controller).NotTo(BeNil())

			s := server.NewServer()
			s.RegisterController(controller, server.ControllerOpts{})

			req := httptest.NewRequest("GET", "/config", http.NoBody)
			res, err := s.App.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.StatusCode).To(Equal(200))
		})
	})
})
