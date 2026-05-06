// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/stretchr/testify/mock"
	storeLib "github.com/telekom/controlplane/common-server/pkg/store"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
)

// Pin the local time zone to UTC so JSON snapshots that contain
// metav1.Time values render consistently across developer machines
// (which may be in any zone) and CI (UTC). Without this, snapshots
// round-tripped through metav1.Time carry the local zone offset.
func init() {
	time.Local = time.UTC
}

const (
	mockObjectStore = true
)

var ctx context.Context
var stores *store.Stores

var InitOrDie = func(ctx context.Context, cfg *rest.Config) {
	if mockObjectStore {
		stores = &store.Stores{}

		stores.RoverStore = mocks.NewRoverStoreMock(GinkgoT())
		stores.APISpecificationStore = mocks.NewAPISpecificationStoreMock(GinkgoT())
		stores.APIStore = mocks.NewAPIStoreMock(GinkgoT())
		stores.APISubscriptionStore = mocks.NewAPISubscriptionStoreMock(GinkgoT())
		stores.APIExposureStore = mocks.NewAPIExposureStoreMock(GinkgoT())
		stores.ApplicationStore = mocks.NewApplicationStoreMock(GinkgoT())
		stores.ZoneStore = mocks.NewZoneStoreMock(GinkgoT())
		eventConfigMock := mocks.NewMockObjectStore[*eventv1.EventConfig](GinkgoT())
		stores.EventConfigStore = eventConfigMock

		eventSubscriptionMock := mocks.NewMockObjectStore[*eventv1.EventSubscription](GinkgoT())
		eventSubscriptionMock.EXPECT().List(mock.Anything, mock.Anything).Return(
			&storeLib.ListResponse[*eventv1.EventSubscription]{Items: []*eventv1.EventSubscription{}}, nil).Maybe()
		stores.EventSubscriptionStore = eventSubscriptionMock

		eventExposureMock := mocks.NewMockObjectStore[*eventv1.EventExposure](GinkgoT())
		eventExposureMock.EXPECT().List(mock.Anything, mock.Anything).Return(
			&storeLib.ListResponse[*eventv1.EventExposure]{Items: []*eventv1.EventExposure{}}, nil).Maybe()
		stores.EventExposureStore = eventExposureMock

		eventTypeMock := mocks.NewMockObjectStore[*eventv1.EventType](GinkgoT())
		eventTypeMock.EXPECT().List(mock.Anything, mock.Anything).Return(
			&storeLib.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{}}, nil).Maybe()
		stores.EventTypeStore = eventTypeMock
	}
}

func TestStatusMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Status Mapper Suite")
}

type ContextKey string

var _ = BeforeSuite(func() {
	ctx = context.WithValue(context.TODO(), ContextKey("test"), "test")

	By("bootstrapping test environment")
	// Initialize the test environment
	// This is where you would set up any necessary test data or configurations
	// For example, you might want to create a mock store or set up a test database connection

	var cfg *rest.Config
	if !mockObjectStore {
		cfg = config.GetConfigOrDie()
	}
	InitOrDie(ctx, cfg)

	rover = mocks.GetRover(GinkgoT(), mocks.RoverFileName)
})
