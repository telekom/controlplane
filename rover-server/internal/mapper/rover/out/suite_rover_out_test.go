// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	storeLib "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
)

var (
	rover *roverv1.Rover

	apiExposure = roverv1.ApiExposure{
		Approval: roverv1.Approval{
			Strategy: "Simple",
		},
		BasePath: "/eni/distr/v1",
		Upstreams: []roverv1.Upstream{
			{
				URL: "https://httpbin.org/anything",
			},
		},
		Visibility: "World",
	}

	apiSubscription = roverv1.ApiSubscription{
		BasePath: "/eni/distr/v1",
	}

	eventExposure = roverv1.EventExposure{
		EventType: "tardis.horizon.demo.cetus.v1",
	}

	eventSubscription = roverv1.EventSubscription{
		EventType: "test-event",
	}
)

type ContextKey string

var ctx context.Context

func TestMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mapper Suite")
}

var _ = BeforeSuite(func() {
	ctx = context.WithValue(context.TODO(), ContextKey("test"), "test")

	// Initialize mock stores for sub-resource staleness checks used by MapRoverStatus.
	store.ApiSubscriptionStore = mocks.NewApiSubscriptionStoreMock(GinkgoT())
	store.ApiExposureStore = mocks.NewApiExposureStoreMock(GinkgoT())
	store.ApplicationStore = mocks.NewApplicationStoreMock(GinkgoT())

	eventExposureMock := mocks.NewMockObjectStore[*eventv1.EventExposure](GinkgoT())
	eventExposureMock.EXPECT().List(mock.Anything, mock.Anything).Return(
		&storeLib.ListResponse[*eventv1.EventExposure]{Items: []*eventv1.EventExposure{}}, nil).Maybe()
	store.EventExposureStore = eventExposureMock

	eventSubscriptionMock := mocks.NewMockObjectStore[*eventv1.EventSubscription](GinkgoT())
	eventSubscriptionMock.EXPECT().List(mock.Anything, mock.Anything).Return(
		&storeLib.ListResponse[*eventv1.EventSubscription]{Items: []*eventv1.EventSubscription{}}, nil).Maybe()
	store.EventSubscriptionStore = eventSubscriptionMock

	rover = mocks.GetRover(GinkgoT(), mocks.RoverFileName)
})

func GetRoverWithReadyCondition(rover *roverv1.Rover) *roverv1.Rover {
	ready := meta.FindStatusCondition(rover.Status.Conditions, condition.ConditionTypeReady)
	if ready != nil {
		ready.Status = metav1.ConditionTrue
	}
	return rover
}

func GetApiExposure(apiExposure *roverv1.ApiExposure) roverv1.Exposure {
	return roverv1.Exposure{
		Api: apiExposure,
	}
}

func GetApiSubscription(apiSubscription *roverv1.ApiSubscription) roverv1.Subscription {
	return roverv1.Subscription{
		Api: apiSubscription,
	}
}

func GetEventExposure(eventExposure *roverv1.EventExposure) roverv1.Exposure {
	return roverv1.Exposure{
		Event: eventExposure,
	}
}

func GetEventSubscription(eventSubscription *roverv1.EventSubscription) roverv1.Subscription {
	return roverv1.Subscription{
		Event: eventSubscription,
	}
}
