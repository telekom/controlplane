// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ types.StatusContainer = (*SecretRotationStatusResponse)(nil)

// SecretRotationStatusResponse represents the response from the secret rotation status endpoint.
type SecretRotationStatusResponse struct {
	Gone               bool   `json:"-" yaml:"-"`
	ClientId           string `json:"clientId" yaml:"clientId"`
	ProcessingState    string `json:"processingState" yaml:"processingState"`
	OverallStatus      string `json:"overallStatus" yaml:"overallStatus"`
	CurrentSecretValue string `json:"currentSecretValue,omitempty" yaml:"currentSecretValue,omitempty"`
	RotatedSecretValue string `json:"rotatedSecretValue,omitempty" yaml:"rotatedSecretValue,omitempty"`
	RotatedExpiresAt   string `json:"rotatedExpiresAt,omitempty" yaml:"rotatedExpiresAt,omitempty"`
	CurrentExpiresAt   string `json:"currentExpiresAt,omitempty" yaml:"currentExpiresAt,omitempty"`
}

// GetOverallStatus implements [StatusContainer].
func (s *SecretRotationStatusResponse) GetOverallStatus() types.OverallStatus {
	return types.OverallStatus(s.OverallStatus)
}

// GetProcessingState implements [StatusContainer].
func (s *SecretRotationStatusResponse) GetProcessingState() types.ProcessingState {
	return types.ProcessingState(s.ProcessingState)
}

// IsGone implements [StatusContainer].
func (s *SecretRotationStatusResponse) IsGone() bool {
	return s.Gone
}

// GetSecretRotationStatus fetches the current secret rotation status for an application.
func (h *RoverHandler) GetSecretRotationStatus(ctx context.Context, name string) (*SecretRotationStatusResponse, error) {
	token := h.Setup(ctx)
	url := h.GetRequestUrl(token.Group, token.Team, name, "secret", "status")

	resp, err := h.SendRequest(ctx, common.NoBody, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = common.CheckResponseCode(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}

	var response SecretRotationStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, errors.Wrap(err, "failed to parse secret rotation status response")
	}

	return &response, nil
}

// WaitForSecretConvergence polls the secret rotation status endpoint until the rotation has converged.
func (h *RoverHandler) WaitForSecretConvergence(ctx context.Context, name string) (*SecretRotationStatusResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, viper.GetDuration("timeout.secretRotation"))
	defer cancel()

	ticker := time.NewTicker(viper.GetDuration("poll.interval"))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("timed out waiting for secret rotation to converge")
		case <-ticker.C:
			status, err := h.GetSecretRotationStatus(ctx, name)
			if err != nil {
				return nil, err
			}
			if status.OverallStatus == "complete" {
				return status, nil
			}
		}
	}
}

// ResetSecret triggers a secret rotation and waits until the new secret has converged.
func (h *RoverHandler) ResetSecret(ctx context.Context, name string) (*SecretRotationStatusResponse, error) {
	token := h.Setup(ctx)
	url := h.GetRequestUrl(token.Group, token.Team, name, "secret")

	resp, err := h.SendRequest(ctx, common.NoBody, http.MethodPatch, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = common.CheckResponseCode(resp, http.StatusOK, http.StatusAccepted)
	if err != nil {
		return nil, err
	}

	return h.WaitForSecretConvergence(ctx, name)
}
