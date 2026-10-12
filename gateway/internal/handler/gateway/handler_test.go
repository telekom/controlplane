// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_test

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	gwhandler "github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GatewayHandler", func() {
	Describe("CreateOrUpdate()", func() {
		It("sets DoneProcessing and Ready conditions", func() {
			handler := &gwhandler.GatewayHandler{}
			gw := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "test-gw"},
			}

			err := handler.CreateOrUpdate(context.Background(), gw)
			Expect(err).NotTo(HaveOccurred())

			Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeProcessing)).To(BeFalse())
		})
	})

	Describe("Delete()", func() {
		It("returns nil", func() {
			handler := &gwhandler.GatewayHandler{}
			gw := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "test-gw"},
			}

			err := handler.Delete(context.Background(), gw)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("GatewayHandler trust", func() {
	It("sets TrustReady=True when trust resolves", func() {
		h := &gwhandler.GatewayHandler{Trust: validatorFunc(func() error { return nil })}
		gw := &gatewayv1.Gateway{}
		Expect(h.CreateOrUpdate(context.Background(), gw)).To(Succeed())
		Expect(meta.IsStatusConditionTrue(gw.GetConditions(), gwhandler.ConditionTypeTrustReady)).To(BeTrue())
	})

	It("sets TrustReady=False and stays Ready on trust errors", func() {
		h := &gwhandler.GatewayHandler{Trust: validatorFunc(func() error {
			return kongutil.NewTrustError("LabelMissing", ctrlerrors.BlockedErrorf("label missing"))
		})}
		gw := &gatewayv1.Gateway{}
		Expect(h.CreateOrUpdate(context.Background(), gw)).To(Succeed())
		Expect(meta.IsStatusConditionFalse(gw.GetConditions(), gwhandler.ConditionTypeTrustReady)).To(BeTrue())
		Expect(meta.FindStatusCondition(gw.GetConditions(), gwhandler.ConditionTypeTrustReady).Reason).To(Equal("LabelMissing"))
		Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
	})

	It("returns non-trust errors", func() {
		h := &gwhandler.GatewayHandler{Trust: validatorFunc(func() error { return errors.New("boom") })}
		gw := &gatewayv1.Gateway{}
		Expect(h.CreateOrUpdate(context.Background(), gw)).To(MatchError(ContainSubstring("boom")))
		Expect(meta.FindStatusCondition(gw.GetConditions(), gwhandler.ConditionTypeTrustReady)).To(BeNil())
	})
})

type validatorFunc func() error

func (f validatorFunc) Validate(context.Context, *gatewayv1.Gateway) error { return f() }
