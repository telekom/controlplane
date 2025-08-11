// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

var ctx context.Context
var cancel context.CancelFunc

func TestMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mapper Suite")
}

var _ = BeforeSuite(func() {
	rover = mocks.GetRover(GinkgoT(), mocks.RoverFileName)
	ctx, cancel = context.WithCancel(context.TODO())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
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
