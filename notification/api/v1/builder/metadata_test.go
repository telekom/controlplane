// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/config"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestNotificationLabelBounds(t *testing.T) {
	g := NewWithT(t)
	n := &notificationv1.Notification{Spec: notificationv1.NotificationSpec{Purpose: strings.Repeat("purpose-", 40) + "completed"}}
	ensureLabels(n)
	value := n.Labels[config.BuildLabelKey("purpose")]
	g.Expect(validation.IsValidLabelValue(value)).To(BeEmpty())
	g.Expect(n.Spec.Purpose).To(HaveLen(329))
	ensureLabels(n)
	g.Expect(n.Labels[config.BuildLabelKey("purpose")]).To(Equal(value))
}
