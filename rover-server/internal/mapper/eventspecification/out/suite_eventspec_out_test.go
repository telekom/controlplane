// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"

	storeLib "github.com/telekom/controlplane/common-server/pkg/store"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type ContextKey string

var (
	ctx                context.Context
	eventSpecification *roverv1.EventSpecification
	stores             *store.Stores
)

func TestMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EventSpecification Out Mapper Suite")
}

var _ = BeforeSuite(func() {
	ctx = context.WithValue(context.TODO(), ContextKey("test"), "test")

	stores = &store.Stores{}

	eventTypeMock := mocks.NewMockObjectStore[*eventv1.EventType](GinkgoT())
	eventTypeMock.EXPECT().List(mock.Anything, mock.Anything).Return(
		&storeLib.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{}}, nil).Maybe()
	stores.EventTypeStore = eventTypeMock

	eventSpecification = mocks.GetEventSpecification(GinkgoT(), mocks.EventSpecificationFileName)
})
