// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

type StatusEval struct {
	obj    types.Object
	status types.ObjectStatus
}

func NewStatusEval(obj types.Object, status types.ObjectStatus) StatusEval {
	return StatusEval{
		obj:    obj,
		status: status,
	}
}

func (s StatusEval) PrettyPrint(w io.Writer, format string) error {
	switch format {
	case "console":
		return s.ConsolePrettyPrint(w)
	case "json":
		return s.JsonPrettyPrint(w)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

// Group errors, warnings and infos by resource kind
// Print using a structured format and "eye-catchers" ❌ ⚠️ ℹ️
func (s StatusEval) ConsolePrettyPrint(w io.Writer) error {
	buf := bytes.NewBuffer(nil)

	// Print resource details
	fmt.Fprintf(buf, "%s: %s/%s\n", "Resource", s.obj.GetKind(), s.obj.GetName())
	fmt.Fprintf(buf, "Status: %s | Processing: %s\n", s.status.GetOverallStatus(), s.status.GetProcessingState())

	// Group information messages by kind
	if len(s.status.GetInfo()) > 0 {
		fmt.Fprintf(buf, "\nℹ️  Information\n")
		fmt.Fprintf(buf, "-------------\n")

		// Group by kind
		kindMap := make(map[string][]types.StatusInfo)
		for _, info := range s.status.GetInfo() {
			kindMap[info.Resource.Kind] = append(kindMap[info.Resource.Kind], info)
		}

		// Print grouped by kind
		for kind, infos := range kindMap {
			fmt.Fprintf(buf, "Kind: %s\n", kind)
			for _, info := range infos {
				fmt.Fprintf(buf, "  Resource: %s\n", info.Resource.Name)
				fmt.Fprintf(buf, "    Cause:   %s\n", info.Cause)
				fmt.Fprintf(buf, "    Message: %s\n", info.Message)
				if info.Details != "" {
					fmt.Fprintf(buf, "    Details: %s\n", info.Details)
				}
			}
			fmt.Fprintf(buf, "\n")
		}
	}

	// Group warning messages by kind
	if len(s.status.GetWarnings()) > 0 {
		fmt.Fprintf(buf, "\n⚠️  Warnings\n")
		fmt.Fprintf(buf, "----------\n")

		// Group by kind
		kindMap := make(map[string][]types.StatusInfo)
		for _, warning := range s.status.GetWarnings() {
			kindMap[warning.Resource.Kind] = append(kindMap[warning.Resource.Kind], warning)
		}

		// Print grouped by kind
		for kind, warnings := range kindMap {
			fmt.Fprintf(buf, "Kind: %s\n", kind)
			for _, warning := range warnings {
				fmt.Fprintf(buf, "  Resource: %s\n", warning.Resource.Name)
				fmt.Fprintf(buf, "    Cause:   %s\n", warning.Cause)
				fmt.Fprintf(buf, "    Message: %s\n", warning.Message)
				if warning.Details != "" {
					fmt.Fprintf(buf, "    Details: %s\n", warning.Details)
				}
			}
			fmt.Fprintf(buf, "\n")
		}
	}

	// Group error messages by kind
	if len(s.status.GetErrors()) > 0 {
		fmt.Fprintf(buf, "\n❌ Errors\n")
		fmt.Fprintf(buf, "--------\n")

		// Group by kind
		kindMap := make(map[string][]types.StatusInfo)
		for _, err := range s.status.GetErrors() {
			kindMap[err.Resource.Kind] = append(kindMap[err.Resource.Kind], err)
		}

		// Print grouped by kind
		for kind, errors := range kindMap {
			fmt.Fprintf(buf, "Kind: %s\n", kind)
			for _, err := range errors {
				fmt.Fprintf(buf, "  Resource: %s\n", err.Resource.Name)
				fmt.Fprintf(buf, "    Cause:   %s\n", err.Cause)
				fmt.Fprintf(buf, "    Message: %s\n", err.Message)
				if err.Details != "" {
					fmt.Fprintf(buf, "    Details: %s\n", err.Details)
				}
			}
			fmt.Fprintf(buf, "\n")
		}
	}

	// If no messages, show success
	if !s.status.HasInfo() && !s.status.HasWarnings() && !s.status.HasErrors() && s.IsSuccess() {
		fmt.Fprintf(buf, "✅ Operation completed successfully\n")
	}

	if _, err := buf.WriteTo(w); err != nil {
		return err
	}
	return nil
}

func (s StatusEval) JsonPrettyPrint(w io.Writer) error {
	// Group status information by kind
	type GroupedStatusInfo map[string][]types.StatusInfo

	// Group errors, warnings, and info by resource kind
	groupedErrors := make(GroupedStatusInfo)
	groupedWarnings := make(GroupedStatusInfo)
	groupedInfo := make(GroupedStatusInfo)

	// Process errors
	for _, err := range s.status.GetErrors() {
		groupedErrors[err.Resource.Kind] = append(groupedErrors[err.Resource.Kind], err)
	}

	// Process warnings
	for _, warning := range s.status.GetWarnings() {
		groupedWarnings[warning.Resource.Kind] = append(groupedWarnings[warning.Resource.Kind], warning)
	}

	// Process info
	for _, info := range s.status.GetInfo() {
		groupedInfo[info.Resource.Kind] = append(groupedInfo[info.Resource.Kind], info)
	}

	// Create a structured representation of the status for JSON output
	output := struct {
		Resource        string            `json:"resource"`
		Status          string            `json:"status"`
		ProcessingState string            `json:"processingState"`
		Errors          GroupedStatusInfo `json:"errors,omitempty"`
		Warnings        GroupedStatusInfo `json:"warnings,omitempty"`
		Info            GroupedStatusInfo `json:"info,omitempty"`
		Success         bool              `json:"success"`
	}{
		Resource:        fmt.Sprintf("%s/%s", s.obj.GetKind(), s.obj.GetName()),
		Status:          s.status.GetOverallStatus(),
		ProcessingState: s.status.GetProcessingState(),
		Errors:          groupedErrors,
		Warnings:        groupedWarnings,
		Info:            groupedInfo,
		Success:         s.IsSuccess(),
	}

	// Marshal with indentation for pretty printing
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}

	// Write to output
	_, err = w.Write(append(data, '\n'))
	return err
}

func (s StatusEval) IsSuccess() bool {
	return s.status.GetOverallStatus() == "complete" && s.status.GetProcessingState() == "done"
}

func (s StatusEval) IsFailure() bool {
	return s.status.GetOverallStatus() == "failed"
}

func (s StatusEval) IsBlocked() bool {
	return s.status.GetOverallStatus() == "blocked"
}

func (s StatusEval) IsProcessed() bool {
	return s.status.GetProcessingState() == "done"
}
