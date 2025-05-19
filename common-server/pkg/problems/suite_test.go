// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package problems_test

import (
	"encoding/json"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/problems"
)

func TestProblems(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Problems Suite")
}

var _ = Describe("Problems", func() {
	BeforeEach(func() {

	})

	Context("Problem Builder", func() {
		It("should build a problem", func() {

			problem := problems.Builder().
				Title("Title").
				Detail("Detail").
				Type("Type").
				Instance("Instance").
				Status(400).
				Fields(problems.Field{Field: "field", Detail: "invalid"}).
				Build()

			b, err := json.Marshal(problem)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(b)).To(MatchJSON(`{"type":"Type","status":400,"title":"Title","detail":"Detail","instance":"Instance","fields":[{"field":"field","detail":"invalid"}]}`))

			Expect(problem.Error()).To(Equal("Title: Detail"))
			Expect(problem.Code()).To(Equal(400))
		})

		It("should create a problem from an exisiting errror", func() {

			err := errors.New("error message")
			problem := problems.NewProblemOfError(err)

			Expect(problem.Error()).To(Equal("Internal Error: error message"))
			Expect(problem.Code()).To(Equal(500))
		})
	})

	Context("Problem Known", func() {
		It("should create a not found problem", func() {
			problem := problems.NotFound("resource")
			Expect(problem.Error()).To(Equal("Not found: Resource resource not found"))
			Expect(problem.Code()).To(Equal(404))
			Expect(problems.IsNotFound(problem)).To(BeTrue())
		})

		It("should create a method not allowed problem", func() {
			problem := problems.MethodNotAllowed("method")
			Expect(problem.Error()).To(Equal("Method Not Allowed: Method method not allowed"))
			Expect(problem.Code()).To(Equal(405))
		})

		It("should create a validation error problem", func() {
			problem := problems.ValidationError("field", "detail")
			Expect(problem.Error()).To(Equal("Invalid Request: field: detail"))
			Expect(problem.Code()).To(Equal(400))
			Expect(problems.IsValidationError(problem)).To(BeTrue())

			b, err := json.Marshal(problem)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(b)).To(MatchJSON(`{"type":"ValidationError","status":400,"title":"Invalid Request","detail":"field: detail", "instance": ""}`))
		})

		It("should create a validation error problem with fields", func() {
			problem := problems.ValidationErrors(map[string]string{"field": "detail"})
			Expect(problem.Error()).To(Equal("Invalid Request: One or more fields failed validation"))
			Expect(problem.Code()).To(Equal(400))
			Expect(problems.IsValidationError(problem)).To(BeTrue())

			b, err := json.Marshal(problem)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(b)).To(MatchJSON(`
				{
					"type":"ValidationError",
					"status":400,
					"title":"Invalid Request",
					"detail":"One or more fields failed validation",
					"instance": "",
					"fields": [{"field":"field","detail":"detail"}]
				}
			`))

		})

		It("should create a bad request problem", func() {
			problem := problems.BadRequest("detail")
			Expect(problem.Error()).To(Equal("Bad Request: detail"))
			Expect(problem.Code()).To(Equal(400))
		})

		It("should create a unauthorized problem", func() {
			problem := problems.Unauthorized("Unauthorized", "detail")
			Expect(problem.Error()).To(Equal("Unauthorized: detail"))
			Expect(problem.Code()).To(Equal(401))
		})

		It("should create a forbidden problem", func() {
			problem := problems.Forbidden("Forbidden", "detail")
			Expect(problem.Error()).To(Equal("Forbidden: detail"))
			Expect(problem.Code()).To(Equal(403))
		})

		It("should create a conflict problem", func() {
			problem := problems.Conflict("detail")
			Expect(problem.Error()).To(Equal("Conflict: detail"))
			Expect(problem.Code()).To(Equal(409))
		})

		It("should create a internal server error problem", func() {
			problem := problems.InternalServerError("title", "detail")
			Expect(problem.Error()).To(Equal("title: detail"))
			Expect(problem.Code()).To(Equal(500))
		})
	})
})
