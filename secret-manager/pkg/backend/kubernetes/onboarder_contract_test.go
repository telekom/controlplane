// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/kubernetes"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/onboardertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// k8sHarness adapts the Kubernetes onboarder to the shared contract suite.
// The fake client is the shared store: onboarding writes to it and Get reads
// back through KubernetesBackend.
type k8sHarness struct {
	client    client.Client
	onboarder *kubernetes.KubernetesOnboarder
}

func newK8sHarness() onboardertest.Harness {
	c := NewMockK8sClient()
	return &k8sHarness{
		client:    c,
		onboarder: kubernetes.NewOnboarder(c),
	}
}

func (h *k8sHarness) Onboarder() backend.Onboarder {
	return h.onboarder
}

func (h *k8sHarness) Get(ctx context.Context, ref string) (string, error) {
	id, err := kubernetes.FromString(ref)
	if err != nil {
		return "", err
	}
	secret, err := kubernetes.NewBackend(h.client).Get(ctx, id)
	if err != nil {
		return "", err
	}
	return secret.Value(), nil
}

var _ = Describe("Kubernetes Onboarder Contract", func() {
	onboardertest.RunContractSpecs(newK8sHarness)
})
