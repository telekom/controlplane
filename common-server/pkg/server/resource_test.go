package server_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/test/mocks"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("ResourceController", func() {
	var (
		mockStore      *mocks.MockObjectStore[*unstructured.Unstructured]
		app            *fiber.App
		resourceCtrl   *server.ResourceController
		testGVR        schema.GroupVersionResource
		testGVK        schema.GroupVersionKind
		testNamespace  string
		testName       string
		testObjectJSON []byte
	)

	BeforeEach(func() {
		mockStore = mocks.NewMockObjectStore[*unstructured.Unstructured](GinkgoT())
		testGVR = schema.GroupVersionResource{Group: "test.group", Version: "v1", Resource: "tests"}
		testGVK = schema.GroupVersionKind{Group: "test.group", Version: "v1", Kind: "Test"}
		testNamespace = "default"
		testName = "test-name"
		testObjectJSON = []byte(`{"apiVersion":"test.group/v1","kind":"Test","metadata":{"name":"test-name","namespace":"default"}}`)

		mockStore.EXPECT().Info().Return(testGVR, testGVK)
		resourceCtrl = server.NewResourceController(mockStore, GinkgoLogr)
		app = fiber.New()
		resourceCtrl.Register(app.Group("/tests"), server.ControllerOpts{Prefix: "/tests"})
	})

	AfterEach(func() {
	})

	Context("CreateOrUpdate", func() {
		It("should create or update an object successfully", func() {
			mockStore.EXPECT().CreateOrReplace(mock.Anything, mock.Anything).Return(nil)

			req := httptest.NewRequest(http.MethodPost, "/tests", bytes.NewReader(testObjectJSON))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		})

		It("should return an error for invalid JSON", func() {
			req := httptest.NewRequest(http.MethodPost, "/tests", bytes.NewReader([]byte("invalid-json")))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Context("Read", func() {
		It("should read an object successfully", func() {
			mockStore.EXPECT().Get(mock.Anything, testNamespace, testName).Return(&unstructured.Unstructured{}, nil)

			req := httptest.NewRequest(http.MethodGet, "/tests/default/test-name", nil)
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should return an error if the object is not found", func() {
			mockStore.EXPECT().Get(mock.Anything, testNamespace, testName).Return(nil, errors.New("not found"))

			req := httptest.NewRequest(http.MethodGet, "/tests/default/test-name", nil)
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		})
	})

	Context("Delete", func() {
		It("should delete an object successfully", func() {
			mockStore.EXPECT().Delete(mock.Anything, testNamespace, testName).Return(nil)

			req := httptest.NewRequest(http.MethodDelete, "/tests/default/test-name", nil)
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		})

		It("should return an error if the object cannot be deleted", func() {
			mockStore.EXPECT().Delete(mock.Anything, testNamespace, testName).Return(errors.New("delete error"))

			req := httptest.NewRequest(http.MethodDelete, "/tests/default/test-name", nil)
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		})
	})

	Context("List", func() {
		It("should list objects successfully", func() {
			mockStore.EXPECT().List(mock.Anything, mock.Anything).Return(&store.ListResponse[*unstructured.Unstructured]{
				Items: []*unstructured.Unstructured{},
				Links: store.ListResponseLinks{
					Self: "self-link",
					Next: "next-link",
				},
			}, nil)

			req := httptest.NewRequest(http.MethodGet, "/tests", nil)
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("X-Cursor-Self")).To(Equal("self-link"))
			Expect(resp.Header.Get("X-Cursor-Next")).To(Equal("next-link"))
		})

		It("should return an error if listing fails", func() {
			mockStore.EXPECT().List(mock.Anything, mock.Anything).Return(nil, errors.New("list error"))

			req := httptest.NewRequest(http.MethodGet, "/tests", nil)
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		})
	})

	Context("Patch", func() {
		It("should patch an object successfully", func() {
			mockStore.EXPECT().Patch(mock.Anything, testNamespace, testName, mock.Anything).Return(&unstructured.Unstructured{}, nil)

			patchJSON := `[{"op":"replace","path":"/metadata/labels","value":{"key":"value"}}]`
			req := httptest.NewRequest(http.MethodPatch, "/tests/default/test-name", bytes.NewReader([]byte(patchJSON)))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should return an error for invalid patch JSON", func() {
			req := httptest.NewRequest(http.MethodPatch, "/tests/default/test-name", bytes.NewReader([]byte("invalid-json")))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("should return an error if patch operation fails", func() {
			mockStore.EXPECT().Patch(mock.Anything, testNamespace, testName, mock.Anything).Return(nil, errors.New("patch error"))

			patchJSON := `[{"op":"replace","path":"/metadata/labels","value":{"key":"value"}}]`
			req := httptest.NewRequest(http.MethodPatch, "/tests/default/test-name", bytes.NewReader([]byte(patchJSON)))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		})
	})

	Context("QueryParser", func() {
		It("should parse query parameters correctly", func() {
			app.Get("/queryargs", func(c *fiber.Ctx) error {
				opts := store.NewListOpts()
				err := server.QueryParser(c, &opts)
				Expect(err).ToNot(HaveOccurred())
				Expect(opts.Prefix).To(Equal("test-prefix"))
				Expect(opts.Cursor).To(Equal("test-cursor"))
				Expect(opts.Limit).To(Equal(10))
				Expect(opts.Filters).To(HaveLen(1))
				Expect(opts.Filters[0].Path).To(Equal("key"))
				Expect(opts.Filters[0].Value).To(Equal("value"))
				Expect(opts.Sorters).To(HaveLen(1))
				Expect(opts.Sorters[0].Path).To(Equal("key"))
				Expect(opts.Sorters[0].Order).To(Equal(store.SortOrderAsc))
				return nil
			})

			req := httptest.NewRequest(http.MethodGet, "/queryargs?prefix=test-prefix&cursor=test-cursor&limit=10&filter=key==value&sort=key:asc", nil)
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should return an error for invalid filter", func() {
			app.Get("/queryargs", func(c *fiber.Ctx) error {
				opts := store.NewListOpts()
				err := server.QueryParser(c, &opts)
				Expect(err).To(HaveOccurred())
				return nil
			})

			req := httptest.NewRequest(http.MethodGet, "/queryargs?filter=invalid-filter", nil)
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should return an error for invalid sorter", func() {
			app.Get("/queryargs", func(c *fiber.Ctx) error {
				opts := store.NewListOpts()
				err := server.QueryParser(c, &opts)
				Expect(err).To(HaveOccurred())
				return nil
			})

			req := httptest.NewRequest(http.MethodGet, "/queryargs?sort=invalid-sort", nil)
			resp, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})
