// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer_test

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/internal/informer"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	dynamic "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
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
	gate      chan struct{}
	failOnce  string
}

// FailOnce makes the next call of action fail.
func (m *mockEventHandler) FailOnce(action string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.failOnce = action
}

// Block makes the handler wait in every call until Unblock is called.
func (m *mockEventHandler) Block() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.gate = make(chan struct{})
}

func (m *mockEventHandler) Unblock() {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.gate != nil {
		close(m.gate)
		m.gate = nil
	}
}

func (m *mockEventHandler) wait() {
	m.lock.Lock()
	gate := m.gate
	m.lock.Unlock()
	if gate != nil {
		<-gate
	}
}

func (m *mockEventHandler) OnCreate(ctx context.Context, obj *unstructured.Unstructured) error {
	m.wait()
	m.lock.Lock()
	defer m.lock.Unlock()
	m.events = append(m.events, ReceivedEvent{Action: "create", Object: obj})
	return m.err("create")
}

func (m *mockEventHandler) OnDelete(ctx context.Context, obj *unstructured.Unstructured) error {
	m.wait()
	m.lock.Lock()
	defer m.lock.Unlock()
	m.events = append(m.events, ReceivedEvent{Action: "delete", Object: obj})
	return m.err("delete")
}

func (m *mockEventHandler) OnUpdate(ctx context.Context, obj *unstructured.Unstructured) error {
	m.wait()
	m.lock.Lock()
	defer m.lock.Unlock()
	m.events = append(m.events, ReceivedEvent{Action: "update", Object: obj})
	return m.err("update")
}

func (m *mockEventHandler) err(action string) error {
	if m.failOnce == action {
		m.failOnce = ""
		return errors.New("FAKE_ERROR")
	}
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

// HaveEvent matches a ReceivedEvent with the given action and object name.
func HaveEvent(action, name string) gomegatypes.GomegaMatcher {
	return SatisfyAll(
		HaveField("Action", action),
		HaveField("Object.Object", HaveKeyWithValue("metadata", HaveKeyWithValue("name", name))),
	)
}

// CountEvents returns the number of events with the given action and object name.
func CountEvents(events []ReceivedEvent, action, name string) int {
	n := 0
	for _, e := range events {
		if e.Action == action && e.Object.GetName() == name {
			n++
		}
	}
	return n
}

// fakeWatches makes every watch call return a new FakeWatcher. It returns the
// latest watcher and the number of watch calls.
func fakeWatches(client *dynamic.FakeDynamicClient) (*atomic.Pointer[watch.FakeWatcher], *atomic.Int32) {
	var current atomic.Pointer[watch.FakeWatcher]
	var calls atomic.Int32
	client.PrependWatchReactor("*", func(k8stesting.Action) (bool, watch.Interface, error) {
		w := watch.NewFake()
		current.Store(w)
		calls.Add(1)
		return true, w, nil
	})
	return &current, &calls
}

// pageFunc returns the object names of the page requested with continueToken,
// the continue token of the next page, or an error.
type pageFunc func(continueToken string) (names []string, next string, err error)

// pagedLists serves list calls from pages with resourceVersion "10". It
// returns the number of list calls. The fake client drops the continue token
// from list actions, so the token of the previous successful page is used;
// after an error the list starts again from the first page.
func pagedLists(client *dynamic.FakeDynamicClient, pages pageFunc) *atomic.Int32 {
	var calls atomic.Int32
	var continueToken string
	client.PrependReactor("list", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		calls.Add(1)
		names, next, err := pages(continueToken)
		if err != nil {
			continueToken = ""
			return true, nil, err
		}
		continueToken = next
		list := &unstructured.UnstructuredList{Object: map[string]any{"apiVersion": "testgroup/v1", "kind": "TestObjectList"}}
		list.SetResourceVersion("10")
		list.SetContinue(next)
		for _, name := range names {
			list.Items = append(list.Items, *NewUnstructured(name))
		}
		return true, list, nil
	})
	return &calls
}

// watchResourceVersions records the resource version of every watch call. It
// must be registered after the reactor that serves the watch.
func watchResourceVersions(client *dynamic.FakeDynamicClient) func() []string {
	var lock sync.Mutex
	var rvs []string
	client.PrependWatchReactor("*", func(action k8stesting.Action) (bool, watch.Interface, error) {
		lock.Lock()
		defer lock.Unlock()
		rvs = append(rvs, action.(k8stesting.WatchActionImpl).WatchRestrictions.ResourceVersion)
		return false, nil, nil
	})
	return func() []string {
		lock.Lock()
		defer lock.Unlock()
		return slices.Clone(rvs)
	}
}

func NewUnstructuredWithResourceVersion(name, rv string) *unstructured.Unstructured {
	u := NewUnstructured(name)
	u.SetResourceVersion(rv)
	return u
}
