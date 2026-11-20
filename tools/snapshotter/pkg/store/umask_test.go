// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// umask determines the process umask by creating a probe file.
func umask() os.FileMode {
	path := filepath.Join(GinkgoT().TempDir(), "probe")
	Expect(os.WriteFile(path, nil, 0o777)).To(Succeed())
	info, err := os.Stat(path)
	Expect(err).NotTo(HaveOccurred())
	return 0o777 &^ info.Mode().Perm()
}
