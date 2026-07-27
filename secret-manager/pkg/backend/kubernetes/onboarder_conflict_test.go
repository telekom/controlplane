// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/kubernetes"
)

// alwaysConflictClient wraps a real fake client but forces every Create and
// Update to fail with a 409 Conflict, so the onboarder's bounded optimistic
// retry is always exhausted. This deterministically exercises the "retries ran
// out under contention" path without needing real concurrency.
func alwaysConflictClient() client.Client {
	conflict := func(name string) error {
		return apierrors.NewConflict(
			schema.GroupResource{Resource: "secrets"}, name,
			fmt.Errorf("forced conflict"))
	}
	return interceptor.NewClient(fake.NewClientBuilder().Build(), interceptor.Funcs{
		Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
			return conflict(obj.GetName())
		},
		Update: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.UpdateOption) error {
			return conflict(obj.GetName())
		},
	})
}

var _ = Describe("Kubernetes Onboarder conflict classification", func() {
	// A write conflict that survives all retries is transient contention, not a
	// server fault. It must surface as TooManyRequests (HTTP 429) so callers
	// retry, matching the Conjur backend, instead of a generic 500.
	It("maps an exhausted write conflict to TooManyRequests", func() {
		ob := kubernetes.NewOnboarder(alwaysConflictClient())

		_, err := ob.OnboardEnvironment(context.Background(), "test-env")
		Expect(err).To(HaveOccurred())

		var backendErr *backend.BackendError
		Expect(errors.As(err, &backendErr)).To(BeTrue(), "expected a BackendError")
		Expect(backendErr.Type).To(Equal(backend.TypeErrTooManyRequests))
		Expect(backendErr.Code()).To(Equal(429))
	})
})
