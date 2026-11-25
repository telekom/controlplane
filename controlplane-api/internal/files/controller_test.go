// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/gofiber/fiber/v2"

	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/controlplane-api/internal/files"
	"github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/file-manager/api/constants"
	"github.com/telekom/controlplane/file-manager/api/fake"
)

const (
	validUUID    = "0190b2a4-7f3c-7a52-8c1e-3d4f5a6b7c8d"
	legacyFileID = "env--group--team--name.yaml"
)

type logSink struct {
	mu  sync.Mutex
	all []string
}

func (l *logSink) matching(substr string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, e := range l.all {
		if strings.Contains(e, substr) {
			out = append(out, e)
		}
	}
	return out
}

func (l *logSink) logger() logr.Logger {
	return funcr.New(func(prefix, args string) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.all = append(l.all, prefix+args)
	}, funcr.Options{})
}

func newApp(ctrl *files.Controller, guard []fiber.Handler, log logr.Logger) *fiber.App {
	cfg := cserver.NewAppConfig()
	cfg.EnableMetrics = false
	cfg.CtxLog = log
	app := cserver.NewAppWithConfig(cfg)
	ctrl.RegisterRoutes(app, guard)
	return app
}

func do(app *fiber.App, req *http.Request) (*http.Response, []byte) {
	resp, err := app.Test(req, -1)
	Expect(err).NotTo(HaveOccurred())
	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.Body.Close()).To(Succeed())
	return resp, body
}

func unsignedJWT(exp time.Time) string {
	enc := base64.RawURLEncoding.EncodeToString
	header := enc([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := enc([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix())))
	return header + "." + payload + "."
}

var _ = Describe("Files download proxy", func() {
	var (
		fm   *fake.MockFileManager
		app  *fiber.App
		sink *logSink
	)

	BeforeEach(func() {
		fm = fake.NewMockFileManager(GinkgoT())
		sink = &logSink{}
		denyAll := []fiber.Handler{func(c *fiber.Ctx) error { return fiber.ErrUnauthorized }}
		app = newApp(files.NewController(fm), denyAll, sink.logger())
	})

	It("AC-1: serves the file anonymously with Content-Type and ETag", func() {
		fm.EXPECT().DownloadFile(mock.Anything, validUUID, mock.Anything).
			RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*api.FileDownloadResponse, error) {
				_, err := w.Write([]byte("openapi: 3.0.0"))
				return &api.FileDownloadResponse{FileHash: "abc/+=", ContentType: "application/yaml"}, err
			}).Once()

		resp, body := do(app, httptest.NewRequest(http.MethodGet, "/files/v1/"+validUUID, nil))

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(string(body)).To(Equal("openapi: 3.0.0"))
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/yaml"))
		Expect(resp.Header.Get("ETag")).To(Equal(`"abc/+="`))
		Expect(resp.Header.Get("X-Content-Type-Options")).To(Equal("nosniff"))
		Expect(resp.Header.Get("Content-Security-Policy")).To(ContainSubstring("sandbox"))
	})

	It("AC-1: serves legacy ids and falls back to application/octet-stream", func() {
		fm.EXPECT().DownloadFile(mock.Anything, legacyFileID, mock.Anything).
			RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*api.FileDownloadResponse, error) {
				_, err := w.Write([]byte("x"))
				return &api.FileDownloadResponse{FileHash: "h"}, err
			}).Once()

		resp, body := do(app, httptest.NewRequest(http.MethodGet, "/files/v1/"+legacyFileID, nil))

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(string(body)).To(Equal("x"))
		Expect(resp.Header.Get("Content-Type")).To(Equal(constants.DefaultContentType))
	})

	It("AC-3: answers 404 problem when file-manager does not know the id", func() {
		fm.EXPECT().DownloadFile(mock.Anything, validUUID, mock.Anything).Return(nil, api.ErrNotFound).Once()

		resp, body := do(app, httptest.NewRequest(http.MethodGet, "/files/v1/"+validUUID, nil))

		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))
		var problem map[string]any
		Expect(json.Unmarshal(body, &problem)).To(Succeed())
		Expect(problem["status"]).To(BeNumerically("==", http.StatusNotFound))
		Expect(sink.matching("Failed to download")).To(BeEmpty())
	})

	DescribeTable("AC-4: answers 400 problem for malformed ids without calling file-manager",
		func(path string) {
			resp, body := do(app, httptest.NewRequest(http.MethodGet, path, nil))

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			Expect(string(body)).To(ContainSubstring("invalid file id"))
			fm.AssertNotCalled(GinkgoT(), "DownloadFile", mock.Anything, mock.Anything, mock.Anything)
		},
		Entry("path traversal", "/files/v1/..%2Fx"),
		Entry("uppercase UUID", "/files/v1/0190B2A4-7F3C-7A52-8C1E-3D4F5A6B7C8D"),
		Entry("too few legacy parts", "/files/v1/a--b--c"),
		Entry("dot-dot in legacy id", "/files/v1/a--..--c--d"),
		Entry("encoded dots", "/files/v1/%252e%252e"),
		Entry("plain word", "/files/v1/readme"),
		Entry("overlong id", "/files/v1/a--b--c--"+strings.Repeat("d", 600)),
	)

	It("AC-4: answers 400 for an empty or nested id without calling file-manager", func() {
		resp, _ := do(app, httptest.NewRequest(http.MethodGet, "/files/v1/", nil))
		Expect(resp.StatusCode).To(BeNumerically(">=", 400))
		resp, _ = do(app, httptest.NewRequest(http.MethodGet, "/files/v1/a/b", nil))
		Expect(resp.StatusCode).To(BeNumerically(">=", 400))
		fm.AssertNotCalled(GinkgoT(), "DownloadFile", mock.Anything, mock.Anything, mock.Anything)
	})

	DescribeTable("AC-5: answers 502 problem without partial body and logs once",
		func(upstreamErr error) {
			fm.EXPECT().DownloadFile(mock.Anything, validUUID, mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*api.FileDownloadResponse, error) {
					_, _ = w.Write([]byte("PARTIAL-SECRET-BYTES"))
					return nil, upstreamErr
				}).Once()

			resp, body := do(app, httptest.NewRequest(http.MethodGet, "/files/v1/"+validUUID, nil))

			Expect(resp.StatusCode).To(Equal(http.StatusBadGateway))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			Expect(string(body)).NotTo(ContainSubstring("PARTIAL-SECRET-BYTES"))
			Expect(string(body)).NotTo(ContainSubstring(upstreamErr.Error()))
			var problem map[string]any
			Expect(json.Unmarshal(body, &problem)).To(Succeed())
			Expect(problem["status"]).To(BeNumerically("==", http.StatusBadGateway))

			failures := sink.matching("Failed to download file from file-manager")
			Expect(failures).To(HaveLen(1))
			Expect(failures[0]).To(ContainSubstring(validUUID))
			Expect(failures[0]).NotTo(ContainSubstring("Bearer"))
		},
		Entry("upstream 5xx", errors.New("unexpected http error (500): file-manager: internal: boom")),
		Entry("transport error", errors.New("dial tcp 10.0.0.1:443: connection refused")),
		Entry("checksum mismatch", errors.New("checksum mismatch: expected a, got b")),
	)

	DescribeTable("AC-6: does not proxy non-GET methods",
		func(method string) {
			resp, _ := do(app, httptest.NewRequest(method, "/files/v1/"+validUUID, nil))

			Expect(resp.StatusCode).To(BeNumerically(">=", 400))
			Expect(resp.StatusCode).NotTo(Equal(http.StatusOK))
			fm.AssertNotCalled(GinkgoT(), "DownloadFile", mock.Anything, mock.Anything, mock.Anything)
		},
		Entry("POST", http.MethodPost),
		Entry("PUT", http.MethodPut),
		Entry("PATCH", http.MethodPatch),
		Entry("DELETE", http.MethodDelete),
		Entry("HEAD", http.MethodHead),
	)

	It("AC-6: answers 405 problem for POST", func() {
		resp, body := do(app, httptest.NewRequest(http.MethodPost, "/files/v1/"+validUUID, nil))
		Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))
		Expect(string(body)).NotTo(BeEmpty())
	})
})

