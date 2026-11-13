// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	cperrors "github.com/telekom/controlplane/common/pkg/errors"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const hostileText = "<script>leak</script> internal-host:6443"

func forbiddenWithCause(causeType metav1.CauseType, msg string) *apierrors.StatusError {
	err := apierrors.NewForbidden(schema.GroupResource{Group: "g", Resource: "r"}, "n", fmt.Errorf("%s", hostileText))
	err.ErrStatus.Details.Causes = []metav1.StatusCause{{Type: causeType, Message: msg, Field: hostileText}}
	return err
}

var _ = Describe("k8sToMutationError", func() {
	DescribeTable("forwards only the marked public message",
		func(err error, code model.ErrorCode, msg string) {
			Expect(service.K8sToMutationError(err)).To(Equal(model.MutationError{Code: code, Message: msg}))
		},
		Entry("forbidden", forbiddenWithCause(cperrors.CauseTypePublicMessage, "rotation not supported"),
			model.ErrorCodeForbidden, "rotation not supported"),
		Entry("other status keeps its code",
			cperrors.WithPublicMessage(apierrors.NewConflict(schema.GroupResource{}, "n", fmt.Errorf("%s", hostileText)), "try later"),
			model.ErrorCodeConflict, "try later"),
		Entry("wrapped", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", forbiddenWithCause(cperrors.CauseTypePublicMessage, "wrapped msg"))),
			model.ErrorCodeForbidden, "wrapped msg"),
		Entry("unknown cause", forbiddenWithCause("other/Cause", hostileText),
			model.ErrorCodeForbidden, "operation forbidden by cluster policy"),
		Entry("empty public message", forbiddenWithCause(cperrors.CauseTypePublicMessage, ""),
			model.ErrorCodeForbidden, "operation forbidden by cluster policy"),
		Entry("no cause", apierrors.NewForbidden(schema.GroupResource{}, "n", fmt.Errorf("%s", hostileText)),
			model.ErrorCodeForbidden, "operation forbidden by cluster policy"),
	)
})
