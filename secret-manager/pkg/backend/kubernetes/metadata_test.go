// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestSecretMetadataLabelBounds(t *testing.T) {
	g := NewWithT(t)
	team := strings.Repeat("t", 80)
	app := strings.Repeat("a", 253)
	secret := NewSecretObj("test", team, app)
	g.Expect(secret.Name).To(Equal(app))
	for _, value := range secret.Labels {
		g.Expect(validation.IsValidLabelValue(value)).To(BeEmpty())
	}
	g.Expect(secret.Labels["cp.ei.telekom.de/application"]).NotTo(Equal(NewSecretObj("test", team, app+"b").Labels["cp.ei.telekom.de/application"]))
}
