// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiroadmap_test

import (
	"context"

	"entgo.io/ent/privacy"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entapi "github.com/telekom/controlplane/controlplane-api/ent/api"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	"github.com/telekom/controlplane/projector/internal/domain/apiroadmap"
	"github.com/telekom/controlplane/projector/internal/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiRoadmap Translator", func() {
	tr := &apiroadmap.Translator{}
	obj := &apiv1.ApiRoadmap{
		ObjectMeta: metav1.ObjectMeta{Name: "eni-docs", Namespace: "prod--platform--narvi"},
		Spec:       apiv1.ApiRoadmapSpec{BasePath: "/eni/docs/v1", Contents: "file-1", Hash: "h"},
	}

	It("maps the file ID, base path and team", func() {
		data, err := tr.Translate(context.Background(), obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(data.FileID).To(Equal("file-1"))
		Expect(data.BasePath).To(Equal("/eni/docs/v1"))
		Expect(data.TeamName).To(Equal("platform--narvi"))
	})

	It("skips a Roadmap without a file ID", func() {
		empty := obj.DeepCopy()
		empty.Spec.Contents = ""
		skip, _ := tr.ShouldSkip(empty)
		Expect(skip).To(BeTrue())

		skip, _ = tr.ShouldSkip(obj)
		Expect(skip).To(BeFalse())
	})

	It("derives the delete key from the last known object only", func() {
		key, err := tr.KeyFromDelete(types.NamespacedName{Name: "eni-docs"}, obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(key).To(Equal(apiroadmap.Key{BasePath: "/eni/docs/v1", Namespace: "prod--platform--narvi", TeamName: "platform--narvi"}))

		_, err = tr.KeyFromDelete(types.NamespacedName{Name: "eni-docs"}, nil)
		Expect(runtime.IsDeleteKeyLost(err)).To(BeTrue())
	})
})

var _ = Describe("ApiRoadmap Repository", func() {
	var (
		client *ent.Client
		repo   *apiroadmap.Repository
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")
		repo = apiroadmap.NewRepository(client)

		tm, err := client.Team.Create().
			SetName("platform--narvi").SetEmail("n@example.com").SetNamespace("platform--narvi").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Api.Create().
			SetNamespace("prod--platform--narvi").SetBasePath("/eni/docs/v1").SetVersion("1.0.0").
			SetOwner(tm).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = client.Close()
	})

	data := func(basePath string) *apiroadmap.Data {
		return &apiroadmap.Data{BasePath: basePath, Namespace: "prod--platform--narvi", TeamName: "platform--narvi", FileID: "file-1"}
	}

	It("stores the file ID on the Api", func() {
		Expect(repo.Upsert(ctx, data("/eni/docs/v1"))).To(Succeed())

		stored, err := client.Api.Query().Where(entapi.BasePathEQ("/eni/docs/v1")).Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Roadmap).To(Equal("file-1"))
	})

	It("reports a missing dependency while the Api is not projected", func() {
		err := repo.Upsert(ctx, data("/eni/other/v1"))
		Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
	})

	It("clears the file ID but keeps the Api on delete", func() {
		Expect(repo.Upsert(ctx, data("/eni/docs/v1"))).To(Succeed())
		Expect(repo.Delete(ctx, apiroadmap.Key{BasePath: "/eni/docs/v1", Namespace: "prod--platform--narvi", TeamName: "platform--narvi"})).To(Succeed())

		stored, err := client.Api.Query().Where(entapi.BasePathEQ("/eni/docs/v1")).Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Roadmap).To(BeEmpty())
	})

	It("moves the file ID when the mirror is retargeted to another major version", func() {
		tm, err := client.Team.Query().Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Api.Create().
			SetNamespace("prod--platform--narvi").SetBasePath("/eni/docs/v2").SetVersion("2.0.0").
			SetOwner(tm).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(repo.Upsert(ctx, data("/eni/docs/v1"))).To(Succeed())
		Expect(repo.Upsert(ctx, data("/eni/docs/v2"))).To(Succeed())

		v1, err := client.Api.Query().Where(entapi.BasePathEQ("/eni/docs/v1")).Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(v1.Roadmap).To(BeEmpty())
		v2, err := client.Api.Query().Where(entapi.BasePathEQ("/eni/docs/v2")).Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(v2.Roadmap).To(Equal("file-1"))
	})

	It("keeps the previous assignment while the new target Api is not projected", func() {
		Expect(repo.Upsert(ctx, data("/eni/docs/v1"))).To(Succeed())
		err := repo.Upsert(ctx, data("/eni/docs/v2"))
		Expect(runtime.IsDependencyMissing(err)).To(BeTrue())

		v1, err := client.Api.Query().Where(entapi.BasePathEQ("/eni/docs/v1")).Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(v1.Roadmap).To(Equal("file-1"))
	})

	It("deletes idempotently when the Api is gone", func() {
		Expect(repo.Delete(ctx, apiroadmap.Key{BasePath: "/nope", Namespace: "prod--platform--narvi", TeamName: "platform--narvi"})).To(Succeed())
	})
})
