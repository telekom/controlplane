// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"errors"
	"net/http"

	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"
	mockclient "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CreateOrReplaceConsumer", func() {
	var (
		ctx      context.Context
		consumer *v1.Consumer
		api      *mockclient.MockKongAdminApi
		client   clientpkg.KongClient
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test")
		consumer = &v1.Consumer{Spec: v1.ConsumerSpec{Name: "test-consumer"}}
		api = mockclient.NewMockKongAdminApi(GinkgoT())
		client = clientpkg.NewKongClient(api)
	})

	matchingConsumer := func() *kong.GetConsumerResponse {
		return &kong.GetConsumerResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &kong.Consumer{
				Id: ptr("consumer-id"), Username: ptr("test-consumer"), CustomId: ptr("test-consumer"),
				Tags: &[]string{"consumer--test-consumer", "env--test"},
			},
		}
	}

	expectMembership := func() {
		api.EXPECT().ViewGroupConsumerWithResponse(mock.Anything, "test-consumer").Return(
			&kong.ViewGroupConsumerResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &kong.ConsumerGroupResponse{Data: &[]kong.ConsumerGroup{{}}},
			}, nil,
		)
	}

	It("does not upsert a matching consumer and restores its ID", func() {
		api.EXPECT().GetConsumerWithResponse(mock.Anything, "test-consumer").Return(matchingConsumer(), nil)
		expectMembership()

		result, err := client.CreateOrReplaceConsumer(ctx, consumer)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Id).To(Equal(ptr("consumer-id")))
		Expect(consumer.GetProperty("kongConsumerId")).To(Equal("consumer-id"))
	})

	It("does not upsert a consumer when tags are reordered", func() {
		response := matchingConsumer()
		response.JSON200.Tags = &[]string{"env--test", "consumer--test-consumer"}
		api.EXPECT().GetConsumerWithResponse(mock.Anything, "test-consumer").Return(response, nil)
		expectMembership()

		_, err := client.CreateOrReplaceConsumer(ctx, consumer)
		Expect(err).NotTo(HaveOccurred())
	})

	It("upserts a consumer whose custom ID changed", func() {
		response := matchingConsumer()
		response.JSON200.CustomId = ptr("old-id")
		api.EXPECT().GetConsumerWithResponse(mock.Anything, "test-consumer").Return(response, nil)
		api.EXPECT().UpsertConsumerWithResponse(mock.Anything, "test-consumer", mock.Anything).Return(
			&kong.UpsertConsumerResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				Body:         []byte(`{"id":"consumer-id"}`),
			}, nil,
		)
		expectMembership()

		_, err := client.CreateOrReplaceConsumer(ctx, consumer)
		Expect(err).NotTo(HaveOccurred())
	})

	It("upserts a missing consumer", func() {
		api.EXPECT().GetConsumerWithResponse(mock.Anything, "test-consumer").Return(
			&kong.GetConsumerResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil,
		)
		api.EXPECT().UpsertConsumerWithResponse(mock.Anything, "test-consumer", mock.Anything).Return(
			&kong.UpsertConsumerResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				Body:         []byte(`{"id":"consumer-id"}`),
			}, nil,
		)
		expectMembership()

		_, err := client.CreateOrReplaceConsumer(ctx, consumer)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns a consumer GET failure without writing", func() {
		api.EXPECT().GetConsumerWithResponse(mock.Anything, "test-consumer").Return(nil, errors.New("read failed"))

		_, err := client.CreateOrReplaceConsumer(ctx, consumer)
		Expect(err).To(MatchError(ContainSubstring("failed to get consumer")))
	})

	It("rejects a successful consumer read without a body", func() {
		api.EXPECT().GetConsumerWithResponse(mock.Anything, "test-consumer").Return(
			&kong.GetConsumerResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}, nil,
		)

		_, err := client.CreateOrReplaceConsumer(ctx, consumer)
		Expect(err).To(MatchError(ContainSubstring("consumer response body is missing")))
	})

	It("fails without writing when the group membership read fails", func() {
		writes := reconcileCount("consumer", "written")
		api.EXPECT().GetConsumerWithResponse(mock.Anything, "test-consumer").Return(matchingConsumer(), nil)
		api.EXPECT().ViewGroupConsumerWithResponse(mock.Anything, "test-consumer").Return(nil, errors.New("membership failed"))

		_, err := client.CreateOrReplaceConsumer(ctx, consumer)
		Expect(err).To(MatchError(ContainSubstring("error occurred when getting consumer group")))
		after := reconcileCount("consumer", "written")
		Expect(after).To(Equal(writes))
	})

	DescribeTable("reports a malformed group membership response instead of panicking",
		func(response *kong.ViewGroupConsumerResponse, message string) {
			api.EXPECT().GetConsumerWithResponse(mock.Anything, "test-consumer").Return(matchingConsumer(), nil)
			api.EXPECT().ViewGroupConsumerWithResponse(mock.Anything, "test-consumer").Return(response, nil)

			_, err := client.CreateOrReplaceConsumer(ctx, consumer)
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("without a body",
			&kong.ViewGroupConsumerResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}},
			"consumer group response body is missing"),
		Entry("without data",
			&kong.ViewGroupConsumerResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &kong.ConsumerGroupResponse{},
			},
			"consumer group response data is missing"),
	)

	It("adds a consumer that is not yet in its group", func() {
		api.EXPECT().GetConsumerWithResponse(mock.Anything, "test-consumer").Return(matchingConsumer(), nil)
		api.EXPECT().ViewGroupConsumerWithResponse(mock.Anything, "test-consumer").Return(
			&kong.ViewGroupConsumerResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &kong.ConsumerGroupResponse{Data: &[]kong.ConsumerGroup{}},
			}, nil,
		)
		api.EXPECT().AddConsumerToGroupWithResponse(mock.Anything, "test-consumer", mock.Anything).Return(
			&kong.AddConsumerToGroupResponse{HTTPResponse: &http.Response{StatusCode: http.StatusCreated}}, nil,
		).Once()

		_, err := client.CreateOrReplaceConsumer(ctx, consumer)
		Expect(err).NotTo(HaveOccurred())
	})

	It("reports a rejected group membership write", func() {
		api.EXPECT().GetConsumerWithResponse(mock.Anything, "test-consumer").Return(matchingConsumer(), nil)
		api.EXPECT().ViewGroupConsumerWithResponse(mock.Anything, "test-consumer").Return(
			&kong.ViewGroupConsumerResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &kong.ConsumerGroupResponse{Data: &[]kong.ConsumerGroup{}},
			}, nil,
		)
		api.EXPECT().AddConsumerToGroupWithResponse(mock.Anything, "test-consumer", mock.Anything).Return(
			&kong.AddConsumerToGroupResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusBadRequest},
				Body:         []byte(`{"name":"schema violation","message":"group is required"}`),
			}, nil,
		)

		_, err := client.CreateOrReplaceConsumer(ctx, consumer)
		Expect(err).To(MatchError(ContainSubstring("failed to add consumer to group")))
	})
})
