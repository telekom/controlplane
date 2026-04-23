// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	storeLib "github.com/telekom/controlplane/common-server/pkg/store"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	mockObjectStore = true
)

var (
	ctx    context.Context
	rover  *roverv1.Rover
	stores *store.Stores
)

var InitOrDie = func(ctx context.Context, cfg *rest.Config) {
	if mockObjectStore {
		stores = &store.Stores{}

		stores.RoverStore = mocks.NewRoverStoreMock(GinkgoT())
		stores.RoverSecretStore = stores.RoverStore
		stores.APISpecificationStore = mocks.NewAPISpecificationStoreMock(GinkgoT())
		stores.APISubscriptionStore = mocks.NewAPISubscriptionStoreMock(GinkgoT())
		stores.APIExposureStore = mocks.NewAPIExposureStoreMock(GinkgoT())
		stores.ApplicationStore = mocks.NewApplicationStoreMock(GinkgoT())
		stores.ApplicationSecretStore = stores.ApplicationStore
		stores.ZoneStore = mocks.NewZoneStoreMock(GinkgoT())
		stores.EventSubscriptionStore = mocks.NewEventSubscriptionStoreMock(GinkgoT())

		eventExposureMock := mocks.NewMockObjectStore[*eventv1.EventExposure](GinkgoT())
		eventExposureMock.EXPECT().List(mock.Anything, mock.Anything).Return(
			&storeLib.ListResponse[*eventv1.EventExposure]{Items: []*eventv1.EventExposure{}}, nil).Maybe()
		eventExposureMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
		stores.EventExposureStore = eventExposureMock
	}
}

func TestApplicationInfoMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApplicationInfo Mapper Suite")
}

type ContextKey string

var _ = BeforeSuite(func() {
	ctx = context.WithValue(context.TODO(), ContextKey("test"), "test")

	By("bootstrapping test environment")
	// Initialize the test environment
	// This is where you would set up any necessary test data or configurations
	// For example, you might want to create a mock store or set up a test database connection

	InitOrDie(ctx, config.GetConfigOrDie())

	rover = mocks.GetRover(GinkgoT(), mocks.RoverFileName)
})
