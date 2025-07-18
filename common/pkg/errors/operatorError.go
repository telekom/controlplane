// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package operatorErrors

type OperatorError interface {
	error
	Retriable() bool
}
