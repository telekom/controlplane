// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

var _ record.EventRecorder = &EventRecorder{}

type EventRecorder struct{}

func (m *EventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
}

func (m *EventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
}

func (m *EventRecorder) PastEventf(object runtime.Object, timestamp, eventtype, reason, messageFmt string, args ...interface{}) {
}

func (m *EventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
}
