// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func NewEventSpecificationStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*roverv1.EventSpecification] {
	mockStore := NewMockObjectStore[*roverv1.EventSpecification](testing)
	ConfigureEventSpecificationStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureEventSpecificationStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.EventSpecification]) {
	configureEventSpecification(testing, mockedStore)
	configureEventSpecificationNotFound(mockedStore)
}

func configureEventSpecification(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.EventSpecification]) {
	eventSpecification := GetEventSpecification(testing, EventSpecificationFileName)

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "tardis-horizon-demo-cetus-v1"
		}),
	).Return(eventSpecification, nil).Maybe()

	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).Return(
		&store.ListResponse[*roverv1.EventSpecification]{
			Items: []*roverv1.EventSpecification{eventSpecification}}, nil).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "tardis-horizon-demo-cetus-v1"
		}),
	).Return(nil).Maybe()

	mockedStore.EXPECT().CreateOrReplace(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("*v1.EventSpecification"),
	).Return(nil).Maybe()
}

func configureEventSpecificationNotFound(mockedStore *MockObjectStore[*roverv1.EventSpecification]) {
	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "tardis-horizon-demo-cetus-v1"
		}),
	).Return(nil, problems.NotFound("eventspec not found")).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "tardis-horizon-demo-cetus-v1"
		}),
	).Return(problems.NotFound("eventspec not found")).Maybe()
}
