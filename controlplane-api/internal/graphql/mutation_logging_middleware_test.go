// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql_test

import (
	"bytes"
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/vektah/gqlparser/v2/ast"

	cpgraphql "github.com/telekom/controlplane/controlplane-api/internal/graphql"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogMutationUser", func() {
	var (
		buf bytes.Buffer
		log logr.Logger
	)

	BeforeEach(func() {
		buf.Reset()
		log = funcr.New(func(prefix, args string) {
			buf.WriteString(args)
		}, funcr.Options{})
	})

	// withOperationContext sets up a gqlgen OperationContext on ctx.
	withOperationContext := func(ctx context.Context, opType ast.Operation, opName string) context.Context {
		oc := &graphql.OperationContext{
			OperationName: opName,
			Operation:     &ast.OperationDefinition{Operation: opType},
		}
		return graphql.WithOperationContext(ctx, oc)
	}

	noopNext := func(ctx context.Context) graphql.ResponseHandler {
		return func(ctx context.Context) *graphql.Response { return nil }
	}

	It("should log user name and email for mutations", func() {
		ctx := logr.NewContext(context.Background(), log)
		ctx = viewer.NewContext(ctx, &viewer.Viewer{
			UserName:  "Jane Doe",
			UserEmail: "jane@example.com",
		})
		ctx = withOperationContext(ctx, ast.Mutation, "CreateTeam")

		mw := cpgraphql.LogMutationUser()
		mw(ctx, noopNext)

		output := buf.String()
		Expect(output).To(ContainSubstring("Mutation requested"))
		Expect(output).To(ContainSubstring("CreateTeam"))
		Expect(output).To(ContainSubstring("Jane Doe"))
		Expect(output).To(ContainSubstring("jane@example.com"))
	})

	It("should not log for query operations", func() {
		ctx := logr.NewContext(context.Background(), log)
		ctx = viewer.NewContext(ctx, &viewer.Viewer{
			UserName:  "Jane Doe",
			UserEmail: "jane@example.com",
		})
		ctx = withOperationContext(ctx, ast.Query, "GetTeams")

		mw := cpgraphql.LogMutationUser()
		mw(ctx, noopNext)

		Expect(buf.String()).To(BeEmpty())
	})

	It("should handle nil viewer gracefully", func() {
		ctx := logr.NewContext(context.Background(), log)
		ctx = withOperationContext(ctx, ast.Mutation, "CreateTeam")

		mw := cpgraphql.LogMutationUser()
		mw(ctx, noopNext)

		output := buf.String()
		Expect(output).To(ContainSubstring("Mutation requested"))
		Expect(output).To(ContainSubstring("CreateTeam"))
	})
})
