// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func NewRoadmapStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*roverv1.Roadmap] {
	mockStore := NewMockObjectStore[*roverv1.Roadmap](testing)
	ConfigureRoadmapStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureRoadmapStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.Roadmap]) {
	configureRoadmap(testing, mockedStore)
	configureRoadmapNotFound(mockedStore)
}

func configureRoadmap(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.Roadmap]) {
	roadmap := GetRoadmap(testing, RoadmapFileName) // eni-test-api-v1

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "eni-test-api-v1"
		}),
	).Return(roadmap, nil).Maybe()

	// List with a prefix that matches our test data (eni/hyperion)
	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(opts store.ListOpts) bool {
			return opts.Prefix != "" && strings.HasPrefix("poc--eni--hyperion/", opts.Prefix)
		}),
	).Return(
		&store.ListResponse[*roverv1.Roadmap]{
			Items: []*roverv1.Roadmap{roadmap}}, nil).Maybe()

	// List with a prefix that does NOT match our test data (e.g., different team)
	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(opts store.ListOpts) bool {
			return opts.Prefix != "" && !strings.HasPrefix("poc--eni--hyperion/", opts.Prefix)
		}),
	).Return(
		&store.ListResponse[*roverv1.Roadmap]{
			Items: []*roverv1.Roadmap{}}, nil).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "eni-test-api-v1"
		}),
	).Return(nil).Maybe()

	mockedStore.EXPECT().CreateOrReplace(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("*v1.Roadmap"),
	).Return(nil).Maybe()
}

func configureRoadmapNotFound(mockedStore *MockObjectStore[*roverv1.Roadmap]) {
	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "eni-test-api-v1"
		}),
	).Return(nil, problems.NotFound("roadmap not found")).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "eni-test-api-v1"
		}),
	).Return(problems.NotFound("roadmap not found")).Maybe()
}
