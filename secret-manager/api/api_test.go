// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"
	"github.com/telekom/controlplane/secret-manager/api/gen"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("API Wrapper", func() {
	var (
		mockClient *fake.MockClientWithResponsesInterface
		sut        api.SecretManager
		ctx        context.Context
	)

	BeforeEach(func() {
		mockClient = &fake.MockClientWithResponsesInterface{}
		sut = api.NewSecretManagerFromClient(mockClient)
		ctx = context.Background()
	})

	Describe("UpsertEnvironment", func() {
		It("should pass strategy to the generated client", func() {
			var capturedBody gen.EnvironmentWriteRequest
			mockClient.EXPECT().
				UpsertEnvironmentWithResponse(mock.Anything, "env-1", mock.Anything).
				Run(func(_ context.Context, _ string, body gen.EnvironmentWriteRequest, _ ...gen.RequestEditorFn) {
					capturedBody = body
				}).
				Return(&gen.UpsertEnvironmentResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &gen.OnboardingResponse{Items: []gen.ListSecretItem{}},
				}, nil)

			_, err := sut.UpsertEnvironment(ctx, "env-1", api.WithStrategy(gen.Replace))
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedBody.Strategy).ToNot(BeNil())
			Expect(*capturedBody.Strategy).To(Equal(gen.Replace))
		})

		It("should not set strategy when not provided", func() {
			var capturedBody gen.EnvironmentWriteRequest
			mockClient.EXPECT().
				UpsertEnvironmentWithResponse(mock.Anything, "env-1", mock.Anything).
				Run(func(_ context.Context, _ string, body gen.EnvironmentWriteRequest, _ ...gen.RequestEditorFn) {
					capturedBody = body
				}).
				Return(&gen.UpsertEnvironmentResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &gen.OnboardingResponse{Items: []gen.ListSecretItem{}},
				}, nil)

			_, err := sut.UpsertEnvironment(ctx, "env-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedBody.Strategy).To(BeNil())
		})

		It("should pass both strategy and secrets together", func() {
			var capturedBody gen.EnvironmentWriteRequest
			mockClient.EXPECT().
				UpsertEnvironmentWithResponse(mock.Anything, "env-1", mock.Anything).
				Run(func(_ context.Context, _ string, body gen.EnvironmentWriteRequest, _ ...gen.RequestEditorFn) {
					capturedBody = body
				}).
				Return(&gen.UpsertEnvironmentResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &gen.OnboardingResponse{Items: []gen.ListSecretItem{}},
				}, nil)

			_, err := sut.UpsertEnvironment(ctx, "env-1",
				api.WithStrategy(gen.Merge),
				api.WithSecretValue("mySecret", "myValue"),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedBody.Strategy).ToNot(BeNil())
			Expect(*capturedBody.Strategy).To(Equal(gen.Merge))
			Expect(capturedBody.Secrets).To(HaveLen(1))
			Expect(capturedBody.Secrets[0].Name).To(Equal("mySecret"))
			Expect(capturedBody.Secrets[0].Value).To(Equal("myValue"))
		})
	})

	Describe("UpsertTeam", func() {
		It("should pass strategy to the generated client", func() {
			var capturedBody gen.TeamWriteRequest
			mockClient.EXPECT().
				UpsertTeamWithResponse(mock.Anything, "env-1", "team-1", mock.Anything).
				Run(func(_ context.Context, _, _ string, body gen.TeamWriteRequest, _ ...gen.RequestEditorFn) {
					capturedBody = body
				}).
				Return(&gen.UpsertTeamResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &gen.OnboardingResponse{Items: []gen.ListSecretItem{}},
				}, nil)

			_, err := sut.UpsertTeam(ctx, "env-1", "team-1", api.WithStrategy(gen.Replace))
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedBody.Strategy).ToNot(BeNil())
			Expect(*capturedBody.Strategy).To(Equal(gen.Replace))
		})

		It("should not set strategy when not provided", func() {
			var capturedBody gen.TeamWriteRequest
			mockClient.EXPECT().
				UpsertTeamWithResponse(mock.Anything, "env-1", "team-1", mock.Anything).
				Run(func(_ context.Context, _, _ string, body gen.TeamWriteRequest, _ ...gen.RequestEditorFn) {
					capturedBody = body
				}).
				Return(&gen.UpsertTeamResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &gen.OnboardingResponse{Items: []gen.ListSecretItem{}},
				}, nil)

			_, err := sut.UpsertTeam(ctx, "env-1", "team-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedBody.Strategy).To(BeNil())
		})
	})

	Describe("UpsertApplication", func() {
		It("should pass strategy to the generated client", func() {
			var capturedBody gen.ApplicationWriteRequest
			mockClient.EXPECT().
				UpsertAppWithResponse(mock.Anything, "env-1", "team-1", "app-1", mock.Anything).
				Run(func(_ context.Context, _, _, _ string, body gen.ApplicationWriteRequest, _ ...gen.RequestEditorFn) {
					capturedBody = body
				}).
				Return(&gen.UpsertAppResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &gen.OnboardingResponse{Items: []gen.ListSecretItem{}},
				}, nil)

			_, err := sut.UpsertApplication(ctx, "env-1", "team-1", "app-1", api.WithStrategy(gen.Replace))
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedBody.Strategy).ToNot(BeNil())
			Expect(*capturedBody.Strategy).To(Equal(gen.Replace))
		})

		It("should not set strategy when not provided", func() {
			var capturedBody gen.ApplicationWriteRequest
			mockClient.EXPECT().
				UpsertAppWithResponse(mock.Anything, "env-1", "team-1", "app-1", mock.Anything).
				Run(func(_ context.Context, _, _, _ string, body gen.ApplicationWriteRequest, _ ...gen.RequestEditorFn) {
					capturedBody = body
				}).
				Return(&gen.UpsertAppResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &gen.OnboardingResponse{Items: []gen.ListSecretItem{}},
				}, nil)

			_, err := sut.UpsertApplication(ctx, "env-1", "team-1", "app-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedBody.Strategy).To(BeNil())
		})
	})
})
