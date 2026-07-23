// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package publication

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeClient struct {
	requests []*xdsapi.PublishBundleRequest
	errors   []error
}

func (f *fakeClient) PublishBundle(
	_ context.Context,
	request *xdsapi.PublishBundleRequest,
	_ ...grpc.CallOption,
) (*xdsapi.PublishBundleResponse, error) {
	f.requests = append(f.requests, request)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		return nil, err
	}
	return &xdsapi.PublishBundleResponse{Activated: true}, nil
}

var _ = Describe("Publisher", func() {
	It("retries transient failures with the identical immutable envelope", func() {
		client := &fakeClient{errors: []error{status.Error(codes.Unavailable, "temporary")}}
		publisher := &Publisher{client: client, timeout: time.Second, attempts: 2, retryDelay: time.Millisecond}
		bundle := &xdsapi.Bundle{TargetId: "target-a"}

		response, err := publisher.Publish(context.Background(), bundle)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Activated).To(BeTrue())
		Expect(client.requests).To(HaveLen(2))
		Expect(client.requests[0].Bundle).To(BeIdenticalTo(client.requests[1].Bundle))
	})

	It("does not retry permanent validation failures", func() {
		client := &fakeClient{errors: []error{status.Error(codes.InvalidArgument, "invalid")}}
		publisher := &Publisher{client: client, timeout: time.Second, attempts: 3, retryDelay: time.Millisecond}

		_, err := publisher.Publish(context.Background(), &xdsapi.Bundle{})
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		Expect(client.requests).To(HaveLen(1))
	})

	It("includes sanitized validation details in permanent errors", func() {
		grpcStatus, err := status.New(codes.InvalidArgument, "bundle validation failed").WithDetails(
			&xdsapi.ValidationError{Field: "routes[0]", Message: "resource violates Envoy validation constraints"},
		)
		Expect(err).NotTo(HaveOccurred())
		client := &fakeClient{errors: []error{grpcStatus.Err()}}
		publisher := &Publisher{client: client, timeout: time.Second, attempts: 1}

		_, err = publisher.Publish(context.Background(), &xdsapi.Bundle{})
		Expect(err).To(MatchError(ContainSubstring(
			"routes[0]: resource violates Envoy validation constraints")))
	})
})
