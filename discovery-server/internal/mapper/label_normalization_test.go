// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVerifyLongApplicationLabel(t *testing.T) {
	g := NewWithT(t)
	app := strings.Repeat("a", 253)
	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{ApplicationLabelKey: labelutil.NormalizeLabelValue(app)}}}
	g.Expect(VerifyApplicationLabel(obj, app)).To(Succeed())
	g.Expect(VerifyApplicationLabel(obj, app+"b")).NotTo(Succeed())
}
