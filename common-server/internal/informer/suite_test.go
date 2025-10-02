// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var timeout = 5 * time.Second
var interval = 500 * time.Millisecond

func TestInformer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Informer Suite")
}
