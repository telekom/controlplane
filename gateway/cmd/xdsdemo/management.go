// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"
)

const targetWaitTimeout = 30 * time.Second

func runManagement(args []string) error {
	flags := flag.NewFlagSet("management", flag.ContinueOnError)
	targetFile := flags.String("target-file", "/state/target-id", "file containing the operator target ID")
	binary := flags.String("binary", "/xds-management-server", "management-server binary")
	if err := flags.Parse(args); err != nil {
		return err
	}

	targetID, err := waitForTarget(*targetFile, targetWaitTimeout)
	if err != nil {
		return err
	}
	serverArgs := managementServerArgs(*binary, targetID, flags.Args())
	return syscall.Exec(*binary, serverArgs, os.Environ()) //nolint:gosec // Binary path is operator-configured demo input.
}

func managementServerArgs(binary, targetID string, args []string) []string {
	return append([]string{
		binary,
		"-node-mappings=demo-envoy-a=" + targetID + ",demo-envoy-b=" + targetID,
	}, args...)
}

func waitForTarget(path string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		content, err := os.ReadFile(path) //nolint:gosec // Path is operator-configured demo state.
		if err == nil && strings.TrimSpace(string(content)) != "" {
			return strings.TrimSpace(string(content)), nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("reading target file: %w", err)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("target file %q was not populated within %s", path, timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func runHealth(args []string) error {
	flags := flag.NewFlagSet("health", flag.ContinueOnError)
	timeout := flags.Duration("timeout", 2*time.Second, "request timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("health requires one URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, flags.Arg(0), http.NoBody)
	if err != nil {
		return fmt.Errorf("creating health request: %w", err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("requesting health: %w", err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "xdsdemo: closing health response failed: %v\n", closeErr)
		}
	}()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned %s", response.Status)
	}
	return nil
}