var _ = Describe("Files download proxy against an upstream file-manager", func() {
	It("AC-2: authenticates with the token file and never forwards the client token", func() {
		var gotPath, gotAuth string
		upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			sum := sha256.Sum256([]byte("hello"))
			w.Header().Set(constants.XFileChecksum, base64.StdEncoding.EncodeToString(sum[:]))
			w.Header().Set(constants.XFileContentType, "application/yaml")
			_, _ = w.Write([]byte("hello"))
		}))
		DeferCleanup(upstream.Close)

		fileToken := unsignedJWT(time.Now().Add(time.Hour))
		tokenFile := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenFile, []byte(fileToken), 0o600)).To(Succeed())

		client := files.NewFileManagerClient(upstream.URL, tokenFile, api.WithSkipTLSVerify(true))
		app := newApp(files.NewController(client), nil, logr.Discard())

		req := httptest.NewRequest(http.MethodGet, "/files/v1/"+validUUID, nil)
		req.Header.Set("Authorization", "Bearer client-token")
		resp, body := do(app, req)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(string(body)).To(Equal("hello"))
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/yaml"))
		Expect(gotPath).To(Equal("/v1/files/" + validUUID))
		Expect(gotAuth).To(Equal("Bearer " + fileToken))
		Expect(gotAuth).NotTo(ContainSubstring("client-token"))
	})

	It("AC-5: answers 502 on a checksum mismatch without leaking the body", func() {
		upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set(constants.XFileChecksum, "not-the-real-checksum")
			_, _ = w.Write([]byte("TAMPERED"))
		}))
		DeferCleanup(upstream.Close)

		tokenFile := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenFile, []byte(unsignedJWT(time.Now().Add(time.Hour))), 0o600)).To(Succeed())

		client := files.NewFileManagerClient(upstream.URL, tokenFile, api.WithSkipTLSVerify(true))
		app := newApp(files.NewController(client), nil, logr.Discard())

		resp, body := do(app, httptest.NewRequest(http.MethodGet, "/files/v1/"+validUUID, nil))

		Expect(resp.StatusCode).To(Equal(http.StatusBadGateway))
		Expect(string(body)).NotTo(ContainSubstring("TAMPERED"))
	})
})
