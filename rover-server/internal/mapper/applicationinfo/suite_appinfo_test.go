// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
