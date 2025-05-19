// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"io"
	"net/http/httptest"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server"
)

var _ = Describe("Probes Controller", func() {

	Context("Controller Config", func() {

		ctrl := server.NewProbesController()

		It("should have a default nop check", func() {
			Expect(ctrl.ReadyChecks).To(HaveLen(1))
			Expect(ctrl.HealthyChecks).To(HaveLen(1))
		})

		It("should add a ready check", func() {
			ctrl.AddReadyCheck(server.CustomCheck(func() bool {
				return true
			}))
			Expect(ctrl.ReadyChecks).To(HaveLen(2))
		})

		It("should add a healthy check", func() {
			ctrl.AddHealthyCheck(server.CustomCheck(func() bool {
				return true
			}))
			Expect(ctrl.HealthyChecks).To(HaveLen(2))
		})
	})

	Context("Http", func() {

		It("should return a healthy response", func() {
			ctrl := server.NewProbesController()
			ctrl.AddHealthyCheck(server.CustomCheck(func() bool {
				return true
			}))
			router := fiber.New()
			ctrl.Register(router, server.ControllerOpts{})

			req := httptest.NewRequest("GET", "/healthz", nil)
			res, err := router.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(200))
		})

		It("should return a ready response", func() {
			ctrl := server.NewProbesController()
			ctrl.AddReadyCheck(server.CustomCheck(func() bool {
				return true
			}))

			router := fiber.New()
			ctrl.Register(router, server.ControllerOpts{})

			req := httptest.NewRequest("GET", "/readyz", nil)
			res, err := router.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(200))
		})

		It("should return a 503 response (ready)", func() {
			ctrl := server.NewProbesController()
			ctrl.AddReadyCheck(server.CustomCheck(func() bool {
				return false
			}))
			router := fiber.New()
			ctrl.Register(router, server.ControllerOpts{})

			req := httptest.NewRequest("GET", "/readyz", nil)
			res, err := router.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(503))
			b, err := io.ReadAll(res.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(b).To(MatchJSON(`{"type":"","status":503,"title":"Service Unavailable","detail":"","instance":""}`))
		})

		It("should return a 503 response (healthy)", func() {
			ctrl := server.NewProbesController()
			ctrl.AddHealthyCheck(server.CustomCheck(func() bool {
				return false
			}))
			router := fiber.New()
			ctrl.Register(router, server.ControllerOpts{})

			req := httptest.NewRequest("GET", "/healthz", nil)
			res, err := router.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(503))
			b, err := io.ReadAll(res.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(b).To(MatchJSON(`{"type":"","status":503,"title":"Service Unavailable","detail":"","instance":""}`))
		})

	})
})
