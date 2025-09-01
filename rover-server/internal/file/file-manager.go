// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package file

// TODO: Check if this should be moved to the file-manager/api package
import (
	"sync"

	"github.com/telekom/controlplane/file-manager/api"
)

var (
	once        sync.Once
	fileManager api.FileManager
	options     []api.Option
)

func AppendOption(option api.Option) {
	if len(options) == 0 {
		options = make([]api.Option, 0)
	}
	options = append(options, option)
}

var GetFileManager = func() api.FileManager {
	once.Do(func() {
		fileManager = api.New(options...)
	})
	return fileManager
}
