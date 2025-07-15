// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"flag"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Configuration key constants
const (
	configKeyFile                = "config"
	configKeyRequeueAfterOnError = "requeue-after-on-error"
	configKeyRequeueAfter        = "requeue-after"
	configKeyDefaultNamespace    = "default-namespace"
	configKeyDefaultEnvironment  = "default-environment"
	configKeyLabelKeyPrefix      = "label-key-prefix"
	configKeyFinalizerSuffix     = "finalizer-suffix"
	configKeyJitterFactor        = "jitter-factor"
	configKeyMaxBackoff          = "max-backoff"
	configKeyMaxConcurrentRec    = "max-concurrent-reconciles"
)

// exposed configuration variables
var (
	// RequeueAfterOnError is the time to wait before retrying a failed operation.
	// This applies for all controller errors.
	RequeueAfterOnError = 1 * time.Second
	// RequeueAfter is the time to wait before retrying a successful operation.
	RequeueAfter       = 30 * time.Minute
	DefaultNamespace   = "default"
	DefaultEnvironment = "default"
	LabelKeyPrefix     = "cp.ei.telekom.de"
	FinalizerSuffix    = "finalizer"
	FinalizerName      = LabelKeyPrefix + "/" + FinalizerSuffix
	// JitterFactor is the factor to apply to the backoff duration.
	JitterFactor = 0.7
	// MaxBackoff is the maximum backoff duration.
	MaxBackoff = 5 * time.Minute
	// MaxConcurrentReconciles is the maximum number of concurrent reconciles.
	MaxConcurrentReconciles = 10
)

func init() {
	registerDefaults()

	if err := registerEnvs(); err != nil {
		panic("failed to bind environment variables: " + err.Error())
	}
	registerFlag()

	// go run ./cmd/main.go --metrics-bind-address=:8080 --jitter-factor=0.9
	// results in "flag provided but not defined: -metrics-bind-address" (so all in main.go are not registered here)
	// if err := Parse(); err != nil {
	// 	panic("failed to parse configuration: " + err.Error())
	// }
}

func registerDefaults() {
	viper.SetDefault(configKeyRequeueAfterOnError, RequeueAfterOnError)
	viper.SetDefault(configKeyRequeueAfter, RequeueAfter)
	viper.SetDefault(configKeyDefaultNamespace, DefaultNamespace)
	viper.SetDefault(configKeyDefaultEnvironment, DefaultEnvironment)
	viper.SetDefault(configKeyLabelKeyPrefix, LabelKeyPrefix)
	viper.SetDefault(configKeyFinalizerSuffix, FinalizerSuffix)
	viper.SetDefault(configKeyJitterFactor, JitterFactor)
	viper.SetDefault(configKeyMaxBackoff, MaxBackoff)
	viper.SetDefault(configKeyMaxConcurrentRec, MaxConcurrentReconciles)
}

func registerFlag() {
	// Config file flag
	flag.String(configKeyFile, "", "Path to the configuration file")

	// Add flags for all configuration parameters
	flag.Duration(configKeyRequeueAfterOnError, RequeueAfterOnError, "Time to wait before retrying a failed operation")
	flag.Duration(configKeyRequeueAfter, RequeueAfter, "Time to wait before retrying a successful operation")
	flag.String(configKeyDefaultNamespace, DefaultNamespace, "Default namespace")
	flag.String(configKeyDefaultEnvironment, DefaultEnvironment, "Default environment")
	flag.String(configKeyLabelKeyPrefix, LabelKeyPrefix, "Label key prefix")
	flag.String(configKeyFinalizerSuffix, FinalizerSuffix, "Finalizer name suffix")
	flag.Float64(configKeyJitterFactor, JitterFactor, "Factor to apply to the backoff duration")
	flag.Duration(configKeyMaxBackoff, MaxBackoff, "Maximum backoff duration")
	flag.Int(configKeyMaxConcurrentRec, MaxConcurrentReconciles, "Maximum number of concurrent reconciles")
}

func registerEnvs() error {
	err := viper.BindEnv(configKeyRequeueAfterOnError, "REQUEUE_AFTER_ON_ERROR")
	if err != nil {
		return err
	}
	err = viper.BindEnv(configKeyRequeueAfter, "REQUEUE_AFTER")
	if err != nil {
		return err
	}
	err = viper.BindEnv(configKeyDefaultNamespace, "DEFAULT_NAMESPACE")
	if err != nil {
		return err
	}
	err = viper.BindEnv(configKeyDefaultEnvironment, "DEFAULT_ENVIRONMENT")
	if err != nil {
		return err
	}
	err = viper.BindEnv(configKeyLabelKeyPrefix, "LABEL_KEY_PREFIX")
	if err != nil {
		return err
	}
	err = viper.BindEnv(configKeyFinalizerSuffix, "FINALIZER_SUFFIX")
	if err != nil {
		return err
	}
	err = viper.BindEnv(configKeyJitterFactor, "JITTER_FACTOR")
	if err != nil {
		return err
	}
	err = viper.BindEnv(configKeyMaxBackoff, "MAX_BACKOFF")
	if err != nil {
		return err
	}
	err = viper.BindEnv(configKeyMaxConcurrentRec, "MAX_CONCURRENT_RECONCILES")
	if err != nil {
		return err
	}
	return nil
}

func Parse() error {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	flag.Parse()
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		return err
	}

	if err := loadConfigFileFromFlag(); err != nil {
		return err
	}

	RequeueAfterOnError = viper.GetDuration(configKeyRequeueAfterOnError)
	RequeueAfter = viper.GetDuration(configKeyRequeueAfter)
	DefaultNamespace = viper.GetString(configKeyDefaultNamespace)
	DefaultEnvironment = viper.GetString(configKeyDefaultEnvironment)
	LabelKeyPrefix = viper.GetString(configKeyLabelKeyPrefix)
	FinalizerSuffix = viper.GetString(configKeyFinalizerSuffix)
	FinalizerName = LabelKeyPrefix + "/" + FinalizerSuffix
	JitterFactor = viper.GetFloat64(configKeyJitterFactor)
	MaxBackoff = viper.GetDuration(configKeyMaxBackoff)
	MaxConcurrentReconciles = viper.GetInt(configKeyMaxConcurrentRec)

	return nil
}

func loadConfigFileFromFlag() error {
	// Get config file path from command line flag
	configPath := viper.GetString(configKeyFile)
	if configPath != "" {
		viper.SetConfigFile(configPath)
	}
	err := viper.ReadInConfig()
	var configFileNotFoundError viper.ConfigFileNotFoundError
	if err != nil && !errors.As(err, &configFileNotFoundError) {
		return err
	}
	return nil
}
