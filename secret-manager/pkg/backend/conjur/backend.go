// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

import (
	"context"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur/bouncer"
	"github.com/telekom/controlplane/secret-manager/pkg/tracing"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ChecksumMode controls how the backend handles checksum mismatches on Get.
type ChecksumMode string

const (
	// ChecksumModeDisabled returns the secret silently on mismatch (default, backward-compatible).
	ChecksumModeDisabled ChecksumMode = "disabled"
	// ChecksumModeObserver returns the secret but logs the mismatch and increments a metric.
	ChecksumModeObserver ChecksumMode = "observer"
	// ChecksumModeStrict returns ErrBadChecksum on mismatch.
	ChecksumModeStrict ChecksumMode = "strict"
)

// ParseChecksumMode parses a string into a ChecksumMode, defaulting to Disabled on unknown input.
func ParseChecksumMode(s string) ChecksumMode {
	switch ChecksumMode(strings.ToLower(s)) {
	case ChecksumModeObserver:
		return ChecksumModeObserver
	case ChecksumModeStrict:
		return ChecksumModeStrict
	default:
		return ChecksumModeDisabled
	}
}

var (
	checksumMetricsOnce   sync.Once
	checksumMismatchTotal *prometheus.CounterVec
)

// RegisterChecksumMetrics registers the checksum mismatch counter with the given registerer.
func RegisterChecksumMetrics(reg prometheus.Registerer) {
	checksumMetricsOnce.Do(func() {
		checksumMismatchTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "secret_get_checksum_mismatch_total",
				Help: "Total number of GET requests where the requested checksum did not match the actual secret checksum",
			},
			[]string{"env", "mode"},
		)
		reg.MustRegister(checksumMismatchTotal)
	})
}

func recordChecksumMismatch(env string, mode ChecksumMode) {
	if checksumMismatchTotal != nil {
		checksumMismatchTotal.WithLabelValues(env, string(mode)).Inc()
	}
}

var _ backend.Backend[ConjurSecretId, backend.DefaultSecret[ConjurSecretId]] = &ConjurBackend{}

type ConjurBackend struct {
	writeAPI ConjurAPI
	readAPI  ConjurAPI

	// ChecksumMode controls how checksum mismatches are handled on Get.
	// See ChecksumModeDisabled, ChecksumModeObserver, ChecksumModeStrict.
	ChecksumMode ChecksumMode

	// bouncer provides per-variable locking for Set operations to prevent
	// concurrent read-modify-write races on the same secret.
	bouncer bouncer.Bouncer
}

func NewBackend(writeAPI, readAPI ConjurAPI) *ConjurBackend {
	return &ConjurBackend{
		writeAPI:     writeAPI,
		readAPI:      readAPI,
		ChecksumMode: ChecksumModeDisabled,
	}
}

func (c *ConjurBackend) WithBouncer(b bouncer.Bouncer) *ConjurBackend {
	if b == nil {
		return c
	}
	c.bouncer = b
	return c
}

func (c *ConjurBackend) ParseSecretId(rawId string) (ConjurSecretId, error) {
	return FromString(rawId)
}

func (c *ConjurBackend) Get(ctx context.Context, id ConjurSecretId) (res backend.DefaultSecret[ConjurSecretId], err error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Getting secret", "variableID", id.VariableId())
	if tracing.Enabled() {
		log.V(1).Info("trace conjur get start", "traceId", tracing.TraceID(ctx), "id", id.String(), "variableID", id.VariableId(), "subPath", id.SubPath())
	}
	secret, err := c.readAPI.RetrieveSecret(id.VariableId())
	if err != nil {
		return res, handleError(err, id)
	}

	subPath := id.SubPath()
	if subPath != backend.NoSubPath {
		log.Info("Subpath found. Using subpath to get secret", "subPath", subPath)
		result := gjson.GetBytes(secret, subPath)
		if !result.Exists() {
			return res, backend.ErrSecretNotFound(id)
		}
		newId := id.CopyWithChecksum(backend.MakeChecksum(result.String()))
		res = backend.NewDefaultSecret(newId, result.String())
	} else {
		newId := id.CopyWithChecksum(backend.MakeChecksum(string(secret)))
		res = backend.NewDefaultSecret(newId, string(secret))
	}
	if tracing.Enabled() {
		log.V(1).Info("trace conjur get result", "traceId", tracing.TraceID(ctx), "requestedId", id.String(), "returnedId", res.Id().String(), "valueChecksum", backend.MakeChecksum(res.Value()))
	}

	if id.checksum != backend.NoChecksum && id.checksum != res.Id().checksum {
		switch c.ChecksumMode {
		case ChecksumModeStrict:
			log.Info("Checksum mismatch. Returning error", "id", id.String())
			recordChecksumMismatch(id.Env(), c.ChecksumMode)
			return res, backend.ErrBadChecksum(id)

		case ChecksumModeObserver:
			if tracing.Enabled() {
				log.V(1).Info("trace stale-checksum-get",
					"traceId", tracing.TraceID(ctx),
					"id", id.String(),
					"requestedChecksum", id.checksum,
					"actualChecksum", res.Id().checksum,
				)
			}
			log.Info("Checksum mismatch observed. Returning secret", "id", id.String())
			recordChecksumMismatch(id.Env(), c.ChecksumMode)
			return res, nil

		default: // ChecksumModeDisabled
			return res, nil
		}
	}

	return res, nil
}

