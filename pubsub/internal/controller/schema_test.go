// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/controller"
)

func TestRegisterSchemesOrDie(t *testing.T) {
	scheme := runtime.NewScheme()

	// Should not panic
	controller.RegisterSchemesOrDie(scheme)

	// Verify pubsub types are registered
	for _, obj := range []runtime.Object{
		&pubsubv1.EventStore{},
		&pubsubv1.Publisher{},
		&pubsubv1.Subscriber{},
	} {
		gvks, _, err := scheme.ObjectKinds(obj)
		if err != nil {
			t.Fatalf("expected type %T to be registered, got error: %v", obj, err)
		}
		if len(gvks) == 0 {
			t.Fatalf("expected type %T to have at least one GVK registered", obj)
		}
	}
}
