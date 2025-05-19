// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"errors"
	"io"
	"net/http/httptest"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server"
)

var _ = Describe("Error", func() {

	Context("ReturnWithProblem", func() {

		var app *fiber.App

		BeforeEach(func() {
			app = server.NewApp()
		})

		It("should return a problem", func() {
			problem := problems.BadRequest("test")
			app.Get("/", func(c *fiber.Ctx) error {
				return server.ReturnWithProblem(c, problem, nil)
			})

			req := httptest.NewRequest("GET", "/", nil)
			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(problem.Code()))
			b, _ := io.ReadAll(res.Body)
			Expect(b).To(MatchJSON(`{"type": "BadRequest", "status": 400, "title": "Bad Request", "detail": "test", "instance": ""}`))
		})

		It("should return an error", func() {
			app.Get("/", func(c *fiber.Ctx) error {
				return server.ReturnWithProblem(c, nil, errors.New("unknown error"))
			})

			req := httptest.NewRequest("GET", "/", nil)
			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(500))
			b, _ := io.ReadAll(res.Body)
			Expect(b).To(MatchJSON(`{"type": "InternalError", "status": 500, "title": "Internal Error", "detail": "unknown error", "instance": ""}`))
		})

		It("should return a fiber error", func() {
			app.Get("/", func(c *fiber.Ctx) error {
				return server.ReturnWithProblem(c, nil, fiber.ErrMethodNotAllowed)
			})

			req := httptest.NewRequest("GET", "/", nil)
			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(405))
			b, _ := io.ReadAll(res.Body)
			Expect(b).To(MatchJSON(`{"type": "MethodNotAllowed", "status": 405, "title": "Method Not Allowed", "detail": "Method GET not allowed", "instance": ""}`))
		})

		It("should return a 404 error", func() {
			req := httptest.NewRequest("GET", "/", nil)
			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(404))
			b, _ := io.ReadAll(res.Body)
			Expect(b).To(MatchJSON(`{"type": "NotFound", "status": 404, "title": "Not found", "detail": "No resource found", "instance": ""}`))
		})

		It("should cast to a problem", func() {
			problem := problems.BadRequest("test")
			app.Get("/", func(c *fiber.Ctx) error {
				return server.ReturnWithProblem(c, nil, problem)
			})

			req := httptest.NewRequest("GET", "/", nil)
			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(problem.Code()))
			b, _ := io.ReadAll(res.Body)
			Expect(b).To(MatchJSON(`{"type": "BadRequest", "status": 400, "title": "Bad Request", "detail": "test", "instance": ""}`))
		})

	})
})
