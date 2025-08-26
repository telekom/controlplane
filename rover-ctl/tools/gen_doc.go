// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"

	"github.com/spf13/cobra/doc"
	"github.com/telekom/controlplane/rover-ctl/cmd"
)

func main() {
	if err := doc.GenMarkdownTree(cmd.NewRootCommand(), "docs"); err != nil {
		log.Fatalf("Failed to generate markdown docs: %v", err)
	}
}
