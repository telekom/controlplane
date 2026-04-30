// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/mock"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	securitymock "github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
	csstore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
	"github.com/telekom/controlplane/discovery-server/internal/config"
	"github.com/telekom/controlplane/discovery-server/internal/server"
	"github.com/telekom/controlplane/discovery-server/pkg/log"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
	"github.com/telekom/controlplane/discovery-server/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	obfuscToken    = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:team:obfuscated"})

	// groupOtherToken belongs to a different group ("other") – must be denied access to "eni" resources.
	groupOtherToken = securitymock.NewMockAccessToken("poc", "other", "someteam", []string{"tardis:group:all"})
	// teamPrefixToken has team="hyper" (a prefix of "hyperion") – must NOT match "hyperion" resources.
	teamPrefixToken = securitymock.NewMockAccessToken("poc", "eni", "hyper", []string{"tardis:team:all"})
	// groupPrefixToken has group="en" (a prefix of "eni") – must NOT match "eni" resources.
	groupPrefixToken = securitymock.NewMockAccessToken("poc", "en", "hyperion", []string{"tardis:group:all"})
)

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

// noopReplacer passes through objects unchanged. Used in tests where
// the secret-manager resolver is not needed (test data has no secret placeholders).
type noopReplacer struct{}

func (n *noopReplacer) ReplaceAll(_ context.Context, obj any, _ []string) (any, error) {
	return obj, nil
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")

	log.Init()

	// Create secret-wrapped stores that return deep copies (the obfuscator
	// modifies objects in place via JSON round-trip, so each call must
	// receive a fresh copy to avoid corrupting shared test data).
	apiExposure := mocks.GetApiExposure(GinkgoT(), "apiExposure.json")
	exposureSecretMock := csmocks.NewMockObjectStore[*apiv1.ApiExposure](GinkgoT())
	exposureSecretMock.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).RunAndReturn(func(_ context.Context, _, _ string) (*apiv1.ApiExposure, error) {
		return apiExposure.DeepCopy(), nil
	}).Maybe()
	exposureSecretMock.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).RunAndReturn(func(_ context.Context, _ csstore.ListOpts) (*csstore.ListResponse[*apiv1.ApiExposure], error) {
		return &csstore.ListResponse[*apiv1.ApiExposure]{Items: []*apiv1.ApiExposure{apiExposure.DeepCopy()}}, nil
	}).Maybe()

	apiSubscription := mocks.GetApiSubscription(GinkgoT(), "apiSubscription.json")
	subscriptionSecretMock := csmocks.NewMockObjectStore[*apiv1.ApiSubscription](GinkgoT())
	subscriptionSecretMock.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).RunAndReturn(func(_ context.Context, _, _ string) (*apiv1.ApiSubscription, error) {
		return apiSubscription.DeepCopy(), nil
	}).Maybe()
	subscriptionSecretMock.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).RunAndReturn(func(_ context.Context, _ csstore.ListOpts) (*csstore.ListResponse[*apiv1.ApiSubscription], error) {
		return &csstore.ListResponse[*apiv1.ApiSubscription]{Items: []*apiv1.ApiSubscription{apiSubscription.DeepCopy()}}, nil
	}).Maybe()

	stores = &sstore.Stores{
		ApplicationStore:           mocks.NewApplicationStoreMock(GinkgoT()),
		APIExposureStore:           mocks.NewAPIExposureStoreMock(GinkgoT()),
		APIExposureSecretStore:     secrets.WrapStore(exposureSecretMock, sstore.SecretsForKinds["ApiExposure"], &noopReplacer{}),
		APISubscriptionStore:       mocks.NewAPISubscriptionStoreMock(GinkgoT()),
		APISubscriptionSecretStore: secrets.WrapStore(subscriptionSecretMock, sstore.SecretsForKinds["ApiSubscription"], &noopReplacer{}),
		ZoneStore:                  mocks.NewZoneStoreMock(GinkgoT()),
		ApprovalStore:              mocks.NewApprovalStoreMock(GinkgoT()),
		EventExposureStore:         mocks.NewEventExposureStoreMock(GinkgoT()),
		EventSubscriptionStore:     mocks.NewEventSubscriptionStoreMock(GinkgoT()),
		EventTypeStore:             mocks.NewEventTypeStoreMock(GinkgoT()),
	}

	appCfg := cserver.NewAppConfig()
	appCfg.CtxLog = log.Log
	app = cserver.NewAppWithConfig(appCfg)

	s := server.Server{
		Config:             &config.ServerConfig{Security: config.SecurityConfig{Enabled: true}},
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

// ExpectStatusOk asserts HTTP 200 with application/json content type and returns the response body.
func ExpectStatusOk(response *http.Response, err error) string {
	expectNoError(err)
	expectResponseWithStatus(response, http.StatusOK, "application/json")
	return readBody(response)
}

// ExpectStatus asserts response status code and content type.
func ExpectStatus(response *http.Response, err error, statusCode int, contentType string) {
	expectNoError(err)
	expectResponseWithStatus(response, statusCode, contentType)
}

func expectNoError(err error) {
	Expect(err).ToNot(HaveOccurred())
	Expect(err).ToNot(HaveOccurred())
}

func expectResponseWithStatus(response *http.Response, statusCode int, contentType string) {
	Expect(response).ToNot(BeNil())
	Expect(response.StatusCode).To(Equal(statusCode))
	Expect(response.Header.Get("Content-Type")).To(Equal(contentType))
}

func readBody(response *http.Response) string {
	b, err := io.ReadAll(response.Body)
	Expect(err).ToNot(HaveOccurred())
	return string(b)
}
