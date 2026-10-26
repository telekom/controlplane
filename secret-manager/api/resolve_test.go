// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"context"
	"errors"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"
	"github.com/telekom/controlplane/secret-manager/api/gen"
)

var _ = Describe("Resolve and Get", func() {
	isRetryable := func(err error) bool {
		var httpErr *client.HttpError
		Expect(errors.As(err, &httpErr)).To(BeTrue())
		return httpErr.IsRetryable()
	}

	const (
		requestID   = "poc:team:app:externalSecrets/foo:"
		canonicalID = "poc:team:app:externalSecrets/foo:abc123"
		secretValue = "synthetic-value"
	)

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

	expectGet := func(status int, body *gen.SecretResponse, raw string) {
		mockClient.EXPECT().
			GetSecretWithResponse(mock.Anything, requestID).
			Return(&gen.GetSecretResponse{
				HTTPResponse: &http.Response{StatusCode: status},
				JSON200:      body,
				Body:         []byte(raw),
			}, nil).Once()
	}

	DescribeTable("should return the value and the canonical ref from the response",
		func(input string) {
			expectGet(http.StatusOK, &gen.SecretResponse{Id: canonicalID, Value: secretValue}, "")

			resolved, err := sut.Resolve(ctx, input)
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Value).To(Equal(secretValue))
			Expect(resolved.Ref).To(Equal("$<" + canonicalID + ">"))
			mockClient.AssertNumberOfCalls(GinkgoT(), "GetSecretWithResponse", 1)
		},
		Entry("raw id", requestID),
		Entry("wrapped ref", "$<"+requestID+">"),
	)

	It("should keep Get returning only the value", func() {
		expectGet(http.StatusOK, &gen.SecretResponse{Id: canonicalID, Value: secretValue}, "")

		value, err := sut.Get(ctx, "$<"+requestID+">")
		Expect(err).ToNot(HaveOccurred())
		Expect(value).To(Equal(secretValue))
	})

	It("should return ErrNotFound on 404", func() {
		expectGet(http.StatusNotFound, nil, "")

		_, err := sut.Resolve(ctx, requestID)
		Expect(err).To(Equal(api.ErrNotFound))

		expectGet(http.StatusNotFound, nil, "")
		_, err = sut.Get(ctx, requestID)
		Expect(err).To(Equal(api.ErrNotFound))
	})

	It("should return a retryable error on network failure", func() {
		mockClient.EXPECT().
			GetSecretWithResponse(mock.Anything, requestID).
			Return(nil, errors.New("connection refused")).Times(2)

		resolved, err := sut.Resolve(ctx, requestID)
		Expect(err).To(HaveOccurred())
		Expect(resolved).To(Equal(api.ResolvedSecret{}))
		Expect(isRetryable(err)).To(BeTrue())

		_, err = sut.Get(ctx, requestID)
		Expect(isRetryable(err)).To(BeTrue())
	})

	DescribeTable("should classify HTTP errors",
		func(status int, retryable bool) {
			expectGet(status, nil, "failure")
			_, err := sut.Resolve(ctx, requestID)
			Expect(err).To(HaveOccurred())
			Expect(isRetryable(err)).To(Equal(retryable))

			expectGet(status, nil, "failure")
			_, err = sut.Get(ctx, requestID)
			Expect(err).To(HaveOccurred())
			Expect(isRetryable(err)).To(Equal(retryable))
		},
		Entry("401 is blocked", http.StatusUnauthorized, false),
		Entry("400 is blocked", http.StatusBadRequest, false),
		Entry("500 is retryable", http.StatusInternalServerError, true),
	)
})
