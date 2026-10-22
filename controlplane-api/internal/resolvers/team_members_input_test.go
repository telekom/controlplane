// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package resolvers

import (
	"context"

	"github.com/99designs/gqlgen/graphql"

	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpdateTeam membership input", func() {
	DescribeTable("distinguishes preservation from replacement", func(input map[string]any, expected []model.MemberInput) {
		ctx := graphql.WithOperationContext(context.Background(), &graphql.OperationContext{})
		ec := executionContext{}
		result, err := ec.unmarshalInputUpdateTeamInput(ctx, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Members).To(Equal(expected))
	},
		Entry("omitted", map[string]any{"teamId": "10"}, nil),
		Entry("null", map[string]any{"teamId": "10", "members": nil}, nil),
		Entry("empty", map[string]any{"teamId": "10", "members": []any{}}, []model.MemberInput{}),
		Entry("replacement", map[string]any{"teamId": "10", "members": []any{map[string]any{"name": "Bob", "email": "bob@example.com"}}}, []model.MemberInput{{Name: "Bob", Email: "bob@example.com"}}),
	)
})
