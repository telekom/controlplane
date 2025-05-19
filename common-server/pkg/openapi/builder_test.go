package openapi_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/openapi"
)

var _ = Describe("Builder Test", func() {
	BeforeEach(func() {

	})

	Context("Builder", func() {

		It("should return a valid OpenAPI document", func() {
			docBytes, err := openapi.NewDocumentBuilder().NewInfo("title", "description", "1.0.0").BuildBytes()
			Expect(err).ToNot(HaveOccurred())
			Expect(docBytes).ToNot(BeNil())
			Expect(docBytes).To(MatchJSON(`{"openapi":"3.0.0","info":{"title":"title","description":"description","version":"1.0.0"},"components":{}}`))
		})

		It("should add the server", func() {
			docBytes, err := openapi.NewDocumentBuilder().NewInfo("title", "description", "1.0.0").AddServer("http://localhost:8080", "test server").BuildBytes()
			Expect(err).ToNot(HaveOccurred())
			Expect(docBytes).ToNot(BeNil())
			Expect(docBytes).To(MatchJSON(`{"openapi":"3.0.0","info":{"title":"title","description":"description","version":"1.0.0"},"servers":[{"url":"http://localhost:8080","description":"test server"}],"components":{}}`))
		})

		It("should add a new path", func() {
			docBytes, err := openapi.NewDocumentBuilder().
				NewInfo("title", "description", "1.0.0").
				AddPath("/test", openapi.NewPathItemBuilder().
					SetOperation("GET", openapi.NewOperationBuilder().SetMeta("test", "testId").Build()).Build(),
				).BuildBytes()

			Expect(err).ToNot(HaveOccurred())
			Expect(docBytes).ToNot(BeNil())
			Expect(docBytes).To(MatchJSON(`{"openapi":"3.0.0","info":{"title":"title","description":"description","version":"1.0.0"},"paths":{"/test":{"get":{"summary":"test","operationId":"testId"}}},"components":{}}`))
		})

		It("should add a new path using the path-builder", func() {
			builder := openapi.NewDocumentBuilder().
				NewInfo("title", "description", "1.0.0")

			builder.AddNewPath("/test").
				SetOperation("GET", openapi.NewOperationBuilder().
					SetMeta("test", "testId").
					SetDescription("test description").
					AddTags("test").
					Build()).
				Build()

			docBytes, err := builder.BuildBytes()

			Expect(err).ToNot(HaveOccurred())
			Expect(docBytes).ToNot(BeNil())
			Expect(docBytes).To(MatchJSON(`{"openapi":"3.0.0","info":{"title":"title","description":"description","version":"1.0.0"}, "tags": [{"name": "test"}], "paths":{"/test":{"get":{"summary":"test","description":"test description","operationId":"testId","tags":["test"]}}},"components":{}}`))
		})

		It("should add a new schema item from bytes", func() {
			docBytes, err := openapi.NewDocumentBuilder().
				NewInfo("title", "description", "1.0.0").
				AddSchemaBytesOrDie("test", []byte(`{"type":"string"}`)).
				BuildBytes()

			Expect(err).ToNot(HaveOccurred())
			Expect(docBytes).ToNot(BeNil())
			Expect(docBytes).To(MatchJSON(`{"openapi":"3.0.0","info":{"title":"title","description":"description","version":"1.0.0"},"components":{"schemas":{"test":{"type":"string"}}}}`))
		})

		It("should add a response and a reference to it", func() {
			docBytes, err := openapi.NewDocumentBuilder().
				NewInfo("title", "description", "1.0.0").
				AddResponse("200", openapi.NewSchemaItemSchemaRef("test")).
				AddSchemaItem("test", map[string]interface{}{"type": "string"}).
				BuildBytes()

			Expect(err).ToNot(HaveOccurred())
			Expect(docBytes).ToNot(BeNil())
			Expect(docBytes).To(MatchJSON(`{"openapi":"3.0.0","info":{"title":"title","description":"description","version":"1.0.0"},"components":{"responses":{"200":{"$ref":"#/components/schemas/test"}},"schemas":{"test":{"type":"string"}}}}`))
		})
	})

	Context("OperationBuilder", func() {

		It("should return a valid operation", func() {
			op := openapi.NewOperationBuilder().
				SetMeta("test", "testId").
				SetDescription("test description").
				AddTags("test").
				SetJsonRequestBody(openapi.NewSchemaItemSchemaRef("test")).
				SetJsonResponse("200", openapi.NewSchemaItemSchemaRef("test")).
				Build()

			b, err := json.Marshal(op)
			Expect(err).ToNot(HaveOccurred())
			Expect(b).To(MatchJSON(`{"tags":["test"],"summary":"test","operationId":"testId","description":"test description","requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/test"}}}},"responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/test"}}}}}}`))
		})

		It("should return a valid operation with json request bytes", func() {
			op := openapi.NewOperationBuilder().
				SetJsonRequestBodyBytes([]byte(`{"type":"string"}`)).
				Build()

			b, err := json.Marshal(op)
			Expect(err).ToNot(HaveOccurred())
			Expect(b).To(MatchJSON(`{"summary":"","operationId":"","requestBody":{"content":{"application/json":{"schema":{"type":"string"}}}}}`))
		})

		It("should return a valid operation with json response bytes", func() {
			op := openapi.NewOperationBuilder().
				SetJsonResponseBytes("200", []byte(`{"type":"string"}`)).
				Build()

			b, err := json.Marshal(op)
			Expect(err).ToNot(HaveOccurred())
			Expect(b).To(MatchJSON(`{"summary":"","operationId":"","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"string"}}}}}}`))
		})

		It("should add a parmeter to the operation", func() {

		})
	})

	Context("ParameterBuilder", func() {

		It("should return a valid parameter", func() {
			param := openapi.NewParameterBuilder().
				SetName("test").
				SetIn("query").
				SetDescription("test description").
				SetRequired(true).
				SetSchema(map[string]interface{}{"type": "string"}).
				Build()

			b, err := json.Marshal(param)
			Expect(err).ToNot(HaveOccurred())
			Expect(b).To(MatchJSON(`{"name":"test","in":"query","description":"test description","required":true,"schema":{"type":"string"}}`))
		})

	})
})
