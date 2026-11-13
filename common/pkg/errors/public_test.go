// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package errors_test

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	cperrors "github.com/telekom/controlplane/common/pkg/errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PublicMessage", func() {
	gr := schema.GroupResource{Group: "g", Resource: "r"}

	It("round-trips a public message", func() {
		err := cperrors.WithPublicMessage(apierrors.NewForbidden(gr, "n", fmt.Errorf("internal")), "safe text")
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
		msg, ok := cperrors.PublicMessage(err)
		Expect(ok).To(BeTrue())
		Expect(msg).To(Equal("safe text"))
	})

	It("ignores empty, unmarked and nil inputs", func() {
		_, ok := cperrors.PublicMessage(cperrors.WithPublicMessage(apierrors.NewForbidden(gr, "n", fmt.Errorf("x")), ""))
		Expect(ok).To(BeFalse())
		_, ok = cperrors.PublicMessage(apierrors.NewForbidden(gr, "n", fmt.Errorf("x")))
		Expect(ok).To(BeFalse())
		_, ok = cperrors.PublicMessage(&apierrors.StatusError{})
		Expect(ok).To(BeFalse())
		_, ok = cperrors.PublicMessage(nil)
		Expect(ok).To(BeFalse())
	})
})
