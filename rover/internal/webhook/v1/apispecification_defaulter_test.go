// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"errors"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiSpecification Defaulter", func() {
	ctx := context.Background()

	newCategories := func() *apiv1.ApiCategoryList {
		return &apiv1.ApiCategoryList{Items: []apiv1.ApiCategory{
			{Spec: apiv1.ApiCategorySpec{LabelValue: "Public", Active: true}},
			{Spec: apiv1.ApiCategorySpec{LabelValue: "Retired", Active: false}},
		}}
	}

	newDefaulter := func(list *apiv1.ApiCategoryList, err error) (*ApiSpecificationCustomDefaulter, *int) {
		calls := 0
		return &ApiSpecificationCustomDefaulter{
			ListApiCategories: func(context.Context) (*apiv1.ApiCategoryList, error) {
				calls++
				return list, err
			},
		}, &calls
	}

	newSpec := func(category string) *roverv1.ApiSpecification {
		return &roverv1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-api",
				Namespace: "test--my-group--my-team",
				Labels:    map[string]string{config.EnvironmentLabelKey: "test"},
			},
			Spec: roverv1.ApiSpecificationSpec{Category: category},
		}
	}

	DescribeTable("canonicalizes or preserves spec.category",
		func(input, expected string) {
			d, _ := newDefaulter(newCategories(), nil)
			spec := newSpec(input)
			Expect(d.Default(ctx, spec)).To(Succeed())
			Expect(spec.Spec.Category).To(Equal(expected))
		},
		Entry("upper case input", "PUBLIC", "Public"),
		Entry("lower case input", "public", "Public"),
		Entry("already canonical (idempotent)", "Public", "Public"),
		Entry("inactive match is left for the validator", "retired", "retired"),
		Entry("unknown category is left for the validator", "Internal", "Internal"),
		Entry("whitespace is not trimmed", " public ", " public "),
		Entry("empty category", "", ""),
	)

	It("is idempotent when applied twice", func() {
		d, _ := newDefaulter(newCategories(), nil)
		spec := newSpec("pUbLiC")
		Expect(d.Default(ctx, spec)).To(Succeed())
		Expect(d.Default(ctx, spec)).To(Succeed())
		Expect(spec.Spec.Category).To(Equal("Public"))
	})

	It("preserves the category when no ApiCategories exist", func() {
		for _, list := range []*apiv1.ApiCategoryList{nil, {}} {
			d, _ := newDefaulter(list, nil)
			spec := newSpec("PUBLIC")
			Expect(d.Default(ctx, spec)).To(Succeed())
			Expect(spec.Spec.Category).To(Equal("PUBLIC"))
		}
	})

	It("does nothing without an environment label", func() {
		d, calls := newDefaulter(newCategories(), nil)
		spec := newSpec("PUBLIC")
		spec.Labels = nil
		Expect(d.Default(ctx, spec)).To(Succeed())
		Expect(spec.Spec.Category).To(Equal("PUBLIC"))
		Expect(*calls).To(BeZero())
	})

	It("does nothing for objects being deleted", func() {
		d, calls := newDefaulter(newCategories(), nil)
		spec := newSpec("PUBLIC")
		now := metav1.NewTime(time.Now())
		spec.DeletionTimestamp = &now
		Expect(d.Default(ctx, spec)).To(Succeed())
		Expect(spec.Spec.Category).To(Equal("PUBLIC"))
		Expect(*calls).To(BeZero())
	})

	It("rejects the request when ApiCategories cannot be listed", func() {
		d, _ := newDefaulter(nil, errors.New("boom"))
		spec := newSpec("PUBLIC")
		err := d.Default(ctx, spec)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsInternalError(err)).To(BeTrue())
		Expect(spec.Spec.Category).To(Equal("PUBLIC"))
	})
})
