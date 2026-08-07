// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFileHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "File Handler Suite")
}

const (
	testEnvironment = "test"
	testZone        = "cetus"
)

var _ = Describe("File Exposure/Subscription Handlers", func() {
	var (
		ctx        context.Context
		fakeClient ctrlclient.Client
	)

	newOwner := func() *roverv1.Rover {
		return &roverv1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "default"},
			Spec:       roverv1.RoverSpec{Zone: testZone},
		}
	}

	newJanitor := func() commonclient.JanitorClient {
		scoped := commonclient.NewScopedClient(fakeClient, testEnvironment)
		return commonclient.NewJanitorClient(scoped)
	}

	BeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(roverv1.AddToScheme(scheme)).To(Succeed())
		Expect(filev1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		// Env is required by the exposure handler (zone namespace resolution).
		ctx = contextutil.WithEnv(logr.NewContext(context.Background(), logr.Discard()), testEnvironment)
	})

	Context("HandleExposure", func() {
		It("should create a file-domain FileExposure owned by the Rover", func() {
			owner := newOwner()
			exp := &roverv1.FileExposure{
				FileType:   "demo-sftp-spec-v1",
				Visibility: roverv1.VisibilityWorld,
				Approval:   roverv1.Approval{Strategy: roverv1.ApprovalStrategyAuto},
				PublicKeys: []roverv1.PublicKey{{Label: "provider-key", Key: "ssh-ed25519 AAAAprovider"}},
			}

			Expect(HandleExposure(ctx, newJanitor(), owner, exp)).To(Succeed())

			name := MakeName(exp.FileType, owner.Name)
			fileExposure := &filev1.FileExposure{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, fileExposure)).To(Succeed())

			Expect(fileExposure.Spec.FileType).To(Equal("demo-sftp-spec-v1"))
			Expect(fileExposure.Spec.Visibility).To(Equal(filev1.Visibility("World")))
			Expect(fileExposure.Spec.Approval.Strategy).To(Equal(filev1.ApprovalStrategy("Auto")))
			Expect(fileExposure.Spec.SFTP).NotTo(BeNil())
			Expect(fileExposure.Spec.SFTP.PublicKeys).To(HaveLen(1))
			Expect(fileExposure.Spec.SFTP.PublicKeys[0].Key).To(Equal("ssh-ed25519 AAAAprovider"))
			Expect(fileExposure.Spec.Zone).NotTo(BeNil())
			Expect(fileExposure.Spec.Zone.Name).To(Equal(testZone))
			Expect(fileExposure.Spec.Zone.Namespace).To(Equal(testEnvironment))

			Expect(fileExposure.Labels).To(HaveKeyWithValue(filev1.FileTypeNameLabelKey, "demo-sftp-spec-v1"))
			Expect(fileExposure.Labels).To(HaveKeyWithValue(config.BuildLabelKey("application"), "my-app"))
			Expect(fileExposure.Labels).To(HaveKeyWithValue(config.EnvironmentLabelKey, testEnvironment))
			Expect(fileExposure.OwnerReferences).To(HaveLen(1))
			Expect(fileExposure.OwnerReferences[0].Name).To(Equal("my-app"))

			Expect(owner.Status.FileExposures).To(HaveLen(1))
			Expect(owner.Status.FileExposures[0].Name).To(Equal(name))
		})
	})

	Context("HandleSubscription", func() {
		It("should create a file-domain FileSubscription owned by the Rover", func() {
			owner := newOwner()
			sub := &roverv1.FileSubscription{
				FileType:   "demo-sftp-spec-v1",
				PublicKeys: []roverv1.PublicKey{{Label: "consumer-key", Key: "ssh-ed25519 AAAAconsumer"}},
			}

			Expect(HandleSubscription(ctx, newJanitor(), owner, sub)).To(Succeed())

			name := MakeName(sub.FileType, owner.Name)
			fileSubscription := &filev1.FileSubscription{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, fileSubscription)).To(Succeed())

			Expect(fileSubscription.Spec.FileType).To(Equal("demo-sftp-spec-v1"))
			Expect(fileSubscription.Spec.Zone).NotTo(BeNil())
			Expect(fileSubscription.Spec.Zone.Name).To(Equal(testZone))
			Expect(fileSubscription.Spec.Zone.Namespace).To(Equal(testEnvironment))
			Expect(fileSubscription.Spec.SFTP).NotTo(BeNil())
			Expect(fileSubscription.Spec.SFTP.PublicKeys).To(HaveLen(1))
			Expect(fileSubscription.Spec.SFTP.PublicKeys[0].Key).To(Equal("ssh-ed25519 AAAAconsumer"))

			Expect(fileSubscription.Labels).To(HaveKeyWithValue(filev1.FileTypeNameLabelKey, "demo-sftp-spec-v1"))
			Expect(fileSubscription.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), testZone))
			Expect(fileSubscription.OwnerReferences).To(HaveLen(1))
			Expect(fileSubscription.OwnerReferences[0].Name).To(Equal("my-app"))

			Expect(owner.Status.FileSubscriptions).To(HaveLen(1))
			Expect(owner.Status.FileSubscriptions[0].Name).To(Equal(name))
		})
	})
})
