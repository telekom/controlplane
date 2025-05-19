package server_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/test/mocks"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/store"
)

var _ = Describe("Predefined controller test with placeholder filter", func() {

	var (
		mockStore            *mocks.MockObjectStore[*unstructured.Unstructured]
		app                  *fiber.App
		predefinedController server.PredefinedController
	)

	BeforeEach(func() {
		mockStore = mocks.NewMockObjectStore[*unstructured.Unstructured](GinkgoT())
		app = fiber.New()

		predefinedController := server.NewPredefinedController("byKey1Placeholder", mockStore, logr.Discard())
		predefinedController.AddFilter(store.Filter{
			Path:  "spec.key1",
			Op:    store.OpEqual,
			Value: "$<key1>",
		})
		predefinedController.Register(app.Group("/tests"), server.ControllerOpts{Prefix: "/tests"})
	})

	AfterEach(func() {
	})

	Context("Test setup is valid", func() {
		It("should register all routes", func() {
			Expect(predefinedController).ToNot(BeNil())

			expectedRoutes := map[string]bool{
				"GET /tests/byKey1Placeholder":  false,
				"HEAD /tests/byKey1Placeholder": false,
			}

			for _, route := range app.GetRoutes() {
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

	Context("List objects", func() {

		BeforeEach(func() {
			By("Letting the store accept any arguments for listing (checked in later stages)")
			mockStore.EXPECT().List(mock.Anything, mock.Anything).Return(&store.ListResponse[*unstructured.Unstructured]{
				Items: []*unstructured.Unstructured{},
				Links: store.ListResponseLinks{
					Self: "self-link",
					Next: "next-link",
				},
			}, nil)
		})

		It("should construct store.ListOpts properly - replace placeholder", func() {

			By("Calling the predefined controller endpoint")
			req := httptest.NewRequest(http.MethodGet, "/tests/byKey1Placeholder?key1=value1_2", nil)
			resp, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			By("Checking the store was called just once")
			Expect(mockStore.Calls).To(HaveLen(1))
			Expect(mockStore.Calls[0].Arguments).To(HaveLen(2))

			By("Checking the arguments of the store list call")
			var arg = mockStore.Calls[0].Arguments.Get(1)
			realOpts, ok := arg.(store.ListOpts)
			Expect(ok).To(BeTrue())

			Expect(realOpts.Filters).To(HaveLen(1))
			Expect(realOpts.Filters).To(ConsistOf(store.Filter{
				Path:  "spec.key1",
				Op:    store.OpEqual,
				Value: "value1_2",
			}))
			Expect(realOpts.Limit).To(Equal(100))
			Expect(realOpts.Prefix).To(Equal(""))
			Expect(realOpts.Cursor).To(Equal(""))
			Expect(realOpts.Sorters).To(BeEmpty())
		})

		It("should construct store.ListOpts properly - replace placeholder properly after multiple calls", func() {

			By("Calling the predefined controller endpoint - first call")
			req := httptest.NewRequest(http.MethodGet, "/tests/byKey1Placeholder?key1=value1_2", nil)
			resp, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			By("Checking the store was called just once - first call")
			Expect(mockStore.Calls).To(HaveLen(1))
			Expect(mockStore.Calls[0].Arguments).To(HaveLen(2))

			By("Checking the arguments of the store list call - first call")
			var arg = mockStore.Calls[0].Arguments.Get(1)
			realOpts, ok := arg.(store.ListOpts)
			Expect(ok).To(BeTrue())

			Expect(realOpts.Filters).To(HaveLen(1))
			Expect(realOpts.Filters).To(ConsistOf(store.Filter{
				Path:  "spec.key1",
				Op:    store.OpEqual,
				Value: "value1_2",
			}))
			Expect(realOpts.Limit).To(Equal(100))
			Expect(realOpts.Prefix).To(Equal(""))
			Expect(realOpts.Cursor).To(Equal(""))
			Expect(realOpts.Sorters).To(BeEmpty())

			By("Calling the predefined controller endpoint - second call")
			req = httptest.NewRequest(http.MethodGet, "/tests/byKey1Placeholder?key1=value5_6", nil)
			resp, err = app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			By("Checking the store was called just once - second call")
			Expect(mockStore.Calls).To(HaveLen(2))
			Expect(mockStore.Calls[1].Arguments).To(HaveLen(2))

			By("Checking the arguments of the store list call - second call")
			arg = mockStore.Calls[1].Arguments.Get(1)
			realOpts, ok = arg.(store.ListOpts)
			Expect(ok).To(BeTrue())

			Expect(realOpts.Filters).To(HaveLen(1))
			Expect(realOpts.Filters).To(ConsistOf(store.Filter{
				Path:  "spec.key1",
				Op:    store.OpEqual,
				Value: "value5_6",
			}))
			Expect(realOpts.Limit).To(Equal(100))
			Expect(realOpts.Prefix).To(Equal(""))
			Expect(realOpts.Cursor).To(Equal(""))
			Expect(realOpts.Sorters).To(BeEmpty())

			By("Calling the predefined controller endpoint - third call")
			req = httptest.NewRequest(http.MethodGet, "/tests/byKey1Placeholder?key1=bla", nil)
			resp, err = app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			By("Checking the store was called just once - third call")
			Expect(mockStore.Calls).To(HaveLen(3))
			Expect(mockStore.Calls[2].Arguments).To(HaveLen(2))

			By("Checking the arguments of the store list call - third call")
			arg = mockStore.Calls[2].Arguments.Get(1)
			realOpts, ok = arg.(store.ListOpts)
			Expect(ok).To(BeTrue())

			Expect(realOpts.Filters).To(HaveLen(1))
			Expect(realOpts.Filters).To(ConsistOf(store.Filter{
				Path:  "spec.key1",
				Op:    store.OpEqual,
				Value: "bla",
			}))
			Expect(realOpts.Limit).To(Equal(100))
			Expect(realOpts.Prefix).To(Equal(""))
			Expect(realOpts.Cursor).To(Equal(""))
			Expect(realOpts.Sorters).To(BeEmpty())
		})

		It("should construct store.ListOpts properly - accept custom filter", func() {

			By("Calling the predefined controller endpoint")
			req := httptest.NewRequest(http.MethodGet, "/tests/byKey1Placeholder?key1=value1_2&filter=spec.key2=~value2_2&limit=22", nil)
			resp, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			By("Checking the store was called just once")
			Expect(mockStore.Calls).To(HaveLen(1))
			Expect(mockStore.Calls[0].Arguments).To(HaveLen(2))

			By("Checking the arguments of the store list call")
			var arg = mockStore.Calls[0].Arguments.Get(1)
			realOpts, ok := arg.(store.ListOpts)
			Expect(ok).To(BeTrue())

			Expect(realOpts.Filters).To(HaveLen(2))
			Expect(realOpts.Filters).To(ConsistOf(store.Filter{
				Path:  "spec.key1",
				Op:    store.OpEqual,
				Value: "value1_2",
			}, store.Filter{
				Path:  "spec.key2",
				Op:    store.OpRegex,
				Value: "value2_2",
			}))

			Expect(realOpts.Limit).To(Equal(22))
			Expect(realOpts.Prefix).To(Equal(""))
			Expect(realOpts.Cursor).To(Equal(""))
			Expect(realOpts.Sorters).To(BeEmpty())
		})
	})

})

var _ = Describe("Predefined controller test with hardcoded filter", func() {

	var (
		mockStore            *mocks.MockObjectStore[*unstructured.Unstructured]
		app                  *fiber.App
		predefinedController server.PredefinedController
	)

	BeforeEach(func() {
		mockStore = mocks.NewMockObjectStore[*unstructured.Unstructured](GinkgoT())
		app = fiber.New()

		predefinedController := server.NewPredefinedController("byKey1Hardcoded", mockStore, logr.Discard())
		predefinedController.AddFilter(store.Filter{
			Path:  "spec.key1",
			Op:    store.OpEqual,
			Value: "value1_1",
		})
		predefinedController.Register(app.Group("/tests"), server.ControllerOpts{Prefix: "/tests"})
	})

	AfterEach(func() {
	})

	Context("Test setup is valid", func() {
		It("should register all routes", func() {
			Expect(predefinedController).ToNot(BeNil())

			expectedRoutes := map[string]bool{
				"GET /tests/byKey1Hardcoded":  false,
				"HEAD /tests/byKey1Hardcoded": false,
			}

			for _, route := range app.GetRoutes() {
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

	Context("List objects", func() {

		BeforeEach(func() {
			By("Letting the store accept any arguments for listing (checked in later stages)")
			mockStore.EXPECT().List(mock.Anything, mock.Anything).Return(&store.ListResponse[*unstructured.Unstructured]{
				Items: []*unstructured.Unstructured{},
				Links: store.ListResponseLinks{
					Self: "self-link",
					Next: "next-link",
				},
			}, nil)
		})

		var verifyListOpts = func(realOpts store.ListOpts) {
			Expect(realOpts.Filters).To(HaveLen(1))
			Expect(realOpts.Filters).To(ConsistOf(store.Filter{
				Path:  "spec.key1",
				Op:    store.OpEqual,
				Value: "value1_1",
			}))

			Expect(realOpts.Limit).To(Equal(100))
			Expect(realOpts.Prefix).To(Equal(""))
			Expect(realOpts.Cursor).To(Equal(""))
			Expect(realOpts.Sorters).To(BeEmpty())
		}

		It("should construct store.ListOpts properly - use hardcoded value", func() {
			By("Calling the predefined controller endpoint")
			req := httptest.NewRequest(http.MethodGet, "/tests/byKey1Hardcoded", nil)
			resp, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			By("Checking the store was called just once")
			Expect(mockStore.Calls).To(HaveLen(1))
			Expect(mockStore.Calls[0].Arguments).To(HaveLen(2))

			By("Checking the arguments of the store list call")
			var arg = mockStore.Calls[0].Arguments.Get(1)
			realOpts, ok := arg.(store.ListOpts)
			Expect(ok).To(BeTrue())
			verifyListOpts(realOpts)
		})

		It("should construct store.ListOpts properly - dont replace hardcoded filter", func() {
			By("Calling the predefined controller endpoint + custom query params")
			req := httptest.NewRequest(http.MethodGet, "/tests/byKey1Hardcoded?filter=spec.key1==value9_9", nil)
			resp, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			By("Checking the store was called just once")
			Expect(mockStore.Calls).To(HaveLen(1))
			Expect(mockStore.Calls[0].Arguments).To(HaveLen(2))

			By("Checking the arguments of the store list call")
			var arg = mockStore.Calls[0].Arguments.Get(1)
			realOpts, ok := arg.(store.ListOpts)
			Expect(ok).To(BeTrue())
			verifyListOpts(realOpts)
		})
	})

})
