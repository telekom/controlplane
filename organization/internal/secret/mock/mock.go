// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"context"
	"sync"

	"github.com/telekom/controlplane/common/pkg/util/hash"
	"github.com/telekom/controlplane/organization/internal/secret"
	"github.com/telekom/controlplane/secret-manager/api"
	"k8s.io/apimachinery/pkg/util/uuid"
)

// would be nice to replace with https://github.com/vektra/mockery in future

var (
	once                sync.Once
	mockManagerInstance *mockManager
)

func SecretManager() api.SecretManager {
	once.Do(func() {
		mockManagerInstance = &mockManager{
			mockedSecretMap: make(map[string]string),
		}
	})
	return mockManagerInstance
}

type mockManager struct {
	writeMutax      sync.Mutex
	mockedSecretMap map[string]string
}

func (m *mockManager) Get(_ context.Context, secretID string) (value string, err error) {
	return m.mockedSecretMap[secretID], nil
}

func (m *mockManager) Set(_ context.Context, secretID string, secretValue string) (newID string, err error) {
	m.writeMutax.Lock()
	defer m.writeMutax.Unlock()
	delete(m.mockedSecretMap, secretID)
	newID = hash.ComputeHash(secretValue, nil)
	m.mockedSecretMap[newID] = secretValue
	return newID, nil
}

func (m *mockManager) Rotate(ctx context.Context, secretID string) (newID string, err error) {
	return m.Set(ctx, secretID, string(uuid.NewUUID()))
}

func (m *mockManager) UpsertEnvironment(_ context.Context, _ string) (availableSecrets map[string]string, err error) {
	panic("not implemented")
}

func (m *mockManager) UpsertTeam(ctx context.Context, envID, teamID string) (availableSecrets map[string]string, err error) {
	combined := envID + ":" + teamID
	teamTokenId, _ := m.Set(ctx, combined+":"+secret.TeamToken, string(uuid.NewUUID()))
	clientSecretId, _ := m.Set(ctx, combined+":"+secret.ClientSecret, string(uuid.NewUUID()))
	return map[string]string{
		secret.TeamToken:    teamTokenId,
		secret.ClientSecret: clientSecretId,
	}, nil
}

func (m *mockManager) UpsertApplication(_ context.Context, _, _, _ string, opts ...api.OnboardingOption) (availableSecrets map[string]string, err error) {
	panic("not implemented")
}

func (m *mockManager) DeleteEnvironment(_ context.Context, _ string) (err error) {
	panic("not implemented")
}

func (m *mockManager) DeleteTeam(_ context.Context, _, _ string) (err error) {
	m.writeMutax.Lock()
	defer m.writeMutax.Unlock()
	m.mockedSecretMap = make(map[string]string) // Future improvement: delete only the secrets related to the team
	return nil
}

func (m *mockManager) DeleteApplication(_ context.Context, _, _, _ string) (err error) {
	panic("not implemented")
}
