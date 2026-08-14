// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filespecification

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFileSpecificationHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FileSpecification Handler Suite")
}

const testEnvironment = "test"

var _ = Describe("FileSpecificationHandler", func() {
	var (
		ctx        context.Context
		fakeClient ctrlclient.Client
		handler    *FileSpecificationHandler
	)

	newFileSpec := func(name string) *roverv1.FileSpecification {
		return &roverv1.FileSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: roverv1.FileSpecificationSpec{
				Description:   "demo file type",
				Specification: "file-id-123",
				StorageType:   roverv1.FileStorageTypeSFTP,
			},
		}
	}

	// newContext returns a context carrying a fresh JanitorClient over the shared
	// fake client, so AnyChanged() reflects only the operations of one reconcile.
	newContext := func() context.Context {
		scoped := commonclient.NewScopedClient(fakeClient, testEnvironment)
		janitor := commonclient.NewJanitorClient(scoped)
		return commonclient.WithClient(logr.NewContext(ctx, logr.Discard()), janitor)
	}

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(roverv1.AddToScheme(scheme)).To(Succeed())
		Expect(filev1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		handler = &FileSpecificationHandler{}
	})

	getFileType := func(name string) *filev1.FileType {
		fileType := &filev1.FileType{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, fileType)).To(Succeed())
		return fileType
	}

	It("should create a FileType from the FileSpecification and mark it provisioning", func() {
		fileSpec := newFileSpec("demo-sftp-spec-v1")

		Expect(handler.CreateOrUpdate(newContext(), fileSpec)).To(Succeed())

		fileType := getFileType("demo-sftp-spec-v1")
		Expect(fileType.Spec.Description).To(Equal("demo file type"))
		Expect(fileType.Labels).To(HaveKey(filev1.FileTypeNameLabelKey))
		Expect(fileType.Labels).To(HaveKeyWithValue(config.EnvironmentLabelKey, testEnvironment))
		Expect(fileType.OwnerReferences).To(HaveLen(1))
		Expect(fileType.OwnerReferences[0].Name).To(Equal("demo-sftp-spec-v1"))

		// Status references the created FileType.
		Expect(fileSpec.Status.FileType.Name).To(Equal("demo-sftp-spec-v1"))
		Expect(fileSpec.Status.FileType.Namespace).To(Equal("default"))

		// First reconcile changed the cluster, so it is not yet ready.
		ready := meta.FindStatusCondition(fileSpec.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
	})

	It("should mark the FileSpecification ready when nothing changed (idempotent)", func() {
		fileSpec := newFileSpec("demo-sftp-spec-v1")
		Expect(handler.CreateOrUpdate(newContext(), fileSpec)).To(Succeed())

		// Second reconcile with a fresh janitor client: no change expected.
		fileSpec = newFileSpec("demo-sftp-spec-v1")
		Expect(handler.CreateOrUpdate(newContext(), fileSpec)).To(Succeed())

		ready := meta.FindStatusCondition(fileSpec.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
	})

	It("should normalize the FileType name derived from the specification name", func() {
		fileSpec := newFileSpec("De.Telekom.Foo.v1")

		Expect(handler.CreateOrUpdate(newContext(), fileSpec)).To(Succeed())

		// dots -> hyphens and lower-cased.
		getFileType("de-telekom-foo-v1")
		Expect(fileSpec.Status.FileType.Name).To(Equal("de-telekom-foo-v1"))
	})

	It("should return nil on Delete", func() {
		Expect(handler.Delete(newContext(), newFileSpec("demo-sftp-spec-v1"))).To(Succeed())
	})
})
