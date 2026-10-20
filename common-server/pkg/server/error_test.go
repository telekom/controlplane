// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/go-logr/zapr"
	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pkgerrors "github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/util/validation/field"
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

	Context("ReturnWithError logging", func() {

		var (
			app  *fiber.App
			logs []string
		)

		readBody := func(res *http.Response) []byte {
			b, err := io.ReadAll(res.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Body.Close()).To(Succeed())
			return b
		}

		BeforeEach(func() {
			logs = nil
			logger := funcr.New(func(prefix, args string) {
				logs = append(logs, args)
			}, funcr.Options{})
			app = server.NewApp()
			app.Use(func(c *fiber.Ctx) error {
				c.SetUserContext(logr.NewContext(context.Background(), logger))
				return c.Next()
			})
		})

		It("should log the field causes of a wrapped validation problem", func() {
			upstream := problems.ValidationErrors(map[string]string{
				"spec.specification": "Required value",
			})
			app.Get("/", func(c *fiber.Ctx) error {
				return server.ReturnWithProblem(c, nil, pkgerrors.Wrapf(upstream, "failed to create object %s/%s", "ns", "name"))
			})

			res, err := app.Test(httptest.NewRequest("GET", "/", http.NoBody))
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(400))
			Expect(readBody(res)).To(MatchJSON(`{"type": "ValidationError", "status": 400, "title": "Invalid Request", "detail": "One or more fields failed validation", "instance": "", "fields": [{"field": "spec.specification", "detail": "Required value"}]}`))

			Expect(logs).To(HaveLen(1))
			Expect(logs[0]).To(ContainSubstring(`"msg"="Returning error response"`))
			Expect(logs[0]).To(ContainSubstring("failed to create object ns/name: Invalid Request: One or more fields failed validation"))
			Expect(logs[0]).To(ContainSubstring(`"problemType"="ValidationError"`))
			Expect(logs[0]).To(ContainSubstring(`"status"=400`))
			Expect(logs[0]).To(ContainSubstring(`"problemFields"=[{"field"="spec.specification" "detail"="Required value"}]`))
		})

		It("should log full field messages and long field paths", func() {
			longPath := "spec.exposures[0]." + strings.Repeat("a", 1000)
			invalid := field.Invalid(field.NewPath("spec", "exposures").Index(1).Child("upstream"),
				"https://api.example.com/v1", "must use an allowed host")
			upstream := problems.ValidationErrors(map[string]string{
				invalid.Field: invalid.ErrorBody(),
				longPath:      "Required value",
			})
			app.Get("/", func(c *fiber.Ctx) error {
				return server.ReturnWithProblem(c, nil, pkgerrors.Wrap(upstream, "failed to create object ns/name"))
			})

			res, err := app.Test(httptest.NewRequest("GET", "/", http.NoBody))
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(400))
			Expect(string(readBody(res))).To(ContainSubstring("must use an allowed host"))

			Expect(logs).To(HaveLen(1))
			Expect(logs[0]).To(ContainSubstring(`{"field"="spec.exposures[1].upstream" "detail"="Invalid value: \"https://api.example.com/v1\": must use an allowed host"}`))
			Expect(logs[0]).To(ContainSubstring(`{"field"="` + longPath + `" "detail"="Required value"}`))
		})

		It("should log generic errors once without problem details", func() {
			app.Get("/", func(c *fiber.Ctx) error {
				return server.ReturnWithProblem(c, nil, errors.New("unknown error"))
			})

			res, err := app.Test(httptest.NewRequest("GET", "/", http.NoBody))
			Expect(err).ToNot(HaveOccurred())
			Expect(res.StatusCode).To(Equal(500))
			Expect(readBody(res)).To(MatchJSON(`{"type": "InternalError", "status": 500, "title": "Internal Error", "detail": "unknown error", "instance": ""}`))

			Expect(logs).To(HaveLen(1))
			Expect(logs[0]).To(ContainSubstring(`"error"="unknown error"`))
			Expect(logs[0]).ToNot(ContainSubstring("problemType"))
			Expect(logs[0]).ToNot(ContainSubstring("problemFields"))
		})
	})

	Context("ReturnWithError severity with zap", func() {

		var (
			app *fiber.App
			buf *bytes.Buffer
		)

		readEntries := func() []map[string]any {
			var entries []map[string]any
			dec := json.NewDecoder(buf)
			for dec.More() {
				entry := map[string]any{}
				Expect(dec.Decode(&entry)).To(Succeed())
				entries = append(entries, entry)
			}
			return entries
		}

		serve := func(err error) *http.Response {
			app.Get("/", func(c *fiber.Ctx) error {
				return server.ReturnWithError(c, err)
			})
			res, testErr := app.Test(httptest.NewRequest("GET", "/", http.NoBody))
			Expect(testErr).ToNot(HaveOccurred())
			return res
		}

		BeforeEach(func() {
			buf = &bytes.Buffer{}
			core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(buf), zapcore.DebugLevel)
			logger := zapr.NewLogger(zap.New(core, zap.AddStacktrace(zapcore.ErrorLevel)))
			app = server.NewApp()
			app.Use(func(c *fiber.Ctx) error {
				c.SetUserContext(logr.NewContext(context.Background(), logger))
				return c.Next()
			})
		})

		It("should log a wrapped 4xx problem at info level without stack trace", func() {
			upstream := problems.ValidationErrors(map[string]string{"spec.name": "Required value"})
			res := serve(pkgerrors.Wrap(upstream, "failed to create object ns/name"))
			Expect(res.StatusCode).To(Equal(400))

			entries := readEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0]).To(HaveKeyWithValue("level", "info"))
			Expect(entries[0]).To(HaveKeyWithValue("msg", "Returning error response"))
			Expect(entries[0]).To(HaveKeyWithValue("error", "failed to create object ns/name: Invalid Request: One or more fields failed validation"))
			Expect(entries[0]).To(HaveKeyWithValue("problemType", "ValidationError"))
			Expect(entries[0]).To(HaveKeyWithValue("status", BeNumerically("==", 400)))
			Expect(entries[0]).To(HaveKeyWithValue("problemFields", ConsistOf(HaveKeyWithValue("field", "spec.name"))))
			Expect(entries[0]).ToNot(HaveKey("errorVerbose"))
			Expect(entries[0]).ToNot(HaveKey("stacktrace"))
		})

		It("should log a fiber 4xx error at info level", func() {
			res := serve(fiber.ErrMethodNotAllowed)
			Expect(res.StatusCode).To(Equal(405))

			entries := readEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0]).To(HaveKeyWithValue("level", "info"))
			Expect(entries[0]).To(HaveKeyWithValue("error", "Method Not Allowed"))
		})

		It("should log a wrapped 5xx problem at error level with the original error", func() {
			res := serve(pkgerrors.Wrap(problems.InternalServerError("boom", "db down"), "failed to load"))
			Expect(res.StatusCode).To(Equal(500))

			entries := readEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0]).To(HaveKeyWithValue("level", "error"))
			Expect(entries[0]).To(HaveKeyWithValue("problemType", "InternalServerError"))
			Expect(entries[0]).To(HaveKeyWithValue("errorVerbose", ContainSubstring("error_test.go")))
			Expect(entries[0]).To(HaveKey("stacktrace"))
		})

		It("should log deadline exceeded at error level", func() {
			res := serve(pkgerrors.Wrap(context.DeadlineExceeded, "calling upstream"))
			Expect(res.StatusCode).To(Equal(503))

			entries := readEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0]).To(HaveKeyWithValue("level", "error"))
			Expect(entries[0]).To(HaveKey("errorVerbose"))
		})

		It("should log unmapped errors at error level", func() {
			res := serve(pkgerrors.New("unknown error"))
			Expect(res.StatusCode).To(Equal(500))

			entries := readEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0]).To(HaveKeyWithValue("level", "error"))
			Expect(entries[0]).To(HaveKeyWithValue("error", "unknown error"))
			Expect(entries[0]).To(HaveKey("errorVerbose"))
		})
	})
})
