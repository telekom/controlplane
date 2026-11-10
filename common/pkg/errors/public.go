// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CauseTypePublicMessage marks a StatusCause whose Message is safe to show to
// end users. API layers forward only this cause's Message; all other error
// text stays internal. Only put user-safe text into it.
const CauseTypePublicMessage metav1.CauseType = "cp.ei.telekom.de/PublicMessage"

// WithPublicMessage adds a CauseTypePublicMessage cause carrying msg to err.
func WithPublicMessage(err *apierrors.StatusError, msg string) *apierrors.StatusError {
	if err.ErrStatus.Details == nil {
		err.ErrStatus.Details = &metav1.StatusDetails{}
	}
	err.ErrStatus.Details.Causes = append(err.ErrStatus.Details.Causes, metav1.StatusCause{Type: CauseTypePublicMessage, Message: msg})
	return err
}

// PublicMessage returns the first non-empty CauseTypePublicMessage message of err.
func PublicMessage(err *apierrors.StatusError) (string, bool) {
	if err == nil || err.ErrStatus.Details == nil {
		return "", false
	}
	for _, cause := range err.ErrStatus.Details.Causes {
		if cause.Type == CauseTypePublicMessage && cause.Message != "" {
			return cause.Message, true
		}
	}
	return "", false
}
