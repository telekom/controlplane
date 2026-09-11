// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql_test

import (
	"context"
	"fmt"

	"entgo.io/ent/privacy"
	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/controlplane-api/ent"
	cpgraphql "github.com/telekom/controlplane/controlplane-api/internal/graphql"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ErrorPresenter", func() {
	newPathContext := func(field string) context.Context {
		return graphql.WithPathContext(context.Background(), graphql.NewPathWithField(field))
	}

	It("maps ent not found errors to NOT_FOUND", func() {
		gqlErr := cpgraphql.ErrorPresenter(newPathContext("testField"), &ent.NotFoundError{})
		Expect(gqlErr).To(HaveOccurred())
		Expect(gqlErr.Message).To(Equal("resource not found"))
		Expect(gqlErr.Extensions).To(HaveKeyWithValue("code", "NOT_FOUND"))
	})

	It("maps privacy deny errors to FORBIDDEN", func() {
		err := fmt.Errorf("policy denied: %w", privacy.Deny)
		gqlErr := cpgraphql.ErrorPresenter(newPathContext("testField"), err)
		Expect(gqlErr).To(HaveOccurred())
		Expect(gqlErr.Message).To(Equal("forbidden"))
		Expect(gqlErr.Extensions).To(HaveKeyWithValue("code", "FORBIDDEN"))
	})

	It("maps k8s conflicts to CONFLICT", func() {
		statusErr := apierrors.NewConflict(
			schema.GroupResource{Group: "organization", Resource: "teams"},
			"team-alpha",
			fmt.Errorf("resource version conflict"),
		)
		gqlErr := cpgraphql.ErrorPresenter(newPathContext("testField"), statusErr)
		Expect(gqlErr).To(HaveOccurred())
		Expect(gqlErr.Message).To(Equal("conflict"))
		Expect(gqlErr.Extensions).To(HaveKeyWithValue("code", "CONFLICT"))
	})

	It("sanitizes unknown errors to INTERNAL", func() {
		gqlErr := cpgraphql.ErrorPresenter(newPathContext("testField"), fmt.Errorf("db connection reset by peer"))
		Expect(gqlErr).To(HaveOccurred())
		Expect(gqlErr.Message).To(Equal("internal error while processing request"))
		Expect(gqlErr.Extensions).To(HaveKeyWithValue("code", "INTERNAL"))
	})

	It("passes through existing gqlerror values", func() {
		raw := &gqlerror.Error{
			Message: "graphql parse error",
			Extensions: map[string]any{
				"code": "GRAPHQL_PARSE_FAILED",
			},
		}
		gqlErr := cpgraphql.ErrorPresenter(newPathContext("testField"), raw)
		Expect(gqlErr).To(HaveOccurred())
		Expect(gqlErr.Message).To(Equal("graphql parse error"))
		Expect(gqlErr.Extensions).To(HaveKeyWithValue("code", "GRAPHQL_PARSE_FAILED"))
	})

	It("retains graphql path information", func() {
		err := apierrors.NewNotFound(schema.GroupResource{Resource: "teams"}, "team-alpha")
		gqlErr := cpgraphql.ErrorPresenter(newPathContext("updateTeam"), err)
		Expect(gqlErr).To(HaveOccurred())
		Expect(gqlErr.Path).NotTo(BeEmpty())
	})

	It("returns nil for nil errors", func() {
		Expect(cpgraphql.ErrorPresenter(newPathContext("testField"), nil)).To(Succeed())
	})

	Context("HttpError from external services", func() {
		It("sanitizes HttpError to INTERNAL without leaking details", func() {
			httpErr := client.RetryableErrorf("server error (500): {\"detail\":\"vault seal check failed\"}").WithStatusCode(500)
			gqlErr := cpgraphql.ErrorPresenter(newPathContext("clientSecret"), httpErr)
			Expect(gqlErr).To(HaveOccurred())
			Expect(gqlErr.Message).To(Equal("internal error while processing request"))
			Expect(gqlErr.Message).NotTo(ContainSubstring("vault"))
			Expect(gqlErr.Extensions).To(HaveKeyWithValue("code", "INTERNAL"))
		})

		It("sanitizes wrapped HttpError without leaking details", func() {
			httpErr := client.BlockedErrorf("bad request error (401): {\"detail\":\"Invalid token\"}").WithStatusCode(401)
			wrapped := fmt.Errorf("resolving secret clientSecret: %w", httpErr)
			gqlErr := cpgraphql.ErrorPresenter(newPathContext("clientSecret"), wrapped)
			Expect(gqlErr).To(HaveOccurred())
			Expect(gqlErr.Message).To(Equal("internal error while processing request"))
			Expect(gqlErr.Message).NotTo(ContainSubstring("token"))
			Expect(gqlErr.Extensions).To(HaveKeyWithValue("code", "INTERNAL"))
		})

		It("sanitizes blocked HttpError without leaking details", func() {
			httpErr := client.BlockedErrorf("resource not found").WithStatusCode(404)
			gqlErr := cpgraphql.ErrorPresenter(newPathContext("clientSecret"), httpErr)
			Expect(gqlErr).To(HaveOccurred())
			Expect(gqlErr.Message).To(Equal("internal error while processing request"))
			Expect(gqlErr.Extensions).To(HaveKeyWithValue("code", "INTERNAL"))
		})
	})
})
