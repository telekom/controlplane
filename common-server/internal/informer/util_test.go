// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer_test

import (
	"context"
	"sync"

	"github.com/telekom/controlplane/common-server/internal/informer"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ informer.EventHandler = &mockEventHandler{}

type ReceivedEvent struct {
	Action string
	Object *unstructured.Unstructured
}

type mockEventHandler struct {
	lock      sync.Mutex
	events    []ReceivedEvent
	NextError error
}

func (m *mockEventHandler) OnCreate(ctx context.Context, obj *unstructured.Unstructured) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.events = append(m.events, ReceivedEvent{Action: "create", Object: obj})
	return m.NextError
}

func (m *mockEventHandler) OnDelete(ctx context.Context, obj *unstructured.Unstructured) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.events = append(m.events, ReceivedEvent{Action: "delete", Object: obj})
	return m.NextError
}

func (m *mockEventHandler) OnUpdate(ctx context.Context, obj *unstructured.Unstructured) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.events = append(m.events, ReceivedEvent{Action: "update", Object: obj})
	return m.NextError
}

func (m *mockEventHandler) Reset() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.events = nil
	m.NextError = nil
}

func (m *mockEventHandler) Events() []ReceivedEvent {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.events
}

func NewUnstructured(name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":      name,
				"namespace": "default",
			},
			"spec": map[string]any{},
		},
	}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "testgroup",
		Version: "v1",
		Kind:    "TestObject",
	})
	return u
}
