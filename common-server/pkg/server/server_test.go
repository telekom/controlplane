package server_test

import (
	"io"
	"net/http/httptest"
	"time"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type MockController struct {
	wasCalled bool
}

func (c *MockController) Register(router fiber.Router, opts server.ControllerOpts) {
	c.wasCalled = true
}

var _ = Describe("Server", func() {

	Context("ControllerOpts", func() {

		It("should return default options", func() {
			opts := server.Default()
			Expect(opts.AllowedMethods).To(Equal([]string{"HEAD", "GET", "POST", "PUT", "PATCH", "DELETE"}))
		})

		It("should return read-only options", func() {
			opts := server.ReadOnly()
			Expect(opts.AllowedMethods).To(Equal([]string{"HEAD", "GET"}))
		})

		It("should return true for allowed method", func() {
			opts := server.Default()
			Expect(opts.IsAllowed("GET")).To(BeTrue())
		})

		It("should return false for not allowed method", func() {
			opts := server.ReadOnly()
			Expect(opts.IsAllowed("POST")).To(BeFalse())
		})

		It("should return true for empty allowed methods", func() {
			opts := server.ControllerOpts{}
			Expect(opts.IsAllowed("GET")).To(BeTrue())
		})
	})

	Context("CalculatePrefix", func() {

		gvr := schema.GroupVersionResource{
			Group:    "testgroup",
			Version:  "v1",
			Resource: "tests",
		}

		It("should return only version and resource", func() {
			prefix := server.CalculatePrefix(gvr, false)
			Expect(prefix).To(Equal("/v1/tests"))
		})

		It("should return prefix with complete gvr", func() {
			prefix := server.CalculatePrefix(gvr, true)
			Expect(prefix).To(Equal("/testgroup/v1/tests"))
		})
	})

	Context("Server", func() {

		It("should register controller", func() {
			s := server.Server{
				App: server.NewApp(),
			}
			controller := &MockController{}
			opts := server.ControllerOpts{}
			s.RegisterController(controller, opts)
			Expect(controller.wasCalled).To(BeTrue())
		})

		It("should create new app", func() {
			app := server.NewApp()
			Expect(app).NotTo(BeNil())
		})

		It("should create a new app with custom config", func() {
			appCfg := server.NewAppConfig()
			appCfg.CtxLog = &GinkgoLogr
			app := server.NewAppWithConfig(appCfg)
			Expect(app).NotTo(BeNil())
		})

		It("should create a new server", func() {
			s := server.NewServer()
			Expect(s.App).NotTo(BeNil())
		})

		It("should start the server", func() {
			s := server.NewServer()
			go func() {
				time.Sleep(1 * time.Second)
				Expect(s.App.Shutdown()).To(Succeed())
			}()

			err := s.Start(":0")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Return", func() {
		app := server.NewApp()
		app.All("/test", func(c *fiber.Ctx) error {
			return server.Return(c, 202, map[string]any{
				"message": "test",
			})
		})

		It("should return 202 with JSON", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Accept", "application/json")
			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(202))
			Expect(res.Header.Get("Content-Type")).To(Equal("application/json"))
			b, _ := io.ReadAll(res.Body)
			Expect(b).To(MatchJSON(`{"message": "test"}`))
		})

		It("should return 202 with the fallback", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Accept", "application/xml")
			res, err := app.Test(req)

			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(202))
			Expect(res.Header.Get("Content-Type")).To(Equal("application/json"))
			b, _ := io.ReadAll(res.Body)
			Expect(b).To(MatchJSON(`{"message": "test"}`))
		})

	})
})
