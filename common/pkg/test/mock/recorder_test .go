// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

var _ record.EventRecorder = &EventRecorder{}

// RecordedEvent stores information about an event for test verification
type RecordedEvent struct {
	Object    runtime.Object
	EventType string
	Reason    string
	Message   string
}

type EventRecorder struct {
	// mutex to protect concurrent access to events
	mu     sync.RWMutex
	events []RecordedEvent
}

func (m *EventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, RecordedEvent{
		Object:    object,
		EventType: eventtype,
		Reason:    reason,
		Message:   message,
	})
}

func (m *EventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	m.Event(object, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

func (m *EventRecorder) PastEventf(object runtime.Object, timestamp, eventtype, reason, messageFmt string, args ...interface{}) {
	// For testing purposes, we ignore the timestamp
	m.Event(object, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

func (m *EventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	// For testing purposes, we ignore the annotations
	m.Event(object, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

// GetEvents returns all recorded events for the given object and event type
func (m *EventRecorder) GetEvent(obj runtime.Object, eventtype string) []RecordedEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []RecordedEvent

	for _, event := range m.events {
		if event.Object == obj && event.EventType == eventtype {
			result = append(result, event)
		}
	}

	return result
}

// GetEvents returns all recorded events for the given object
func (m *EventRecorder) GetEvents(obj runtime.Object) []RecordedEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []RecordedEvent

	for _, event := range m.events {
		if event.Object == obj {
			result = append(result, event)
		}
	}

	return result
}
