// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	storeLib "github.com/telekom/controlplane/common-server/pkg/store"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
)

const (
	mockObjectStore = true
)

var ctx context.Context
var rover *roverv1.Rover

var InitOrDie = func(ctx context.Context, cfg *rest.Config) {
	if mockObjectStore {
		store.RoverStore = mocks.NewRoverStoreMock(GinkgoT())
		store.RoverSecretStore = store.RoverStore
		store.ApiSpecificationStore = mocks.NewApiSpecificationStoreMock(GinkgoT())
		store.ApiSubscriptionStore = mocks.NewApiSubscriptionStoreMock(GinkgoT())
		store.ApiExposureStore = mocks.NewApiExposureStoreMock(GinkgoT())
		store.ApplicationStore = mocks.NewApplicationStoreMock(GinkgoT())
		store.ApplicationSecretStore = store.ApplicationStore
		store.ZoneStore = mocks.NewZoneStoreMock(GinkgoT())
		store.EventSubscriptionStore = mocks.NewEventSubscriptionStoreMock(GinkgoT())

		eventExposureMock := mocks.NewMockObjectStore[*eventv1.EventExposure](GinkgoT())
		eventExposureMock.EXPECT().List(mock.Anything, mock.Anything).Return(
			&storeLib.ListResponse[*eventv1.EventExposure]{Items: []*eventv1.EventExposure{}}, nil).Maybe()
		eventExposureMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
		store.EventExposureStore = eventExposureMock
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
