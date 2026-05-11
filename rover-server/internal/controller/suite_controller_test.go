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
	cstore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/file-manager/api"
	filefake "github.com/telekom/controlplane/file-manager/api/fake"
	"github.com/telekom/controlplane/rover-server/internal/file"
	"k8s.io/client-go/rest"
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/stretchr/testify/mock"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/rover-server/internal/config"
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
var teamNoResources string
var app *fiber.App
var mockFileManager *filefake.MockFileManager
var stores *store.Stores

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var InitOrDie = func(ctx context.Context, cfg *rest.Config) {
	if mockObjectStore {
		stores = &store.Stores{}

		stores.RoverStore = mocks.NewRoverStoreMock(GinkgoT())
		stores.RoverSecretStore = stores.RoverStore
		stores.APISpecificationStore = mocks.NewAPISpecificationStoreMock(GinkgoT())
		stores.RoadmapStore = mocks.NewRoadmapStoreMock(GinkgoT())
		stores.APISubscriptionStore = mocks.NewAPISubscriptionStoreMock(GinkgoT())
		stores.APIExposureStore = mocks.NewAPIExposureStoreMock(GinkgoT())
		stores.ApplicationStore = mocks.NewApplicationStoreMock(GinkgoT())
		stores.ApplicationSecretStore = stores.ApplicationStore
		stores.ZoneStore = mocks.NewZoneStoreMock(GinkgoT())
		stores.EventSpecificationStore = mocks.NewEventSpecificationStoreMock(GinkgoT())
		stores.ApiChangelogStore = mocks.NewApiChangelogStoreMock(GinkgoT())

		eventExposureMock := mocks.NewMockObjectStore[*eventv1.EventExposure](GinkgoT())
		eventExposureMock.EXPECT().List(mock.Anything, mock.Anything).Return(
			&cstore.ListResponse[*eventv1.EventExposure]{Items: []*eventv1.EventExposure{}}, nil).Maybe()
		stores.EventExposureStore = eventExposureMock

		stores.EventSubscriptionStore = mocks.NewEventSubscriptionStoreMock(GinkgoT())

		eventConfigMock := mocks.NewMockObjectStore[*eventv1.EventConfig](GinkgoT())
		eventConfigMock.EXPECT().List(mock.Anything, mock.Anything).Return(
			&cstore.ListResponse[*eventv1.EventConfig]{Items: []*eventv1.EventConfig{}}, nil).Maybe()
		stores.EventConfigStore = eventConfigMock
	}

	mockFileManager = filefake.NewMockFileManager(GinkgoT())
	file.GetFileManager = func() api.FileManager {
		return mockFileManager
	}
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	// Initialize the test environment
	// This is where you would set up any necessary test data or configurations
	// For example, you might want to create a mock store or set up a test database connection

	var cfg *rest.Config
	if !mockObjectStore {
		cfg = kconfig.GetConfigOrDie()
	}
	InitOrDie(ctx, cfg)

	// TODO Add more tests with teamToken in apispecification, eventspecification, rover
	// Can be done once the issue with the team token is fixed in common-server
	teamToken = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:team:all"})
	groupToken = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:group:all"})
	teamNoResources = securitymock.NewMockAccessToken("poc", "eni", "nohyper", []string{"tardis:team:all"})

	// Create a new Fiber app
	app = cserver.NewApp()

	// Create a new server
	s := server.Server{
		Config:              &config.ServerConfig{},
		Log:                 log.Log,
		ApiSpecifications:   NewApiSpecificationController(stores),
		Rovers:              NewRoverController(stores),
		Roadmaps:            NewRoadmapController(stores),
		EventSpecifications: NewEventSpecificationController(stores),
		ApiChangelogs:       NewApiChangelogController(stores),
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
	b, err := io.ReadAll(response.Body)
	Expect(err).ToNot(HaveOccurred())
	snaps.MatchJSON(GinkgoT(), string(b), matchers...)
}
