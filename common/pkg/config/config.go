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
	configKeyDefaultNamespace    = "default-namespace"
	configKeyDefaultEnvironment  = "default-environment"
	configKeyLabelKeyPrefix      = "label-key-prefix"
	configKeyJitterFactor        = "jitter-factor"
	configKeyMaxBackoff          = "max-backoff"
	configKeyMaxConcurrentRec    = "max-concurrent-reconciles"
)

const (
	FinalizerSuffix = "finalizer"
)

type Feature string

func (f Feature) String() string {
	return string(f)
}

func (f Feature) IsEnabled() bool {
	return Features[f]
}

func (f Feature) Path() string {
	return "feature-" + f.String() + "-enabled"
}

var (
	Features             map[Feature]bool
	FeaturePubSub        Feature = "pubsub"
	FeatureSecretManager Feature = "secret_manager"
	FeatureFileManager   Feature = "file_manager"
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

	DefaultNamespace   = "default"
	DefaultEnvironment = "default"
	LabelKeyPrefix     = "cp.ei.telekom.de"
	FinalizerName      = LabelKeyPrefix + "/" + FinalizerSuffix
)

func init() {
	registerDefaults()
	registerEnvs()
	Parse()
}

func registerDefaults() {
	viper.SetDefault(configKeyRequeueAfterOnError, RequeueAfterOnError)
	viper.SetDefault(configKeyRequeueAfter, RequeueAfter)
	viper.SetDefault(configKeyDefaultNamespace, DefaultNamespace)
	viper.SetDefault(configKeyDefaultEnvironment, DefaultEnvironment)
	viper.SetDefault(configKeyLabelKeyPrefix, LabelKeyPrefix)
	viper.SetDefault(configKeyJitterFactor, JitterFactor)
	viper.SetDefault(configKeyMaxBackoff, MaxBackoff)
	viper.SetDefault(configKeyMaxConcurrentRec, MaxConcurrentReconciles)
	viper.SetDefault(FeatureSecretManager.Path(), true) // Secret Manager feature enabled by default
	viper.SetDefault(FeatureFileManager.Path(), true)   // File Manager feature enabled by default
}

func registerEnvs() {
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
}

func Parse() {
	RequeueAfterOnError = viper.GetDuration(configKeyRequeueAfterOnError)
	RequeueAfter = viper.GetDuration(configKeyRequeueAfter)
	DefaultNamespace = viper.GetString(configKeyDefaultNamespace)
	DefaultEnvironment = viper.GetString(configKeyDefaultEnvironment)

	JitterFactor = viper.GetFloat64(configKeyJitterFactor)
	MaxBackoff = viper.GetDuration(configKeyMaxBackoff)
	MaxConcurrentReconciles = viper.GetInt(configKeyMaxConcurrentRec)
	LabelKeyPrefix = viper.GetString(configKeyLabelKeyPrefix)

	FinalizerName = LabelKeyPrefix + "/" + FinalizerSuffix
	Features = map[Feature]bool{
		FeaturePubSub:        viper.GetBool(FeaturePubSub.Path()),        // FEATURE_PUBSUB_ENABLED
		FeatureSecretManager: viper.GetBool(FeatureSecretManager.Path()), // FEATURE_SECRET_MANAGER_ENABLED
	}
}
