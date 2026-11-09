// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Configuration key constants
const (
	configKeyRequeueAfterOnError = "requeue-after-on-error"
	configKeyRequeueAfter        = "requeue-after"
	configKeyJitterFactor        = "jitter-factor"
	configKeyMaxBackoff          = "max-backoff"
	configKeyMaxConcurrentRec    = "max-concurrent-reconciles"
)

const (
	LabelKeyPrefix     = "cp.ei.telekom.de"
	FinalizerSuffix    = "finalizer"
	FinalizerName      = LabelKeyPrefix + "/" + FinalizerSuffix
	DefaultNamespace   = "default"
	DefaultEnvironment = "default"
)

// exposed configuration variables
var (
	// RequeueAfterOnError is the time to wait before retrying a failed operation.
	// This applies for all controller errors.
	RequeueAfterOnError = 1 * time.Second
	// RequeueAfter is the time to wait before retrying a successful operation.
	RequeueAfter = 30 * time.Minute
	// JitterFactor is the factor to apply to the backoff duration.
	JitterFactor = 0.7
	// MaxBackoff is the maximum backoff duration.
	MaxBackoff = 5 * time.Minute
	// MaxConcurrentReconciles is the maximum number of concurrent reconciles.
	MaxConcurrentReconciles = 10
)

func init() {
	registerDefaults()
	registerEnvs()
	Parse()
	logFeatureStates()
}

func registerDefaults() {
	viper.SetDefault(configKeyRequeueAfterOnError, RequeueAfterOnError)
	viper.SetDefault(configKeyRequeueAfter, RequeueAfter)
	viper.SetDefault(configKeyJitterFactor, JitterFactor)
	viper.SetDefault(configKeyMaxBackoff, MaxBackoff)
	viper.SetDefault(configKeyMaxConcurrentRec, MaxConcurrentReconciles)
}

func registerEnvs() {
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
}

func Parse() {
	RequeueAfterOnError = viper.GetDuration(configKeyRequeueAfterOnError)
	RequeueAfter = viper.GetDuration(configKeyRequeueAfter)

	JitterFactor = viper.GetFloat64(configKeyJitterFactor)
	MaxBackoff = viper.GetDuration(configKeyMaxBackoff)
	MaxConcurrentReconciles = viper.GetInt(configKeyMaxConcurrentRec)

	for f := range features {
		features[f] = viper.GetBool(f.Path())
	}
}
