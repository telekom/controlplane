// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	basePath    = "/eni/test/v1"
	environment = "test"
)

// newActiveContext builds a fake-client context with the status.active field indexes
// that FindActiveAPI / FindActiveAPIExposure rely on.
func newActiveContext(objects ...crclient.Object) context.Context {
	sch := runtime.NewScheme()
	Expect(apiv1.AddToScheme(sch)).To(Succeed())
	fakeClient := fake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(objects...).
		WithIndex(&apiv1.Api{}, "status.active", func(obj crclient.Object) []string {
			return []string{boolField(obj.(*apiv1.Api).Status.Active)}
		}).
		WithIndex(&apiv1.ApiExposure{}, "status.active", func(obj crclient.Object) []string {
			return []string{boolField(obj.(*apiv1.ApiExposure).Status.Active)}
		}).
		Build()
	janitorClient := cclient.NewJanitorClient(cclient.NewScopedClient(fakeClient, environment))
	return cclient.WithClient(context.Background(), janitorClient)
}

func boolField(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func basePathLabels() map[string]string {
	return map[string]string{
		config.EnvironmentLabelKey: environment,
		apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(basePath),
	}
}

func setReady(conds *[]metav1.Condition) {
	*conds = []metav1.Condition{{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	}}
}

var _ = Describe("FindActiveAPIExposure", func() {
	// makeExposure builds an active ApiExposure; ready toggles the Ready condition.
	makeExposure := func(name string, ready bool, created metav1.Time) *apiv1.ApiExposure {
		exp := &apiv1.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         environment,
				Labels:            basePathLabels(),
				CreationTimestamp: created,
			},
			Spec:   apiv1.ApiExposureSpec{ApiBasePath: basePath},
			Status: apiv1.ApiExposureStatus{Active: true},
		}
		if ready {
			setReady(&exp.Status.Conditions)
		}
		return exp
	}

	It("returns found=false when no active exposure exists", func() {
		ctx := newActiveContext()
		found, exp, err := FindActiveAPIExposure(ctx, basePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(exp).To(BeNil())
	})

	It("returns an active exposure even when it is not ready", func() {
		notReady := makeExposure("not-ready", false, metav1.Now())
		ctx := newActiveContext(notReady)
		found, exp, err := FindActiveAPIExposure(ctx, basePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(exp).NotTo(BeNil())
		Expect(exp.Name).To(Equal("not-ready"))
		// Readiness is no longer checked by the getter.
		Expect(condition.EnsureReady(exp)).To(HaveOccurred())
	})

	It("selects the oldest when multiple active exposures exist", func() {
		old := makeExposure("older", true, metav1.Date(2024, 1, 1, 0, 0, 0, 0, metav1.Now().Location()))
		newer := makeExposure("newer", true, metav1.Date(2025, 1, 1, 0, 0, 0, 0, metav1.Now().Location()))
		// Insert newest first to prove sorting, not insertion order, drives the result.
		ctx := newActiveContext(newer, old)
		found, exp, err := FindActiveAPIExposure(ctx, basePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(exp.Name).To(Equal("older"))
	})

	It("is deterministic on equal creation timestamps (stable order)", func() {
		ts := metav1.Now()
		a := makeExposure("a", true, ts)
		b := makeExposure("b", true, ts)
		// Same input order across runs must yield the same first element.
		ctx1 := newActiveContext(a, b)
		found1, exp1, err1 := FindActiveAPIExposure(ctx1, basePath)
		Expect(err1).NotTo(HaveOccurred())
		Expect(found1).To(BeTrue())
		ctx2 := newActiveContext(a, b)
		_, exp2, _ := FindActiveAPIExposure(ctx2, basePath)
		Expect(exp1.Name).To(Equal(exp2.Name))
	})
})

var _ = Describe("FindActiveAPI", func() {
	// makeApi builds an active Api; ready toggles the Ready condition.
	makeApi := func(name string, ready bool, created metav1.Time) *apiv1.Api {
		api := &apiv1.Api{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         environment,
				Labels:            basePathLabels(),
				CreationTimestamp: created,
			},
			Spec:   apiv1.ApiSpec{BasePath: basePath},
			Status: apiv1.ApiStatus{Active: true},
		}
		if ready {
			setReady(&api.Status.Conditions)
		}
		return api
	}

	It("returns found=false when no active Api exists", func() {
		ctx := newActiveContext()
		found, api, err := FindActiveAPI(ctx, basePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(api).To(BeNil())
	})

	It("returns an active Api even when it is not ready", func() {
		notReady := makeApi("not-ready", false, metav1.Now())
		ctx := newActiveContext(notReady)
		found, api, err := FindActiveAPI(ctx, basePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(api.Name).To(Equal("not-ready"))
	})

	It("selects the oldest when multiple active Apis exist", func() {
		old := makeApi("older", true, metav1.Date(2024, 1, 1, 0, 0, 0, 0, metav1.Now().Location()))
		newer := makeApi("newer", true, metav1.Date(2025, 1, 1, 0, 0, 0, 0, metav1.Now().Location()))
		ctx := newActiveContext(newer, old)
		found, api, err := FindActiveAPI(ctx, basePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(api.Name).To(Equal("older"))
	})
})