func (c *ConjurBackend) Set(ctx context.Context, id ConjurSecretId, secretValue backend.SecretValue, opts ...backend.WriteOption) (res backend.DefaultSecret[ConjurSecretId], err error) {
	if c.bouncer != nil {
		lockKey := id.VariableId()
		bouncerErr := c.bouncer.RunB(ctx, lockKey, func(ctx context.Context) error {
			res, err = c.doSet(ctx, id, secretValue, opts...)
			return err
		})
		if bouncerErr != nil && errors.Is(bouncerErr, bouncer.ErrLockNotAcquired) {
			return res, backend.NewBackendError(nil, bouncerErr, backend.TypeErrTooManyRequests)
		}
		return res, err
	}
	return c.doSet(ctx, id, secretValue, opts...)
}

func (c *ConjurBackend) doSet(ctx context.Context, id ConjurSecretId, secretValue backend.SecretValue, opts ...backend.WriteOption) (res backend.DefaultSecret[ConjurSecretId], err error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Setting secret", "id", id.VariableId())
	if tracing.Enabled() {
		log.V(1).Info("trace conjur set start", "traceId", tracing.TraceID(ctx), "id", id.String(), "variableID", id.VariableId(), "subPath", id.SubPath(), "inputChecksum", backend.MakeChecksum(secretValue.Value()), "allowChange", secretValue.AllowChange())
	}

	options := backend.WriteOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	data, err := c.readAPI.RetrieveSecret(id.VariableId())
	if err != nil {
		cErr, ok := AsError(err)
		if ok && cErr.Code == 404 {
			return c.initialCreation(ctx, id, secretValue)
		}
		return res, handleError(err, id)
	}

	subPath := id.SubPath()
	currentValue := string(data)
	if subPath != backend.NoSubPath {
		result := gjson.GetBytes(data, subPath)
		if !result.Exists() {
			currentValue = backend.NoValue
		} else {
			currentValue = result.String()
		}
	}

	if currentValue != backend.NoValue && !secretValue.AllowChange() {
		log.Info("Secret already exists but is not empty. Not updating...", "id", id.String())
		if tracing.Enabled() {
			log.V(1).Info("trace conjur set no-change (immutable)", "traceId", tracing.TraceID(ctx), "id", id.String(), "currentChecksum", backend.MakeChecksum(currentValue))
		}
		return backend.NewDefaultSecret(id.CopyWithChecksum(backend.MakeChecksum(currentValue)), currentValue), nil
	}

	// For merge strategy, shallow-merge JSON objects with the existing value
	effectiveValue := secretValue.Value()
	if options.Strategy == backend.StrategyMerge && currentValue != backend.NoValue {
		merged, wasMerged := backend.ShallowMergeJSON(currentValue, effectiveValue)
		if wasMerged {
			log.Info("Merge strategy: shallow-merged JSON values", "id", id.String())
			if tracing.Enabled() {
				log.V(1).Info("trace conjur set merge", "traceId", tracing.TraceID(ctx), "id", id.String(), "currentChecksum", backend.MakeChecksum(currentValue), "inputChecksum", backend.MakeChecksum(secretValue.Value()), "effectiveChecksum", backend.MakeChecksum(merged))
			}
			effectiveValue = merged
		}
	}

	if effectiveValue == currentValue {
		log.Info("Secret already exists and is up to date", "id", id.String())
		if tracing.Enabled() {
			log.V(1).Info("trace conjur set no-op", "traceId", tracing.TraceID(ctx), "id", id.String(), "effectiveChecksum", backend.MakeChecksum(effectiveValue))
		}
		return backend.NewDefaultSecret(id.CopyWithChecksum(backend.MakeChecksum(effectiveValue)), currentValue), nil
	}

	nextValue := effectiveValue
	if subPath != backend.NoSubPath {
		log.Info("Subpath found. Using subpath to set secret", "subPath", subPath)
		newData, err := sjson.SetBytes(data, subPath, nextValue)
		if err != nil {
			return res, handleError(err, id)
		}
		nextValue = string(newData)
	}

	log.Info("Secret already exists but is not up to date. Updating...", "id", id.String())
	err = c.writeAPI.AddSecret(id.VariableId(), nextValue)
	if err != nil {
		return res, handleError(err, id)
	}
	newId := id.CopyWithChecksum(backend.MakeChecksum(effectiveValue))
	if tracing.Enabled() {
		log.V(1).Info("trace conjur set updated", "traceId", tracing.TraceID(ctx), "requestedId", id.String(), "returnedId", newId.String(), "effectiveChecksum", backend.MakeChecksum(effectiveValue), "persistedChecksum", backend.MakeChecksum(nextValue))
	}
	return backend.NewDefaultSecret(newId, backend.NoValue), nil
}

