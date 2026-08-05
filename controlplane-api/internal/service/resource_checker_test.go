// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingBody struct {
	closed bool
}

func (*failingBody) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func (b *failingBody) Close() error {
	b.closed = true
	return errors.New("close failed")
}

var _ = Describe("RoverResourceChecker", func() {
	newChecker := func(basePath string, handler http.HandlerFunc) (*roverResourceChecker, *httptest.Server) {
		server := httptest.NewServer(handler)
		return &roverResourceChecker{
			baseURL:     server.URL + basePath,
			environment: "poc",
			scopePrefix: "tardis",
			httpClient:  server.Client(),
		}, server
	}

	It("requests one encoded resource and returns true for one item", func() {
		requestCount := 0
		checker, server := newChecker("", func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			Expect(r.URL.Path).To(Equal("/resources"))
			Expect(r.URL.Query().Get("group")).To(Equal("group & one"))
			Expect(r.URL.Query().Get("team")).To(Equal("team/one"))
			Expect(r.URL.Query().Get("limit")).To(Equal("1"))
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"items":[{}],"_links":{"next":"/resources?cursor=next"}}`))
			Expect(err).NotTo(HaveOccurred())
		})
		DeferCleanup(server.Close)

		hasResources, err := checker.HasResources(context.Background(), "group & one", "team/one")

		Expect(err).NotTo(HaveOccurred())
		Expect(hasResources).To(BeTrue())
		Expect(requestCount).To(Equal(1))
	})

	DescribeTable("joins the resources path to the base URL",
		func(basePath, expectedPath string) {
			requestCount := 0
			paths := make(chan string, 2)
			checker, server := newChecker(basePath, func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				paths <- r.URL.Path
				_, _ = w.Write([]byte(`{"items":[]}`))
			})
			DeferCleanup(server.Close)

			_, err := checker.HasResources(context.Background(), "group", "team")

			Expect(err).NotTo(HaveOccurred())
			Expect(<-paths).To(Equal(expectedPath))
			Expect(requestCount).To(Equal(1))
		},
		Entry("with a trailing slash", "/", "/resources"),
		Entry("with a base path", "/rover-api", "/rover-api/resources"),
	)

	It("returns false for an empty first page", func() {
		checker, server := newChecker("", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"items":[]}`))
			Expect(err).NotTo(HaveOccurred())
		})
		DeferCleanup(server.Close)

		hasResources, err := checker.HasResources(context.Background(), "group", "team")

		Expect(err).NotTo(HaveOccurred())
		Expect(hasResources).To(BeFalse())
	})

	It("trusts rover-server certificates from the configured CA bundle", func() {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(`{"items":[]}`))
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)

		certPath := filepath.Join(GinkgoT().TempDir(), "ca.pem")
		cert := server.Certificate()
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
		Expect(os.WriteFile(certPath, certPEM, 0o600)).To(Succeed())

		checker := NewRoverResourceChecker(server.URL, "poc", "tardis", certPath)
		hasResources, err := checker.HasResources(context.Background(), "group", "team")

		Expect(err).NotTo(HaveOccurred())
		Expect(hasResources).To(BeFalse())
	})

	It("rejects an oversized response", func() {
		checker, server := newChecker("", func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(strings.Repeat("x", 1024*1024+1)))
			Expect(err).NotTo(HaveOccurred())
		})
		DeferCleanup(server.Close)

		hasResources, err := checker.HasResources(context.Background(), "group", "team")

		Expect(hasResources).To(BeFalse())
		Expect(err).To(MatchError("rover-server response exceeds 1048576 bytes"))
	})

	It("closes the response body when reading fails", func() {
		body := &failingBody{}
		checker := &roverResourceChecker{
			baseURL:     "http://rover-server",
			environment: "poc",
			scopePrefix: "tardis",
			httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
			})},
		}

		hasResources, err := checker.HasResources(context.Background(), "group", "team")

		Expect(hasResources).To(BeFalse())
		Expect(err).To(MatchError("reading rover-server response: read failed"))
		Expect(body.closed).To(BeTrue())
	})

	DescribeTable("returns response errors",
		func(status int, body, errorMatch string) {
			checker, server := newChecker("", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, err := w.Write([]byte(body))
				Expect(err).NotTo(HaveOccurred())
			})
			DeferCleanup(server.Close)

			hasResources, err := checker.HasResources(context.Background(), "group", "team")

			Expect(hasResources).To(BeFalse())
			Expect(err).To(MatchError(ContainSubstring(errorMatch)))
		},
		Entry("for a non-200 status", http.StatusBadGateway, "upstream failed", "rover-server returned status 502"),
		Entry("for invalid JSON", http.StatusOK, "not-json", "decoding rover-server response"),
	)
})
