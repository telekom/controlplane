// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runtime_test

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/projector/internal/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// --- mock types ---

type testData struct {
	Name string
}

type testKey string

// mockTranslator implements runtime.Translator for testing.
type mockTranslator struct {
	shouldSkip    bool
	skipReason    string
	translateData *testData
	translateErr  error
	keyFromObj    testKey
	keyFromDel    testKey
	keyFromDelErr error

	// call tracking
	translateCalled  bool
	keyFromDelCalled bool
	shouldSkipCalled bool
	keyFromObjCalled bool
}

func (m *mockTranslator) ShouldSkip(_ *corev1.ConfigMap) (bool, string) {
	m.shouldSkipCalled = true
	return m.shouldSkip, m.skipReason
}

func (m *mockTranslator) Translate(_ context.Context, _ *corev1.ConfigMap) (*testData, error) {
	m.translateCalled = true
	return m.translateData, m.translateErr
}

func (m *mockTranslator) KeyFromObject(_ *corev1.ConfigMap) testKey {
	m.keyFromObjCalled = true
	return m.keyFromObj
}

func (m *mockTranslator) KeyFromDelete(_ types.NamespacedName, _ *corev1.ConfigMap) (testKey, error) {
	m.keyFromDelCalled = true
	return m.keyFromDel, m.keyFromDelErr
}

// mockRepository implements runtime.Repository for testing.
type mockRepository struct {
	upsertErr error
	deleteErr error

	upsertCalled bool
	deleteCalled bool
	upsertData   *testData
	deleteKey    testKey
}

func (m *mockRepository) Upsert(_ context.Context, data *testData) error {
	m.upsertCalled = true
	m.upsertData = data
	return m.upsertErr
}

func (m *mockRepository) Delete(_ context.Context, key testKey) error {
	m.deleteCalled = true
	m.deleteKey = key
	return m.deleteErr
}

// --- tests ---

var _ = Describe("Processor", func() {
	var (
		ctx        context.Context
		translator *mockTranslator
		repo       *mockRepository
		proc       *runtime.Processor[*corev1.ConfigMap, *testData, testKey]
		obj        *corev1.ConfigMap
		req        types.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		translator = &mockTranslator{}
		repo = &mockRepository{}
		proc = runtime.NewProcessor[*corev1.ConfigMap, *testData, testKey](translator, repo)
		obj = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: "default",
			},
		}
		req = types.NamespacedName{Name: "test-cm", Namespace: "default"}
	})

	Describe("Upsert", func() {
		It("returns ErrSkipSync when ShouldSkip is true", func() {
			translator.shouldSkip = true
			translator.skipReason = "missing field"

			err := proc.Upsert(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsSkipSync(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("missing field"))

			Expect(translator.shouldSkipCalled).To(BeTrue())
			Expect(translator.translateCalled).To(BeFalse())
			Expect(repo.upsertCalled).To(BeFalse())
		})

		It("propagates translate errors without calling repository", func() {
			translateErr := errors.New("bad spec")
			translator.translateErr = translateErr

			err := proc.Upsert(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("translate"))
			Expect(errors.Is(err, translateErr)).To(BeTrue())

			Expect(translator.translateCalled).To(BeTrue())
			Expect(repo.upsertCalled).To(BeFalse())
		})

		It("calls translator and repository in order on happy path", func() {
			data := &testData{Name: "translated"}
			translator.translateData = data

			err := proc.Upsert(ctx, obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(translator.shouldSkipCalled).To(BeTrue())
			Expect(translator.translateCalled).To(BeTrue())
			Expect(repo.upsertCalled).To(BeTrue())
			Expect(repo.upsertData).To(Equal(data))
		})

		It("propagates repository upsert errors", func() {
			translator.translateData = &testData{Name: "ok"}
			repoErr := errors.New("db connection lost")
			repo.upsertErr = repoErr

			err := proc.Upsert(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, repoErr)).To(BeTrue())
		})
	})

	Describe("Delete", func() {
		It("calls repository.Delete with correct key on happy path", func() {
			translator.keyFromDel = testKey("test-cm")

			err := proc.Delete(ctx, req, obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(translator.keyFromDelCalled).To(BeTrue())
			Expect(repo.deleteCalled).To(BeTrue())
			Expect(repo.deleteKey).To(Equal(testKey("test-cm")))
		})

		It("propagates ErrDeleteKeyLost from translator", func() {
			translator.keyFromDelErr = runtime.ErrDeleteKeyLost

			err := proc.Delete(ctx, req, nil)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDeleteKeyLost(err)).To(BeTrue())

			Expect(translator.keyFromDelCalled).To(BeTrue())
			Expect(repo.deleteCalled).To(BeFalse())
		})

		It("propagates arbitrary translator errors", func() {
			otherErr := errors.New("unexpected")
			translator.keyFromDelErr = otherErr

			err := proc.Delete(ctx, req, nil)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, otherErr)).To(BeTrue())
			Expect(repo.deleteCalled).To(BeFalse())
		})

		It("propagates repository delete errors", func() {
			translator.keyFromDel = testKey("test-cm")
			repoErr := errors.New("db error")
			repo.deleteErr = repoErr

			err := proc.Delete(ctx, req, obj)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, repoErr)).To(BeTrue())
		})
	})
})
