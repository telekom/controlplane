// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package data

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
)

func getCurrentFileDir() string {
	// Get the file path of the current file
	_, file, _, _ := runtime.Caller(0)

	// Return the directory of the file
	return filepath.Dir(file)
}

func ReadFile(testing ginkgo.FullGinkgoTInterface, filePath string) []byte {
	testDataDir, err := filepath.Abs(getCurrentFileDir())
	require.NoError(testing, err)
	testDataFile := filepath.Join(testDataDir, filePath)
	file, err := os.ReadFile(testDataFile)
	require.NoError(testing, err)
	return file
}
