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

func NewRoverStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*roverv1.Rover] {
	mockStore := NewMockObjectStore[*roverv1.Rover](testing)
	ConfigureRoverStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureRoverStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.Rover]) {
	configureRover(testing, mockedStore)
	configureRoverNotFound(mockedStore)
}

func configureRover(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.Rover]) {
	rover := GetRover(testing, RoverFileName)

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s == "rover-local-sub"
		}),
	).Return(rover, nil).Maybe()

	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).Return(
		&store.ListResponse[*roverv1.Rover]{
			Items: []*roverv1.Rover{rover}}, nil).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "rover-local-sub"
		}),
	).Return(nil).Maybe()

	mockedStore.EXPECT().CreateOrReplace(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("*v1.Rover"),
	).Return(nil).Maybe()
}

func configureRoverNotFound(mockedStore *MockObjectStore[*roverv1.Rover]) {
	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "rover-local-sub"
		}),
	).Return(nil, problems.NotFound("rover not found")).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s != "rover-local-sub"
		}),
	).Return(problems.NotFound("rover not found")).Maybe()
}
