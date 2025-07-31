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
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/telekom/controlplane/rover-server/internal/server"
	"github.com/telekom/controlplane/rover-server/pkg/log"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
)

const (
	mockObjectStore = true
)

var ctx context.Context
var cancel context.CancelFunc
var teamToken string
var groupToken string
var app *fiber.App

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var InitOrDie = func(ctx context.Context, cfg *rest.Config) {
	if mockObjectStore {
		store.RoverStore = mocks.NewRoverStoreMock(GinkgoT())
		store.ApiSpecificationStore = mocks.NewApiSpecificationStoreMock(GinkgoT())
		store.ApiSubscriptionStore = mocks.NewApiSubscriptionStoreMock(GinkgoT())
		store.ApiExposureStore = mocks.NewApiExposureStoreMock(GinkgoT())
		store.ApplicationStore = mocks.NewApplicationStoreMock(GinkgoT())
		store.ZoneStore = mocks.NewZoneStoreMock(GinkgoT())
	}
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	// Initialize the test environment
	// This is where you would set up any necessary test data or configurations
	// For example, you might want to create a mock store or set up a test database connection

	InitOrDie(ctx, config.GetConfigOrDie())

	// TODO Add more tests with teamToken in apispecification, eventspecification, rover
	// Can be done once the issue with the team token is fixed in common-server
	teamToken = mock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:team:all"})
	groupToken = mock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:group:all"})

	// Create a new Fiber app
	app = cserver.NewApp()

	// Create a new server
	s := server.Server{
		Log:                 log.Log,
		ApiSpecifications:   NewApiSpecificationController(),
		Rovers:              NewRoverController(),
		EventSpecifications: NewEventSpecificationController(),
	}

	s.RegisterRoutes(app)

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
})

func ExecuteRequest(request *http.Request, bearerToken string) (*http.Response, error) {
	return ExecuteRequestWithToken(request, bearerToken)
}

func ExecuteRequestWithToken(request *http.Request, bearerToken string) (*http.Response, error) {
	request.Header.Set("Authorization", "Bearer "+bearerToken)
	request.Header.Set("Content-Type", "application/json")
	return app.Test(request, -1)
}

func ExpectStatus(response *http.Response, err error, statusCode int, contentType string) {
	expectNoError(err)
	expectResponseWithStatus(response, statusCode, contentType)
}

func ExpectStatusWithBody(response *http.Response, err error, statusCode int, contentType string, matchers ...match.JSONMatcher) {
	ExpectStatus(response, err, statusCode, contentType)
	expectResponseWithBody(response, matchers...)
}

func ExpectStatusNotImplemented(response *http.Response, err error) {
	expectNoError(err)
	expectResponseWithStatus(response, http.StatusNotImplemented, "application/problem+json")
}

func ExpectStatusOk(response *http.Response, err error, matchers ...match.JSONMatcher) {
	expectNoError(err)
	expectResponseWithStatus(response, http.StatusOK, "application/json")
	expectResponseWithBody(response, matchers...)
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
	b, _ := io.ReadAll(response.Body)
	snaps.MatchJSON(GinkgoT(), string(b), matchers...)
}
