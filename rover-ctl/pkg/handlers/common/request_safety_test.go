// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing/iotest"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/oauth2"

	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	secretMarker = "SYNTHETIC-SECRET-MARKER"
	// numericMarker can appear inside JSON number literals.
	numericMarker = "987654321123456789"
)

var _ = Describe("Request failure diagnostics", func() {
	var (
		logs        *bytes.Buffer
		logger      logr.Logger
		tokenServer *httptest.Server
		apiServer   *httptest.Server
		apiHandler  http.HandlerFunc
		tokenStatus int
		token       *config.Token
	)

	// render collects every representation of err that can reach the user.
	render := func(err error) string {
		var out bytes.Buffer
		common.PrintTextTo(err, &out)
		common.PrintJsonTo(err, &out)
		common.PrintTextTo(errors.Cause(err), &out)
		fmt.Fprintf(&out, "%v\n%+v\n%+v\n", err, err, errors.Cause(err))
		// Debug and normal mode logging as in cmd/root.go.
		logger.Error(err, "An error occurred")
		logger.Error(nil, "An error occurred", "error", errors.Cause(err).Error())
		return out.String() + logs.String()
	}

	tokenCtx := func() context.Context {
		return config.NewContext(context.Background(), token)
	}

	newHandler := func() *common.BaseHandler {
		h := common.NewBaseHandler("v1", "Test", "resources", 100)
		h.Setup(tokenCtx())
		return h
	}

	secretObject := func() types.Object {
		return &types.UnstructuredObject{Content: map[string]any{
			"apiVersion": "v1",
			"kind":       "Test",
			"metadata":   map[string]any{"name": "test-resource"},
			"spec":       map[string]any{"password": secretMarker + "-payload"},
		}}
	}

	BeforeEach(func() {
		logs = &bytes.Buffer{}
		core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(logs), zapcore.DebugLevel)
		logger = zapr.NewLogger(zap.New(core))
		previousLogger := log.L()
		log.SetGlobalLogger(logger)

		previousAccessToken := viper.Get("access.token")
		previousDebug := viper.Get("debug")
		viper.Set("access.token", "")
		DeferCleanup(func() {
			log.SetGlobalLogger(previousLogger)
			viper.Set("access.token", previousAccessToken)
			viper.Set("debug", previousDebug)
		})

		tokenStatus = http.StatusOK
		tokenServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(tokenStatus)
			if tokenStatus != http.StatusOK {
				fmt.Fprintf(w, `{"error":"invalid_client","error_description":"%s-oauth-description","error_uri":"https://x/%s-uri"}`, secretMarker, secretMarker)
				return
			}
			fmt.Fprint(w, `{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`)
		}))
		DeferCleanup(tokenServer.Close)

		apiHandler = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
		apiServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { apiHandler(w, r) }))
		DeferCleanup(apiServer.Close)

		token = &config.Token{
			ClientId:     "client-" + secretMarker,
			ClientSecret: secretMarker + "-client-secret",
			TokenUrl:     tokenServer.URL + "?q=" + secretMarker + "-token-query",
			ServerUrl:    apiServer.URL,
			Group:        "my-group",
			Team:         "my-team",
		}
	})

	It("sends the request payload unchanged without logging it", func() {
		var received []byte
		apiHandler = func(w http.ResponseWriter, r *http.Request) {
			received, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}
		obj := secretObject()

		for _, debug := range []bool{true, false} {
			viper.Set("debug", debug)
			received = nil
			logs.Reset()

			Expect(newHandler().Apply(tokenCtx(), obj)).To(Succeed())

			expected, err := json.Marshal(obj.GetContent())
			Expect(err).ToNot(HaveOccurred())
			Expect(received).To(MatchJSON(expected))
			Expect(logs.String()).ToNot(ContainSubstring(secretMarker+"-payload"), "payload logged with debug=%t", debug)
			Expect(logs.String()).ToNot(ContainSubstring(secretMarker), "debug=%t", debug)
			Expect(logs.String()).To(ContainSubstring(`"url":"` + apiServer.URL + `/resources/my-group--my-team--test-resource"`))
		}
	})

	It("reports invalid request URLs without the URL", func() {
		_, err := newHandler().Get(tokenCtx(), "bad%zz-"+secretMarker)

		var reqErr *common.RequestError
		Expect(errors.As(err, &reqErr)).To(BeTrue())
		Expect(reqErr.Category).To(Equal(common.RequestErrorInvalid))
		Expect(errors.Unwrap(err)).To(HaveOccurred())
		Expect(render(err)).ToNot(ContainSubstring(secretMarker))
	})

	It("logs request URLs and pagination cursors", func() {
		page := 0
		apiHandler = func(w http.ResponseWriter, r *http.Request) {
			page++
			next := ""
			if page == 1 {
				next = "page-2"
			}
			fmt.Fprintf(w, `{"links":{"next":%q},"items":[{"n":%d}]}`, next, page)
		}

		items, err := newHandler().List(tokenCtx())

		Expect(err).ToNot(HaveOccurred())
		Expect(items).To(HaveLen(2))
		Expect(logs.String()).To(ContainSubstring(`"url":"` + apiServer.URL + `/resources"`))
		Expect(logs.String()).To(ContainSubstring(`"url":"` + apiServer.URL + `/resources?cursor=page-2"`))
		Expect(logs.String()).To(ContainSubstring(`"cursor":"page-2"`))
	})

	It("logs the full status response", func() {
		apiHandler = func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"overallStatus":"blocked","processingState":"done","errors":[],"warnings":[],`+
				`"info":[{"cause":"Pending","message":"waiting for approval","resource":{"apiVersion":"v1","kind":"Approval","name":"a1","namespace":"ns"}}]}`)
		}

		status, err := newHandler().Status(tokenCtx(), "test-resource")

		Expect(err).ToNot(HaveOccurred())
		Expect(status.GetOverallStatus()).To(Equal(types.OverallStatusBlocked))
		Expect(logs.String()).To(ContainSubstring(`"msg":"Status response","status":{"overallStatus":"blocked","processingState":"done","errors":[],"warnings":[],` +
			`"info":[{"cause":"Pending","message":"waiting for approval","resource":{"apiVersion":"v1","kind":"Approval","name":"a1","namespace":"ns"}}]}`))
	})

	DescribeTable("reports undecodable responses without their content",
		func(call func(h *common.BaseHandler) error, body string) {
			apiHandler = func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }

			err := call(newHandler())

			var decodeErr *common.ResponseDecodeError
			Expect(errors.As(err, &decodeErr)).To(BeTrue())
			Expect(err.Error()).To(Equal("failed to parse resources response: invalid response body"))
			Expect(render(err)).ToNot(ContainSubstring(numericMarker))
		},
		Entry("get", func(h *common.BaseHandler) error {
			_, err := h.Get(tokenCtx(), "test-resource")
			return err
		}, `{"a":1e`+numericMarker+`}`),
		Entry("list", func(h *common.BaseHandler) error {
			_, err := h.List(tokenCtx())
			return err
		}, `{"items":[1e`+numericMarker+`]}`),
		Entry("status", func(h *common.BaseHandler) error {
			_, err := h.Status(tokenCtx(), "test-resource")
			return err
		}, `{"overallStatus":`+numericMarker+`}`),
		Entry("info", func(h *common.BaseHandler) error {
			h.SupportsInfo = true
			_, err := h.Info(tokenCtx(), "test-resource")
			return err
		}, `{"a":1e`+numericMarker+`}`),
	)

	It("reports body read failures without the reader error", func() {
		readErr := errors.New(secretMarker + "-read-error")
		var decoded map[string]any

		err := common.DecodeResponse("resources", iotest.ErrReader(readErr), &decoded)

		Expect(errors.Is(err, readErr)).To(BeTrue())
		Expect(render(err)).ToNot(ContainSubstring(secretMarker))
	})

	It("reports OAuth failures as a fixed authentication category", func() {
		tokenStatus = http.StatusUnauthorized

		err := newHandler().Apply(tokenCtx(), secretObject())

		var reqErr *common.RequestError
		Expect(errors.As(err, &reqErr)).To(BeTrue())
		Expect(reqErr.Category).To(Equal(common.RequestErrorAuthentication))
		Expect(reqErr.StatusCode).To(Equal(http.StatusUnauthorized))
		var retrieveErr *oauth2.RetrieveError
		Expect(errors.As(err, &retrieveErr)).To(BeTrue())
		Expect(err.Error()).To(Equal("PUT request for resources failed: authentication error (HTTP status 401)"))
		Expect(render(errors.Wrap(err, "failed to apply"))).ToNot(ContainSubstring(secretMarker))
	})

	It("reports connection failures without the request URL", func() {
		apiServer.Close()

		_, err := newHandler().Get(tokenCtx(), "test-resource")

		var reqErr *common.RequestError
		Expect(errors.As(err, &reqErr)).To(BeTrue())
		Expect(reqErr.Category).To(Equal(common.RequestErrorConnection))
		Expect(render(err)).ToNot(ContainSubstring(secretMarker))
	})

	DescribeTable("keeps context errors recognisable",
		func(makeCtx func() context.Context, category string, target error) {
			_, err := newHandler().Get(makeCtx(), "test-resource")

			var reqErr *common.RequestError
			Expect(errors.As(err, &reqErr)).To(BeTrue())
			Expect(reqErr.Category).To(Equal(category))
			Expect(errors.Is(err, target)).To(BeTrue())
			Expect(render(err)).ToNot(ContainSubstring(secretMarker))
		},
		Entry("canceled", func() context.Context {
			ctx, cancel := context.WithCancel(tokenCtx())
			cancel()
			return ctx
		}, common.RequestErrorCanceled, context.Canceled),
		Entry("deadline exceeded", func() context.Context {
			ctx, cancel := context.WithTimeout(tokenCtx(), 0)
			DeferCleanup(cancel)
			return ctx
		}, common.RequestErrorTimeout, context.DeadlineExceeded),
	)

	DescribeTable("omits remote error content",
		func(status int, body, expectedType, expectedTitle string) {
			apiHandler = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, body)
			}

			err := newHandler().Apply(tokenCtx(), secretObject())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Type).To(Equal(expectedType))
			Expect(apiErr.Status).To(Equal(status))
			Expect(apiErr.Title).To(Equal(expectedTitle))
			Expect(apiErr.Detail).To(Equal(fmt.Sprintf("The server responded with HTTP status %d. Response details are omitted.", status)))
			Expect(apiErr.Instance).To(BeEmpty())
			Expect(apiErr.Fields).To(BeEmpty())
			Expect(common.IsValidationError(err)).To(Equal(expectedType == "ValidationError"))
			Expect(render(err)).ToNot(ContainSubstring(secretMarker))
		},
		Entry("validation problem", http.StatusBadRequest,
			`{"type":"ValidationError","status":400,"title":"`+secretMarker+`-title","detail":"`+secretMarker+`-detail",`+
				`"instance":"/x?`+secretMarker+`","fields":[{"field":"`+secretMarker+`-field","detail":"`+secretMarker+`-field-detail"}]}`,
			"ValidationError", "Bad Request"),
		Entry("other problem type", http.StatusConflict,
			`{"type":"`+secretMarker+`-type","status":409,"title":"`+secretMarker+`-title","detail":"`+secretMarker+`"}`,
			"ApiError", "Conflict"),
		Entry("non-problem body", http.StatusInternalServerError, secretMarker+"-raw-body",
			"UnknownError", "Internal Server Error"),
		Entry("unknown status code", 599, secretMarker+"-raw-body",
			"UnknownError", "Unexpected Response"),
	)
})
