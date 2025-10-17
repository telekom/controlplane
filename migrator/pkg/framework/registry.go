// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Registry holds all registered migrators
type Registry struct {
	migrators map[string]ResourceMigrator
}

// NewRegistry creates a new migrator registry
func NewRegistry() *Registry {
	return &Registry{
		migrators: make(map[string]ResourceMigrator),
	}
}

// Register adds a migrator to the registry
func (r *Registry) Register(migrator ResourceMigrator) error {
	name := migrator.GetName()
	if _, exists := r.migrators[name]; exists {
		return fmt.Errorf("migrator %s already registered", name)
	}
	r.migrators[name] = migrator
	return nil
}

// Get retrieves a migrator by name
func (r *Registry) Get(name string) (ResourceMigrator, bool) {
	migrator, exists := r.migrators[name]
	return migrator, exists
}

// SetupAll sets up all registered migrators with the manager
func (r *Registry) SetupAll(
	mgr ctrl.Manager,
	remoteClient client.Client,
	log logr.Logger,
) error {
	for name, migrator := range r.migrators {
		log.Info("Setting up migrator", "name", name)

		reconciler := &GenericMigrationReconciler{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			RemoteClient: remoteClient,
			Migrator:     migrator,
			Log:          log.WithValues("migrator", name),
		}

		if err := reconciler.SetupWithManager(mgr); err != nil {
			return fmt.Errorf("failed to setup migrator %s: %w", name, err)
		}

		log.Info("Successfully setup migrator", "name", name)
	}

	return nil
}

// List returns all registered migrator names
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.migrators))
	for name := range r.migrators {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered migrators
func (r *Registry) Count() int {
	return len(r.migrators)
}
