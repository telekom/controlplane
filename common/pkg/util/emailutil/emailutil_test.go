// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package emailutil_test

import (
	"testing"

	"github.com/telekom/controlplane/common/pkg/util/emailutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEmail(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Email Policy Suite")
}

var _ = Describe("Email policy", func() {
	DescribeTable("applies Unicode lowercase and is idempotent", func(input, expected string) {
		Expect(emailutil.Canonicalize(input)).To(Equal(expected))
		Expect(emailutil.Canonicalize(emailutil.Canonicalize(input))).To(Equal(expected))
	},
		Entry("ASCII and tags", "A.LICE+Tag@EXAMPLE.COM", "a.lice+tag@example.com"),
		Entry("Unicode local part and domain", "ÜsER@BÜCHER.EXAMPLE", "üser@bücher.example"),
		Entry("not full case folding", "STRAẞE@EXAMPLE.COM", "straße@example.com"),
		Entry("quoted mailbox", `"ÜSER Name"@Example.COM`, `"üser name"@example.com`),
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
		Entry("slash", "alice/smith@example.com"),
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
		Entry("NUL", "ali\x00ce@example.com"),
		Entry("quoted NUL", "\"ali\x00ce\"@example.com"),
		Entry("quoted tab", "\"alice\tsmith\"@example.com"),
		Entry("escaped tab", "\"alice\\\tsmith\"@example.com"),
		Entry("ASCII control", "\"ali\x01ce\"@example.com"),
		Entry("DEL", "ali\x7fce@example.com"),
		Entry("Unicode control", "ali\u0080ce@example.com"),
		Entry("quoted Unicode control", "\"ali\u009fce\"@example.com"),
		Entry("unquoted Unicode whitespace", "alice\u00a0smith@example.com"),
		Entry("list", "alice@example.com,bob@example.com"),
		Entry("group", "group:alice@example.com;"),
		Entry("unquoted backslash", `ali\ce@example.com`),
		Entry("double dot", "alice..smith@example.com"),
	)
	It("matches Unicode lowercase equivalents without full case folding", func() {
		Expect(emailutil.Canonicalize("Üser@example.com")).To(Equal(emailutil.Canonicalize("üser@example.com")))
		Expect(emailutil.Canonicalize("straße@example.com")).NotTo(Equal(emailutil.Canonicalize("STRASSE@example.com")))
	})
})