func (c *ConjurBackend) Delete(ctx context.Context, id ConjurSecretId) error {
	err := c.writeAPI.AddSecret(id.VariableId(), "")
	if err != nil {
		return handleError(err, id)
	}

	return nil
}

func handleError(err error, id ConjurSecretId) error {
	if backend.IsBackendError(err) {
		return err
	}
	cErr, ok := AsError(err)
	if ok {
		if cErr.Code == 404 {
			return backend.ErrSecretNotFound(id)
		}
	}
	return backend.NewBackendError(id, err, "InternalError")
}

func (c *ConjurBackend) initialCreation(ctx context.Context, id ConjurSecretId, value backend.SecretValue) (res backend.DefaultSecret[ConjurSecretId], err error) {
	log := logr.FromContextOrDiscard(ctx)
	if tracing.Enabled() {
		log.V(1).Info("trace conjur initial creation start", "traceId", tracing.TraceID(ctx), "id", id.String(), "variableID", id.VariableId(), "subPath", id.SubPath(), "inputChecksum", backend.MakeChecksum(value.Value()))
	}

	log.Info("Secret does not exist yet. Initial creation...", "id", id.VariableId())
	subPath := id.SubPath()
	if subPath == backend.NoSubPath {
		err = c.writeAPI.AddSecret(id.VariableId(), value.Value())
		if err != nil {
			return res, handleError(err, id)
		}
		newId := id.CopyWithChecksum(backend.MakeChecksum(value.Value()))
		log.Info("Successfully created new secret", "id", newId.String())
		if tracing.Enabled() {
			log.V(1).Info("trace conjur initial creation end", "traceId", tracing.TraceID(ctx), "requestedId", id.String(), "returnedId", newId.String())
		}
		res = backend.NewDefaultSecret(newId, backend.NoValue)
	} else {
		log.Info("Subpath found. Using subpath to create secret", "subPath", subPath)
		data, err := c.readAPI.RetrieveSecret(id.VariableId())
		if err != nil {
			return res, handleError(err, id)
		}
		newData, err := sjson.SetBytes(data, subPath, value.Value())
		if err != nil {
			return res, handleError(err, id)
		}
		err = c.writeAPI.AddSecret(id.VariableId(), string(newData))
		if err != nil {
			return res, handleError(err, id)
		}
		newId := id.CopyWithChecksum(backend.MakeChecksum(value.Value()))
		log.Info("Successfully created new secret", "id", newId.String())
		if tracing.Enabled() {
			log.V(1).Info("trace conjur initial creation end", "traceId", tracing.TraceID(ctx), "requestedId", id.String(), "returnedId", newId.String())
		}
		res = backend.NewDefaultSecret(newId, backend.NoValue)
	}

	return res, err
}
