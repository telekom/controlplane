// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/gkampitakis/go-snaps/match"
	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	securitymock "github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"

	"github.com/telekom/controlplane/spy-server/internal/config"
	"github.com/telekom/controlplane/spy-server/internal/server"
	"github.com/telekom/controlplane/spy-server/pkg/log"
	sstore "github.com/telekom/controlplane/spy-server/pkg/store"
	"github.com/telekom/controlplane/spy-server/test/mocks"
)

var (
	ctx    context.Context
	cancel context.CancelFunc
	app    *fiber.App
	stores *sstore.Stores

	teamToken      = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:team:all"})
	groupToken     = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:group:all"})
	adminToken     = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:admin:all"})
	teamNoResToken = securitymock.NewMockAccessToken("poc", "eni", "nohyper", []string{"tardis:team:all"})
)

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")

	log.Init()

	stores = &sstore.Stores{
		ApplicationStore:       mocks.NewApplicationStoreMock(GinkgoT()),
		APIExposureStore:       mocks.NewAPIExposureStoreMock(GinkgoT()),
		APISubscriptionStore:   mocks.NewAPISubscriptionStoreMock(GinkgoT()),
		ZoneStore:              mocks.NewZoneStoreMock(GinkgoT()),
		ApprovalStore:          mocks.NewApprovalStoreMock(GinkgoT()),
		EventExposureStore:     mocks.NewEventExposureStoreMock(GinkgoT()),
		EventSubscriptionStore: mocks.NewEventSubscriptionStoreMock(GinkgoT()),
		EventTypeStore:         mocks.NewEventTypeStoreMock(GinkgoT()),
	}

	appCfg := cserver.NewAppConfig()
	appCfg.CtxLog = log.Log
	app = cserver.NewAppWithConfig(appCfg)

	s := server.Server{
		Config:             &config.ServerConfig{},
		Log:                log.Log,
		Applications:       NewApplicationController(stores),
		ApiExposures:       NewApiExposureController(stores),
		ApiSubscriptions:   NewApiSubscriptionController(stores),
		EventExposures:     NewEventExposureController(stores),
		EventSubscriptions: NewEventSubscriptionController(stores),
		EventTypes:         NewEventTypeController(stores),
	}
	s.RegisterRoutes(app)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
})

// ExecuteRequest sends an HTTP request through the Fiber app with the given bearer token.
func ExecuteRequest(request *http.Request, bearerToken string) (*http.Response, error) {
	request.Header.Set("Authorization", "Bearer "+bearerToken)
	request.Header.Set("Content-Type", "application/json")
	return app.Test(request, -1)
}

// ExpectStatusOk asserts HTTP 200 with application/json content type and matches the body snapshot.
func ExpectStatusOk(response *http.Response, err error, matchers ...match.JSONMatcher) {
	expectNoError(err)
	expectResponseWithStatus(response, http.StatusOK, "application/json")
	expectResponseWithBody(response, matchers...)
}

// ExpectStatusWithBody asserts response status, content type, and matches the body snapshot.
func ExpectStatusWithBody(response *http.Response, err error, statusCode int, contentType string, matchers ...match.JSONMatcher) {
	ExpectStatus(response, err, statusCode, contentType)
	expectResponseWithBody(response, matchers...)
}

// ExpectStatus asserts response status code and content type.
func ExpectStatus(response *http.Response, err error, statusCode int, contentType string) {
	expectNoError(err)
	expectResponseWithStatus(response, statusCode, contentType)
}

func expectNoError(err error) {
	Expect(err).To(BeNil())
	Expect(err).ToNot(HaveOccurred())
}

func expectResponseWithStatus(response *http.Response, statusCode int, contentType string) {
	Expect(response).ToNot(BeNil())
	Expect(response.StatusCode).To(Equal(statusCode))
	Expect(response.Header.Get("Content-Type")).To(Equal(contentType))
}

func expectResponseWithBody(response *http.Response, matchers ...match.JSONMatcher) {
	b, err := io.ReadAll(response.Body)
	Expect(err).ToNot(HaveOccurred())
	snaps.MatchJSON(GinkgoT(), string(b), matchers...)
}
