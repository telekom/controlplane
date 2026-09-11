// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApiSpecificationHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApiSpecification Handler Suite")
}
