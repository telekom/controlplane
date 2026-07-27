// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur_test

import (
	"context"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/cyberark/conjur-api-go/conjurapi"
	conjurapi_response "github.com/cyberark/conjur-api-go/conjurapi/response"
	. "github.com/onsi/ginkgo/v2"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur/bouncer"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/onboardertest"
)

// inMemoryConjur is a minimal in-process ConjurAPI backed by a map. It lets the
// shared contract suite observe onboarding results through Backend.Get, exactly
// like the Kubernetes fake client does, instead of asserting on mock calls.
//
// LoadPolicy is a no-op for creation (PolicyModePost) and, for deletion
// (PolicyModePatch), cascade-removes every variable stored under the deleted
// scope — emulating how Conjur removes variables when their policy is deleted.
type inMemoryConjur struct {
	mu    sync.Mutex
	store map[string]string
}

var deleteRecordRe = regexp.MustCompile(`record: !policy (\S+)`)

func newInMemoryConjur() *inMemoryConjur {
	return &inMemoryConjur{store: make(map[string]string)}
}

func (m *inMemoryConjur) RetrieveSecret(variableID string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.store[variableID]
	if !ok || v == "" {
		return nil, &conjurapi_response.ConjurError{Code: 404, Message: "Not Found"}
	}
	return []byte(v), nil
}

func (m *inMemoryConjur) AddSecret(variableID, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[variableID] = value
	return nil
}

func (m *inMemoryConjur) LoadPolicy(mode conjurapi.PolicyMode, path string, r io.Reader) (*conjurapi.PolicyResponse, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if mode == conjurapi.PolicyModePatch {
		if match := deleteRecordRe.FindSubmatch(buf); match != nil {
			prefix := strings.TrimRight(path, "/") + "/" + string(match[1])
			m.mu.Lock()
			for k := range m.store {
				if k == prefix || strings.HasPrefix(k, prefix+"/") {
					delete(m.store, k)
				}
			}
			m.mu.Unlock()
		}
	}
	return nil, nil
}

func (m *inMemoryConjur) RetrieveBatchSecrets(variableIDs []string) (map[string][]byte, error) {
	res := make(map[string][]byte, len(variableIDs))
	for _, id := range variableIDs {
		v, err := m.RetrieveSecret(id)
		if err != nil {
			return nil, err
		}
		res[id] = v
	}
	return res, nil
}

func (m *inMemoryConjur) RetrieveSecretWithVersion(variableID string, _ int) ([]byte, error) {
	return m.RetrieveSecret(variableID)
}

// conjurHarness adapts the Conjur onboarder to the shared contract suite. The
// in-memory API is the shared store: the onboarder's secretWriter and the read
// path both use the same real ConjurBackend so Get reflects what was written. A
// shared locker mirrors production so concurrent onboarding is serialized.
type conjurHarness struct {
	be        *conjur.ConjurBackend
	onboarder *conjur.ConjurOnboarder
}

func newConjurHarness() onboardertest.Harness {
	api := newInMemoryConjur()
	locker := bouncer.NewLocker("contract")
	be := conjur.NewBackend(api, api).WithBouncer(locker)
	ob := conjur.NewOnboarder(api, be).WithBouncer(locker)
	return &conjurHarness{be: be, onboarder: ob}
}

func (h *conjurHarness) Onboarder() backend.Onboarder {
	return h.onboarder
}

func (h *conjurHarness) Get(ctx context.Context, ref string) (string, error) {
	id, err := conjur.FromString(ref)
	if err != nil {
		return "", err
	}
	secret, err := h.be.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return secret.Value(), nil
}

var _ = Describe("Conjur Onboarder Contract", func() {
	onboardertest.RunContractSpecs(newConjurHarness)
})
