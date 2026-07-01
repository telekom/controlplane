// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
)

var (
	apiRover           *api.Rover
	roverUpdateRequest *api.RoverUpdateRequest

	apiExposure = api.ApiExposure{
		Approval:   "Simple",
		BasePath:   "/eni/distr/v1",
		Upstream:   "https://httpbin.org/anything",
		Visibility: "World",
	}

	apiSubscription = api.ApiSubscription{
		BasePath: "/eni/distr/v1",
	}

	eventExposure = api.EventExposure{
		Approval:      "Simple",
		EventCategory: "SYSTEM",
		EventType:     "tardis.horizon.demo.cetus.v1",
		Visibility:    "World",
	}

	eventSubscription = api.EventSubscription{
		EventType: "test-event",
	}

	fileExposure = api.FileExposure{
		Type:       "file",
		FileType:   "demo-sftp-spec-v1",
		Variant:    "sftp",
		Visibility: "World",
		PublicKeys: []api.PublicKey{
			{Label: "provider-key", Key: "ssh-ed25519 AAAA-provider"},
		},
	}

	fileSubscription = api.FileSubscription{
		Type:     "file",
		FileType: "demo-sftp-spec-v1",
		Variant:  "sftp",
		PublicKeys: []api.PublicKey{
			{Label: "consumer-key", Key: "ssh-ed25519 AAAA-consumer"},
		},
	}

	resourceIdInfo = mapper.ResourceIdInfo{
		Name:        "rover-local-sub",
		Environment: "poc",
		Namespace:   "poc--eni--hyperion",
	}
)

func TestMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mapper Suite")
}

var _ = BeforeSuite(func() {
	apiRover = &api.Rover{
		Zone:          "zone",
		Exposures:     []api.Exposure{GetApiExposure(apiExposure)},
		Subscriptions: []api.Subscription{GetApiSubscription(apiSubscription)},
	}

	roverUpdateRequest = &api.RoverUpdateRequest{
		Zone:          apiRover.Zone,
		Exposures:     apiRover.Exposures,
		Subscriptions: apiRover.Subscriptions,
	}
})

func GetApiExposure(apiExposure api.ApiExposure) api.Exposure {
	var exp api.Exposure
	err := (&exp).FromApiExposure(apiExposure)
	Expect(err).To(BeNil())
	return exp
}

func GetApiSubscription(apiSubscription api.ApiSubscription) api.Subscription {
	var sub api.Subscription
	err := (&sub).FromApiSubscription(apiSubscription)
	Expect(err).To(BeNil())
	return sub
}

func GetEventExposure(eventExposure api.EventExposure) api.Exposure {
	var exp api.Exposure
	err := (&exp).FromEventExposure(eventExposure)
	Expect(err).To(BeNil())
	return exp
}

func GetEventSubscription(eventSubscription api.EventSubscription) api.Subscription {
	var sub api.Subscription
	err := (&sub).FromEventSubscription(eventSubscription)
	Expect(err).To(BeNil())
	return sub
}

func GetFileExposure(fileExposure api.FileExposure) api.Exposure {
	var exp api.Exposure
	err := (&exp).FromFileExposure(fileExposure)
	Expect(err).To(BeNil())
	return exp
}

func GetFileSubscription(fileSubscription api.FileSubscription) api.Subscription {
	var sub api.Subscription
	err := (&sub).FromFileSubscription(fileSubscription)
	Expect(err).To(BeNil())
	return sub
}
