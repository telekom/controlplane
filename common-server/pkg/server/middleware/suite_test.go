// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware"
)

func TestMiddleware(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Middleware Suite")
}

var _ = Describe("Middleware", func() {

	Context("Logging", Ordered, func() {
		It("should log the request in valid JSON format", func() {
			var buffer = bytes.NewBuffer(nil)

			app := fiber.New()
			app.Use(middleware.NewContextLogger(&GinkgoLogr))
			app.Use(middleware.NewLogger(middleware.WithOutput(buffer)))
			req := httptest.NewRequest("GET", "/", nil)
			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			line, err := buffer.ReadString('\n')
			Expect(err).ToNot(HaveOccurred())
			Expect(line).To(ContainSubstring(`"ip":"0.0.0.0","host":"example.com","method":"GET","path":"/","status":404`))

			var logEntry map[string]interface{}
			err = json.Unmarshal([]byte(line), &logEntry)
			Expect(err).ToNot(HaveOccurred())
			Expect(logEntry).To(HaveKey("time"))
			Expect(logEntry).To(HaveKey("ip"))
			Expect(logEntry).To(HaveKey("cid"))
		})
	})
})
