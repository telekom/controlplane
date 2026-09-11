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

func NewApiChangelogStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*roverv1.ApiChangelog] {
	mockStore := NewMockObjectStore[*roverv1.ApiChangelog](testing)
	ConfigureApiChangelogStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureApiChangelogStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.ApiChangelog]) {
	configureApiChangelog(testing, mockedStore)
	configureApiChangelogNotFound(mockedStore)
}

func configureApiChangelog(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.ApiChangelog]) {
	changelog := GetApiChangelog(testing, ApiChangelogFileName) // eni-test-api

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "eni-test-api"
		}),
	).Return(changelog, nil).Maybe()

	// List with a prefix that matches our test data (eni/hyperion)
	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(opts store.ListOpts) bool {
			return opts.Prefix != "" && strings.HasPrefix("poc--eni--hyperion/", opts.Prefix)
		}),
	).Return(
		&store.ListResponse[*roverv1.ApiChangelog]{
			Items: []*roverv1.ApiChangelog{changelog}}, nil).Maybe()

	// List with a prefix that does NOT match our test data (e.g., different team)
	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(opts store.ListOpts) bool {
			return opts.Prefix != "" && !strings.HasPrefix("poc--eni--hyperion/", opts.Prefix)
		}),
	).Return(
		&store.ListResponse[*roverv1.ApiChangelog]{
			Items: []*roverv1.ApiChangelog{}}, nil).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "eni-test-api"
		}),
	).Return(nil).Maybe()

	mockedStore.EXPECT().CreateOrReplace(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("*v1.ApiChangelog"),
	).Return(nil).Maybe()
}

func configureApiChangelogNotFound(mockedStore *MockObjectStore[*roverv1.ApiChangelog]) {
	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "eni-test-api"
		}),
	).Return(nil, problems.NotFound("apichangelog not found")).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "eni-test-api"
		}),
	).Return(problems.NotFound("apichangelog not found")).Maybe()
}
