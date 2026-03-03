// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import "sync"

// EmptySpec is a placeholder spec type for controllers that do not need any
// component-specific configuration.
type EmptySpec struct{}

var once sync.Once

var commonConfig CommonConfig

func GetCommonConfig() ControllerConfig {
	if commonConfig == nil {
		once.Do(func() {
			cfg, err := Load[EmptySpec]()
			if err != nil {
				c := defaultAppConfig[EmptySpec]()
				c.ComputeValues()
				cfg = &c
			}
			commonConfig = cfg
		})
	}
	return commonConfig.CommonConfig()
}

func setCommonConfig(cfg CommonConfig) {
	commonConfig = cfg
}
