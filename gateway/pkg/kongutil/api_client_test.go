// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	ptls "github.com/telekom/controlplane/common-server/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const issuer = "https://issuer.example/realms/r"

type recordedRequest struct {
	path          string
	authorization string
	deadline      time.Time
}

// recordingTransport answers token and API requests and records each request it sees.
type recordingTransport struct {
	mu            sync.Mutex
	requests      []recordedRequest
	redirectToken bool
}

func (t *recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	deadline, _ := r.Context().Deadline()
	t.mu.Lock()
	t.requests = append(t.requests, recordedRequest{path: r.URL.Path, authorization: r.Header.Get("Authorization"), deadline: deadline})
	t.mu.Unlock()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}
	if r.URL.Path == "/realms/r"+tokenURLPath {
		if t.redirectToken {
			resp.StatusCode = http.StatusFound
			resp.Header.Set("Location", "https://elsewhere.example/token")
			return resp, nil
		}
		resp.Header.Set("Content-Type", "application/json")
		resp.Body = io.NopCloser(strings.NewReader(`{"access_token":"tok","token_type":"Bearer","expires_in":3600}`))
	}
	return resp, nil
}

func (t *recordingTransport) recorded() []recordedRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]recordedRequest(nil), t.requests...)
}

var _ = Describe("newAPIClient", func() {
	It("fetches the token without the access token over the shared transport within the request timeout", func() {
		transport := &recordingTransport{}
		api := newAPIClient(context.Background(), transport, issuer, "cid", "secret")

		start := time.Now()
		resp, err := api.Get("https://api.example/consumers/c")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())

		requests := transport.recorded()
		Expect(requests).To(HaveLen(2))
		Expect(requests[0].path).To(Equal("/realms/r" + tokenURLPath))
		Expect(requests[0].authorization).NotTo(HavePrefix("Bearer"))
		Expect(requests[1].path).To(Equal("/consumers/c"))
		Expect(requests[1].authorization).To(Equal("Bearer tok"))
		for _, r := range requests {
			Expect(r.deadline).To(BeTemporally("~", start.Add(RequestTimeout), time.Second))
		}
	})

	It("denies token endpoint redirects without calling the API", func() {
		transport := &recordingTransport{redirectToken: true}
		api := newAPIClient(context.Background(), transport, issuer, "cid", "secret")

		_, err := api.Get("https://api.example/consumers/c")
		Expect(err).To(MatchError(ContainSubstring(ptls.ErrRedirectNotAllowed.Error())))
		requests := transport.recorded()
		Expect(requests).NotTo(BeEmpty())
		for _, r := range requests {
			Expect(r.path).To(Equal("/realms/r"+tokenURLPath), "the API must not be called without a token")
		}
	})
})
