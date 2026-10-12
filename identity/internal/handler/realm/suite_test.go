// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"path/filepath"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	testEnv   *envtest.Environment
	k8sClient client.Client
)

func TestRealmHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Realm Handler Suite")
}

var _ = BeforeSuite(func() {
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(identityv1.AddToScheme(scheme.Scheme)).To(Succeed())
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	_ = context.Background()
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})
