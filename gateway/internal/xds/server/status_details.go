// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"google.golang.org/grpc/status"
)

func addStatusDetails(value *status.Status, validationErrors ValidationErrors) (*status.Status, error) {
	// grpc/status accepts protoadapt.MessageV1; generated v2 messages satisfy it
	// through adaptation, but a concrete call per error avoids unsafe conversion.
	result := value
	for _, validationError := range validationErrors {
		updated, err := result.WithDetails(validationError)
		if err != nil {
			return nil, err
		}
		result = updated
	}
	return result, nil
}
