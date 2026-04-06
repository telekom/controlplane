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

func NewChangelogStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*roverv1.Changelog] {
	mockStore := NewMockObjectStore[*roverv1.Changelog](testing)
	ConfigureChangelogStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureChangelogStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.Changelog]) {
	configureChangelog(testing, mockedStore)
	configureChangelogNotFound(mockedStore)
}

func configureChangelog(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.Changelog]) {
	changelog := GetChangelog(testing, ChangelogFileName)

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "eni-test-api-v1"
		}),
	).Return(changelog, nil).Maybe()

	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(opts store.ListOpts) bool {
			return opts.Prefix != "" && strings.HasPrefix("poc--eni--hyperion/", opts.Prefix)
		}),
	).Return(
		&store.ListResponse[*roverv1.Changelog]{
			Items: []*roverv1.Changelog{changelog}}, nil).Maybe()

	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(opts store.ListOpts) bool {
			return opts.Prefix != "" && !strings.HasPrefix("poc--eni--hyperion/", opts.Prefix)
		}),
	).Return(
		&store.ListResponse[*roverv1.Changelog]{
			Items: []*roverv1.Changelog{}}, nil).Maybe()

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
		mock.AnythingOfType("*v1.Changelog"),
	).Return(nil).Maybe()
}

func configureChangelogNotFound(mockedStore *MockObjectStore[*roverv1.Changelog]) {
	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "eni-test-api-v1"
		}),
	).Return(nil, problems.NotFound("changelog not found")).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "eni-test-api-v1"
		}),
	).Return(problems.NotFound("changelog not found")).Maybe()
}
