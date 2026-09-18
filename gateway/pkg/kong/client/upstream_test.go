// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"errors"
	"net/http"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"
	mockclient "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CreateOrReplaceUpstream", func() {
	var (
		ctx        context.Context
		route      *v1.Route
		api        *mockclient.MockKongAdminApi
		client     clientpkg.KongClient
		upstream   kong.CreateUpstreamJSONRequestBody
		target     kong.CreateTargetForUpstreamJSONRequestBody
		upstreamID string
		targetID   string
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test")
		route = &v1.Route{ObjectMeta: metav1.ObjectMeta{Name: "test-route"}}
		api = mockclient.NewMockKongAdminApi(GinkgoT())
		client = clientpkg.NewKongClient(api)
		algorithm := kong.CreateUpstreamRequestAlgorithm("round-robin")
		activeType := kong.CreateUpstreamRequestHealthchecksActiveType("http")
		upstream = kong.CreateUpstreamJSONRequestBody{
			Name: "test-route", Algorithm: &algorithm,
			Healthchecks: &kong.CreateUpstreamRequestHealthchecks{Active: &kong.CreateUpstreamRequestHealthchecksActive{
				Type: &activeType, Healthy: &kong.CreateUpstreamRequestHealthchecksActiveHealthy{HttpStatuses: ptr([]int{200, 201})},
			}},
			Tags: ptr([]string{"route--test-route", "env--test"}),
		}
		target = kong.CreateTargetForUpstreamJSONRequestBody{
			Target: ptr("localhost:8080"), Weight: ptr(100), Tags: ptr([]string{"targets--test-route", "env--test"}),
		}
		upstreamID = "upstream-id"
		targetID = "target-id"
	})

	matchingUpstream := func() *kong.GetUpstreamResponse {
		return &kong.GetUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Upstream{
			Id: &upstreamID, Name: ptr("test-route"), Algorithm: ptr("round-robin"),
			Healthchecks: &kong.UpstreamHealthchecks{Active: &kong.UpstreamHealthchecksActive{
				Type: ptr("http"), Healthy: &kong.UpstreamHealthchecksActiveHealthy{HttpStatuses: ptr([]int{200, 201})},
			}},
			Tags: ptr([]string{"route--test-route", "env--test"}),
		}}
	}

	targetsResponse := func(targets []kong.Target, offset *string) *kong.ListTargetsForUpstreamResponse {
		return &kong.ListTargetsForUpstreamResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &kong.ListTargetsForUpstream200Response{Data: &targets, Offset: offset},
		}
	}

	matchingTarget := func(createdAt float32) kong.Target {
		return kong.Target{
			Id: &targetID, CreatedAt: &createdAt, Target: ptr("localhost:8080"), Weight: ptr(100),
			Tags: ptr([]string{"targets--test-route", "env--test"}),
		}
	}

	expectMatchingUpstream := func(response *kong.GetUpstreamResponse) {
		api.EXPECT().GetUpstreamWithResponse(mock.Anything, "test-route").Return(response, nil)
	}

	It("suppresses writes for matching upstream and target and restores IDs", func() {
		response := matchingUpstream()
		response.JSON200.Tags = ptr([]string{"env--test", "route--test-route"})
		response.JSON200.Healthchecks.Active.Healthy.HttpStatuses = ptr([]int{201, 200})
		expectMatchingUpstream(response)
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.MatchedBy(func(params *kong.ListTargetsForUpstreamParams) bool {
			return params.Size != nil && *params.Size == 1000 && params.Tags != nil && *params.Tags == "env--test,targets--test-route"
		})).Return(targetsResponse([]kong.Target{matchingTarget(1)}, nil), nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
		Expect(route.GetUpstreamId()).To(Equal("upstream-id"))
		Expect(route.GetTargetsId()).To(Equal("target-id"))
	})

	DescribeTable("rejects missing request bodies",
		func(upstream *kong.CreateUpstreamJSONRequestBody, target *kong.CreateTargetForUpstreamJSONRequestBody, message string) {
			Expect(client.CreateOrReplaceUpstream(ctx, route, upstream, target)).To(MatchError(message))
		},
		Entry("upstream", nil, &kong.CreateTargetForUpstreamJSONRequestBody{}, "upstream request body is missing"),
		Entry("target", &kong.CreateUpstreamJSONRequestBody{}, nil, "target request body is missing"),
	)

	It("omits the target tag filter when no tags are requested", func() {
		target.Tags = nil
		expectMatchingUpstream(matchingUpstream())
		current := matchingTarget(1)
		current.Tags = nil
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.MatchedBy(func(params *kong.ListTargetsForUpstreamParams) bool {
			return params.Tags == nil
		})).Return(targetsResponse([]kong.Target{current}, nil), nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
	})

	It("suppresses the write when Kong fills in health check defaults the controller does not set", func() {
		passiveType := kong.CreateUpstreamRequestHealthchecksPassiveType("http")
		upstream.Healthchecks.Passive = &kong.CreateUpstreamRequestHealthchecksPassive{
			Type: &passiveType,
			Healthy: &kong.CreateUpstreamRequestHealthchecksPassiveHealthy{
				HttpStatuses: ptr([]kong.CreateUpstreamRequestHealthchecksPassiveHealthyHttpStatuses{200}),
				Successes:    ptr(1),
			},
			Unhealthy: &kong.CreateUpstreamRequestHealthchecksPassiveUnhealthy{
				HttpFailures: ptr(5), TcpFailures: ptr(5), Timeouts: ptr(5),
				HttpStatuses: ptr([]kong.CreateUpstreamRequestHealthchecksPassiveUnhealthyHttpStatuses{500}),
			},
		}

		response := matchingUpstream()
		// Kong answers with a fully populated health check object. Only the
		// fields the circuit breaker sets may take part in the comparison.
		response.JSON200.Healthchecks.Threshold = ptr(float32(0))
		response.JSON200.Healthchecks.Active.Concurrency = ptr(10)
		response.JSON200.Healthchecks.Active.HttpPath = ptr("/")
		response.JSON200.Healthchecks.Active.HttpsVerifyCertificate = ptr(true)
		response.JSON200.Healthchecks.Active.Timeout = ptr(float32(1))
		response.JSON200.Healthchecks.Active.Healthy.Interval = ptr(float32(0))
		response.JSON200.Healthchecks.Active.Healthy.Successes = ptr(0)
		response.JSON200.Healthchecks.Active.Unhealthy = &kong.UpstreamHealthchecksActiveUnhealthy{
			Interval: ptr(float32(0)), HttpFailures: ptr(0), TcpFailures: ptr(0), Timeouts: ptr(0),
		}
		response.JSON200.Healthchecks.Passive = &kong.UpstreamHealthchecksPassive{
			Type:    ptr("http"),
			Healthy: &kong.UpstreamHealthchecksPassiveHealthy{HttpStatuses: ptr([]int{200}), Successes: ptr(1)},
			Unhealthy: &kong.UpstreamHealthchecksPassiveUnhealthy{
				HttpFailures: ptr(5), TcpFailures: ptr(5), Timeouts: ptr(5), HttpStatuses: ptr([]int{500}),
			},
		}
		expectMatchingUpstream(response)
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			targetsResponse([]kong.Target{matchingTarget(1)}, nil), nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
		Expect(route.GetUpstreamId()).To(Equal("upstream-id"))
	})

	It("upserts a changed upstream health check", func() {
		response := matchingUpstream()
		response.JSON200.Healthchecks.Active.Healthy.HttpStatuses = ptr([]int{200})
		expectMatchingUpstream(response)
		api.EXPECT().UpsertUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.UpsertUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Upstream{Id: &upstreamID}}, nil)
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			targetsResponse([]kong.Target{matchingTarget(1)}, nil), nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
	})

	It("upserts a missing upstream", func() {
		expectMatchingUpstream(&kong.GetUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}})
		api.EXPECT().UpsertUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.UpsertUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Upstream{Id: &upstreamID}}, nil)
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			targetsResponse([]kong.Target{matchingTarget(1)}, nil), nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
	})

	It("rejects a successful upstream response without a body", func() {
		expectMatchingUpstream(&kong.GetUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}})
		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(MatchError(ContainSubstring("upstream response body is missing")))
	})

	It("rejects a successful upstream response without an ID", func() {
		response := matchingUpstream()
		response.JSON200.Id = nil
		expectMatchingUpstream(response)
		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(MatchError(ContainSubstring("upstream response ID is missing")))
	})

	It("uses the latest matching target and creates one when its weight differs", func() {
		expectMatchingUpstream(matchingUpstream())
		old := matchingTarget(1)
		latest := matchingTarget(2)
		latest.Weight = ptr(50)
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			targetsResponse([]kong.Target{latest, old}, nil), nil)
		api.EXPECT().CreateTargetForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.CreateTargetForUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusCreated}, JSON200: &kong.Target{Id: &targetID}}, nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
	})

	It("creates a target when no target address matches", func() {
		expectMatchingUpstream(matchingUpstream())
		other := matchingTarget(1)
		other.Target = ptr("other:8080")
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(targetsResponse([]kong.Target{other}, nil), nil)
		api.EXPECT().CreateTargetForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.CreateTargetForUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusCreated}, JSON200: &kong.Target{Id: &targetID}}, nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
	})

	DescribeTable("picks the same target whatever order Kong lists equal timestamps in",
		func(reversed bool) {
			// Without a tie-break the choice would follow the list order, and a
			// Kong that reorders its answer would make the reconciliation write
			// a target on every pass.
			expectMatchingUpstream(matchingUpstream())
			lowerID := matchingTarget(1)
			lowerID.Id = ptr("target-a")
			lowerID.Weight = ptr(50)
			higherID := matchingTarget(1)
			higherID.Id = ptr("target-b")
			listed := []kong.Target{lowerID, higherID}
			if reversed {
				listed = []kong.Target{higherID, lowerID}
			}
			api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
				targetsResponse(listed, nil), nil)

			Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
			Expect(route.GetTargetsId()).To(Equal("target-b"))
		},
		Entry("lower id first", false),
		Entry("higher id first", true),
	)

	It("prefers the later list entry when target timestamps are absent", func() {
		expectMatchingUpstream(matchingUpstream())
		first := matchingTarget(1)
		first.CreatedAt = nil
		first.Weight = ptr(50)
		second := matchingTarget(1)
		second.CreatedAt = nil
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			targetsResponse([]kong.Target{first, second}, nil), nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
		Expect(route.GetTargetsId()).To(Equal("target-id"))
	})

	It("prefers a target with a timestamp over a later target without one", func() {
		expectMatchingUpstream(matchingUpstream())
		withTimestamp := matchingTarget(1)
		withoutTimestamp := matchingTarget(1)
		withoutTimestamp.CreatedAt = nil
		withoutTimestamp.Weight = ptr(50)
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			targetsResponse([]kong.Target{withTimestamp, withoutTimestamp}, nil), nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
		Expect(route.GetTargetsId()).To(Equal("target-id"))
	})

	It("follows target pagination and restores the matching target ID", func() {
		expectMatchingUpstream(matchingUpstream())
		next := "next-page"
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.MatchedBy(func(params *kong.ListTargetsForUpstreamParams) bool {
			return params.Offset == nil
		})).Return(targetsResponse(nil, &next), nil)
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.MatchedBy(func(params *kong.ListTargetsForUpstreamParams) bool {
			return params.Offset != nil && *params.Offset == "next-page"
		})).Return(targetsResponse([]kong.Target{matchingTarget(2)}, nil), nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
		Expect(route.GetTargetsId()).To(Equal("target-id"))
	})

	It("rejects a target page that repeats its offset", func() {
		expectMatchingUpstream(matchingUpstream())
		next := "next-page"
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.MatchedBy(func(params *kong.ListTargetsForUpstreamParams) bool {
			return params.Offset == nil
		})).Return(targetsResponse(nil, &next), nil)
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.MatchedBy(func(params *kong.ListTargetsForUpstreamParams) bool {
			return params.Offset != nil && *params.Offset == next
		})).Return(targetsResponse(nil, &next), nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(MatchError("target list pagination offset did not advance"))
	})

	It("rejects a successful target list without a body", func() {
		before := reconcileCount("target", "error")
		expectMatchingUpstream(matchingUpstream())
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.ListTargetsForUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}, nil)
		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(MatchError(ContainSubstring("target list response body is missing")))
		after := reconcileCount("target", "error")
		Expect(after).To(Equal(before + 1))
	})

	It("rejects a successful target list without data", func() {
		expectMatchingUpstream(matchingUpstream())
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.ListTargetsForUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.ListTargetsForUpstream200Response{}}, nil)
		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(MatchError(ContainSubstring("target list response data is missing")))
	})

	It("rejects a successful target write without an ID", func() {
		expectMatchingUpstream(matchingUpstream())
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(targetsResponse(nil, nil), nil)
		api.EXPECT().CreateTargetForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.CreateTargetForUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusCreated}, JSON200: &kong.Target{}}, nil)
		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(MatchError(ContainSubstring("target response ID is missing")))
	})

	DescribeTable("does not suppress a changed upstream field",
		func(change func(*kong.Upstream)) {
			response := matchingUpstream()
			change(response.JSON200)
			expectMatchingUpstream(response)
			api.EXPECT().UpsertUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
				&kong.UpsertUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Upstream{Id: &upstreamID}}, nil).Once()
			api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
				targetsResponse([]kong.Target{matchingTarget(1)}, nil), nil)

			Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
		},
		Entry("algorithm", func(current *kong.Upstream) { current.Algorithm = ptr("least-connections") }),
		Entry("tags", func(current *kong.Upstream) { current.Tags = ptr([]string{"route--test-route"}) }),
	)

	It("creates a target whose tags differ", func() {
		expectMatchingUpstream(matchingUpstream())
		retagged := matchingTarget(1)
		retagged.Tags = ptr([]string{"env--test"})
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			targetsResponse([]kong.Target{retagged}, nil), nil)
		api.EXPECT().CreateTargetForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.CreateTargetForUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusCreated}, JSON200: &kong.Target{Id: &targetID}}, nil).Once()

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(Succeed())
	})

	DescribeTable("reports a failed upstream write",
		func(response *kong.UpsertUpstreamResponse, writeErr error, message string) {
			current := matchingUpstream()
			current.JSON200.Algorithm = ptr("least-connections")
			expectMatchingUpstream(current)
			api.EXPECT().UpsertUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(response, writeErr)

			Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(MatchError(ContainSubstring(message)))
		},
		Entry("transport error", nil, errors.New("connection refused"), "failed to write upstream"),
		Entry("rejected by Kong",
			&kong.UpsertUpstreamResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusBadRequest},
				Body:         []byte(`{"name":"schema violation","message":"algorithm is invalid"}`),
			}, nil, "failed to write upstream"),
		Entry("accepted without a body",
			&kong.UpsertUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}, nil,
			"upstream response body is missing"),
	)

	DescribeTable("reports a failed target write",
		func(response *kong.CreateTargetForUpstreamResponse, writeErr error, message string) {
			expectMatchingUpstream(matchingUpstream())
			api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
				targetsResponse(nil, nil), nil)
			api.EXPECT().CreateTargetForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(response, writeErr)

			Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(MatchError(ContainSubstring(message)))
		},
		Entry("transport error", nil, errors.New("connection refused"), "failed to write target"),
		Entry("rejected by Kong",
			&kong.CreateTargetForUpstreamResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusConflict},
				Body:         []byte(`{"name":"unique violation","message":"target already exists"}`),
			}, nil, "failed to write target"),
		Entry("accepted without a body",
			&kong.CreateTargetForUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusCreated}}, nil,
			"target response body is missing"),
	)

	It("reports a failed target list", func() {
		expectMatchingUpstream(matchingUpstream())
		api.EXPECT().ListTargetsForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.ListTargetsForUpstreamResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError},
				Body:         []byte(`{"message":"failure to get a peer from the ring-balancer"}`),
			}, nil)

		Expect(client.CreateOrReplaceUpstream(ctx, route, &upstream, &target)).To(
			MatchError(ContainSubstring("failed to list upstream targets")))
	})
})
