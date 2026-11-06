// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
)

var _ = DescribeTable("TeamNameFromNamespace",
	func(namespace, expected string) {
		Expect(organizationv1.TeamNameFromNamespace(namespace)).To(Equal(expected))
	},
	Entry("environment--group--team", "env--group--team", "group--team"),
	Entry("single-hyphen components", "prod-env--my-group--my-team", "my-group--my-team"),
	Entry("no separator falls back to input", "no-separator", "no-separator"),
	Entry("empty input", "", ""),
	Entry("two segments drop the first only", "group--team", "team"),
	Entry("extra segments are kept", "env--a--b--c", "a--b--c"),
	Entry("trailing separator", "env--", ""),
	Entry("leading separator", "--group--team", "group--team"),
	Entry("triple hyphen splits at first pair", "env---group--team", "-group--team"),
)
