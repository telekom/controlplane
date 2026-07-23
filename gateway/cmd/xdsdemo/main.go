// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

// Command xdsdemo hosts the envtest-backed operator and demo process helpers.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: xdsdemo operator|management|health [arguments]")
	}

	var err error
	switch os.Args[1] {
	case "operator":
		err = runOperator(os.Args[2:])
	case "management":
		err = runManagement(os.Args[2:])
	case "health":
		err = runHealth(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "xdsdemo: "+format+"\n", args...)
	os.Exit(1)
}
