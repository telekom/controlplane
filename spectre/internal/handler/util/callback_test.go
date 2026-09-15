// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"net/url"

	"github.com/telekom/controlplane/spectre/internal/handler/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BuildGatewayCallbackURL", func() {
	It("should set callback query parameter with bridge target", func() {
		result, err := util.BuildGatewayCallbackURL(
			"https://gateway.example.com/horizon/callback/v1",
			"http://localhost:8080/autoevent?listener=my-app",
		)
		Expect(err).ToNot(HaveOccurred())

		parsed, parseErr := url.Parse(result)
		Expect(parseErr).ToNot(HaveOccurred())
		Expect(parsed.Query().Get("callback")).To(Equal("http://localhost:8080/autoevent?listener=my-app"))
	})

	It("should set callback query parameter with external customer target", func() {
		result, err := util.BuildGatewayCallbackURL(
			"https://gateway.example.com/horizon/callback/v1",
			"https://customer.example.com/webhook",
		)
		Expect(err).ToNot(HaveOccurred())

		parsed, parseErr := url.Parse(result)
		Expect(parseErr).ToNot(HaveOccurred())
		Expect(parsed.Query().Get("callback")).To(Equal("https://customer.example.com/webhook"))
		Expect(parsed.Scheme).To(Equal("https"))
		Expect(parsed.Host).To(Equal("gateway.example.com"))
	})

	It("should preserve existing query parameters on gateway URL", func() {
		result, err := util.BuildGatewayCallbackURL(
			"https://gateway.example.com/cb?existing=value",
			"https://customer.example.com/hook",
		)
		Expect(err).ToNot(HaveOccurred())

		parsed, parseErr := url.Parse(result)
		Expect(parseErr).ToNot(HaveOccurred())
		Expect(parsed.Query().Get("existing")).To(Equal("value"))
		Expect(parsed.Query().Get("callback")).To(Equal("https://customer.example.com/hook"))
	})

	It("should preserve target query parameters with ampersand through round-trip", func() {
		target := "https://customer.example.com/callback?token=abc&format=json"
		result, err := util.BuildGatewayCallbackURL(
			"https://gateway.example.com/cb",
			target,
		)
		Expect(err).ToNot(HaveOccurred())

		parsed, parseErr := url.Parse(result)
		Expect(parseErr).ToNot(HaveOccurred())
		Expect(parsed.Query().Get("callback")).To(Equal(target))
	})

	It("should return error for invalid gateway URL", func() {
		_, err := util.BuildGatewayCallbackURL("://bad", "https://target.example.com")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid gateway callback URL"))
	})

	It("should return error for non-HTTP gateway URL", func() {
		_, err := util.BuildGatewayCallbackURL("ftp://gateway.example.com/cb", "https://target.example.com")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("must use http or https"))
	})

	It("should return error for gateway URL without host", func() {
		_, err := util.BuildGatewayCallbackURL("https:///path", "https://target.example.com")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("has no host"))
	})

	It("should return error for invalid target URL", func() {
		_, err := util.BuildGatewayCallbackURL("https://gateway.example.com/cb", "://bad")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid callback target"))
	})

	It("should return error for non-HTTP target URL", func() {
		_, err := util.BuildGatewayCallbackURL("https://gateway.example.com/cb", "ftp://target.example.com")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("must use http or https"))
	})

	It("should return error for target URL without host", func() {
		_, err := util.BuildGatewayCallbackURL("https://gateway.example.com/cb", "https:///path")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("has no host"))
	})
})
