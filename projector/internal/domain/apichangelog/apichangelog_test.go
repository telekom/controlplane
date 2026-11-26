// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apichangelog_test

import (
	"context"

	"entgo.io/ent/privacy"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entapi "github.com/telekom/controlplane/controlplane-api/ent/api"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	"github.com/telekom/controlplane/projector/internal/domain/apichangelog"
	"github.com/telekom/controlplane/projector/internal/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiChangelog Translator", func() {
	tr := &apichangelog.Translator{}
	obj := &apiv1.ApiChangelog{
		ObjectMeta: metav1.ObjectMeta{Name: "eni-docs", Namespace: "prod--platform--narvi"},
		Spec:       apiv1.ApiChangelogSpec{BasePath: "/eni/docs/v1", Contents: "file-1", Hash: "h"},
	}

	It("maps the file ID, base path and team", func() {
		data, err := tr.Translate(context.Background(), obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(data.FileID).To(Equal("file-1"))
		Expect(data.BasePath).To(Equal("/eni/docs/v1"))
		Expect(data.TeamName).To(Equal("platform--narvi"))
	})

	It("skips a changelog without a file ID", func() {
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
		Expect(key).To(Equal(apichangelog.Key{BasePath: "/eni/docs/v1", Namespace: "prod--platform--narvi", TeamName: "platform--narvi"}))

		_, err = tr.KeyFromDelete(types.NamespacedName{Name: "eni-docs"}, nil)
		Expect(runtime.IsDeleteKeyLost(err)).To(BeTrue())
	})
})

var _ = Describe("ApiChangelog Repository", func() {
	var (
		client *ent.Client
		repo   *apichangelog.Repository
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")
		repo = apichangelog.NewRepository(client)

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

	data := func(basePath string) *apichangelog.Data {
		return &apichangelog.Data{BasePath: basePath, Namespace: "prod--platform--narvi", TeamName: "platform--narvi", FileID: "file-1"}
	}

	It("stores the file ID on the Api", func() {
		Expect(repo.Upsert(ctx, data("/eni/docs/v1"))).To(Succeed())

		stored, err := client.Api.Query().Where(entapi.BasePathEQ("/eni/docs/v1")).Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Changelog).To(Equal("file-1"))
	})

	It("reports a missing dependency while the Api is not projected", func() {
		err := repo.Upsert(ctx, data("/eni/other/v1"))
		Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
	})

	It("clears the file ID but keeps the Api on delete", func() {
		Expect(repo.Upsert(ctx, data("/eni/docs/v1"))).To(Succeed())
		Expect(repo.Delete(ctx, apichangelog.Key{BasePath: "/eni/docs/v1", Namespace: "prod--platform--narvi", TeamName: "platform--narvi"})).To(Succeed())

		stored, err := client.Api.Query().Where(entapi.BasePathEQ("/eni/docs/v1")).Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Changelog).To(BeEmpty())
	})

	It("deletes idempotently when the Api is gone", func() {
		Expect(repo.Delete(ctx, apichangelog.Key{BasePath: "/nope", Namespace: "prod--platform--narvi", TeamName: "platform--narvi"})).To(Succeed())
	})
})
