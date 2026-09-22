// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package emailutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/util/emailutil"
)

func TestEmail(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Email Policy Suite")
}

var _ = Describe("Email policy", func() {
	DescribeTable("folds ASCII only and is idempotent", func(input, expected string) {
		Expect(emailutil.Canonicalize(input)).To(Equal(expected))
		Expect(emailutil.Canonicalize(emailutil.Canonicalize(input))).To(Equal(expected))
	},
		Entry("ASCII and tags", "A.LICE+Tag@EXAMPLE.COM", "a.lice+tag@example.com"),
		Entry("Unicode unchanged", "ÜsER@EXAMPLE.COM", "Üser@example.com"),
		Entry("no trimming", " Alice@Example.COM ", " alice@example.com "),
		Entry("no Unicode normalization", "U\u0308SER@example.com", "u\u0308ser@example.com"),
	)

	DescribeTable("accepts bare syntax", func(email string) {
		Expect(emailutil.Validate(email)).To(Succeed())
	},
		Entry("mixed case", "Alice@Example.COM"),
		Entry("Unicode", "üser@example.com"),
		Entry("Unicode domain", "user@bücher.example"),
		Entry("punctuation", "a.!#$%&'*+-/=?^_`{|}~@example.com"),
		Entry("quoted spaces", `"alice smith"@example.com`),
		Entry("quoted at", `"alice@work"@example.com`),
		Entry("escaped quote", `"alice\"smith"@example.com`),
		Entry("quoted backslash", `"ali\\ce"@example.com`),
		Entry("quoted punctuation", `"a(b)<c>"@example.com`),
	)
	DescribeTable("rejects non-bare or malformed syntax", func(email string) {
		Expect(emailutil.Validate(email)).NotTo(Succeed())
	},
		Entry("empty", ""), Entry("no domain", "alice"),
		Entry("display name", "Alice <alice@example.com>"),
		Entry("angle address", "<alice@example.com>"),
		Entry("comment", "alice@example.com (Alice)"),
		Entry("leading space", " alice@example.com"),
		Entry("trailing space", "alice@example.com "),
		Entry("inner space", "alice smith@example.com"),
		Entry("newline", "alice@example.com\r\n"),
		Entry("quoted newline", "\"alice\r\nsmith\"@example.com"),
		Entry("list", "alice@example.com,bob@example.com"),
		Entry("group", "group:alice@example.com;"),
		Entry("unquoted backslash", `ali\ce@example.com`),
		Entry("double dot", "alice..smith@example.com"),
	)
	It("keeps Unicode case-distinct identities separate", func() {
		Expect(emailutil.Canonicalize("Üser@example.com")).NotTo(Equal(emailutil.Canonicalize("üser@example.com")))
	})
})
