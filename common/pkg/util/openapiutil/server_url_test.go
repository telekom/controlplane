// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package openapiutil_test

import (
	"github.com/telekom/controlplane/common/pkg/util/openapiutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PathFromURL", func() {
	DescribeTable("extracts the path",
		func(rawURL, expected string) {
			Expect(openapiutil.PathFromURL(rawURL)).To(Equal(expected))
		},
		Entry("absolute", "https://example.com/eni/api/v1", "/eni/api/v1"),
		Entry("templated authority without declarations", "https://{host}:{port}/eni/api/v1?x=1#f", "/eni/api/v1"),
		Entry("protocol-relative", "//{host}/eni/api/v1", "/eni/api/v1"),
		Entry("root-relative", "/eni/api/v1", "/eni/api/v1"),
		Entry("relative", "eni/api/v1", "eni/api/v1"),
		Entry("missing path", "https://{host}", ""),
		Entry("empty", "", ""),
		Entry("only query and fragment", "?a={x}#{y}", ""),
		Entry("percent-encodings unchanged", "https://h/eni/my%20api%2Fx%7bv%7D", "/eni/my%20api%2Fx%7bv%7D"),
		Entry("percent-encodings unchanged without authority", "/a%20b%2f%7B", "/a%20b%2f%7B"),
		Entry("reserved path chars", "https://h/a:b@c!$&'()*+,;=-._~", "/a:b@c!$&'()*+,;=-._~"),
		Entry("repeated slashes and dot segments", "https://h//eni/./api/../V1/", "//eni/./api/../V1/"),
		Entry("protocol-relative with repeated slashes", "//h//a/./b/../C/", "//a/./b/../C/"),
		Entry("scheme without authority", "https:/eni/api/v1?x=1", "/eni/api/v1"),
		Entry("colon after first slash is path", "/eni/a://b", "/eni/a://b"),
		Entry("templated scheme", "{scheme}://{host}/eni/api", "/eni/api"),
	)

	DescribeTable("rejects invalid paths",
		func(rawURL, msg string) {
			_, err := openapiutil.PathFromURL(rawURL)
			Expect(err).To(MatchError(ContainSubstring(msg)))
		},
		Entry("variable in path", "https://{host}/eni/{version}", "must not contain variables"),
		Entry("whole url variable", "{endpoint}", "must not contain variables"),
		Entry("space", "https://h/eni/my api", "invalid character"),
		Entry("tab", "/eni/\tapi", "invalid character"),
		Entry("control char", "/eni/\x01api", "invalid character"),
		Entry("non-ascii", "/eni/äpi", "invalid character"),
		Entry("backslash", "/eni\\api", "invalid character"),
		Entry("quote", "/eni/\"api", "invalid character"),
		Entry("bad hex escape", "https://h/eni/%zz", "malformed percent-encoding"),
		Entry("truncated escape", "/eni/%2", "malformed percent-encoding"),
	)

	It("formats errors with the path and cause", func() {
		_, err := openapiutil.PathFromURL("https://h/eni/%zz")
		Expect(err).To(MatchError(`invalid server url path "/eni/%zz": malformed percent-encoding at position 5`))

		_, err = openapiutil.PathFromURL("https://{host}/eni/{v}")
		Expect(err).To(MatchError(`server url path "/eni/{v}" must not contain variables`))
	})
})
