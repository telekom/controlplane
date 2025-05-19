// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package operatorErrors

var _ OperatorError = &operatorError{}

/*
Should be used for errors related to other resources that the reconcilied resource depends on.
For example if you are not able to get an Api when handling an ApiExposure.
*/

type ErrorType string

const (
	ErrorTypeApi        ErrorType = "ApiError"
	ErrorTypeValidation ErrorType = "ValidationError"
	ErrorTypeResources  ErrorType = "ResourcesError"
)

type operatorError struct {
	errorType ErrorType
	err       error
	message   string
	retriable bool
}

func (e operatorError) Error() string {
	if e.err == nil {
		return e.message + " -- " + " Inner error is nil!"
	} else {
		return e.message + " -- " + e.err.Error()
	}
}

func (e operatorError) Retriable() bool {
	return e.retriable
}

func NewRetriableResourcesError(err error, message string) OperatorError {
	return operatorError{
		errorType: ErrorTypeResources,
		err:       err,
		message:   message,
		retriable: true,
	}
}

func NewNonRetriableResourcesError(err error, message string) OperatorError {
	return operatorError{
		errorType: ErrorTypeResources,
		err:       err,
		message:   message,
		retriable: false,
	}
}

func NewRetriableApiError(err error, message string) OperatorError {
	return operatorError{
		errorType: ErrorTypeApi,
		err:       err,
		message:   message,
		retriable: true,
	}
}

func NewNonRetriableApiError(err error, message string) OperatorError {
	return operatorError{
		errorType: ErrorTypeApi,
		err:       err,
		message:   message,
		retriable: false,
	}
}

func NewRetriableValidationError(err error, message string) operatorError {
	return operatorError{
		errorType: ErrorTypeValidation,
		err:       err,
		message:   message,
		retriable: true,
	}
}

func NewNonRetriableValidationError(err error, message string) operatorError {
	return operatorError{
		errorType: ErrorTypeValidation,
		err:       err,
		message:   message,
		retriable: false,
	}
}
