// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package config provides a small generic viper wrapper for loading
// configuration from an optional YAML file overlaid with environment variables.
package config

import (
	"fmt"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/telekom/controlplane/common-server/pkg/server"
)

// Load reads config from an optional YAML file, overlays environment variables,
// and unmarshals into *T. defaults is a *T already populated with default
// values; Load returns it mutated. Precedence: env var > config file > defaults.
//
// Env keys map from nested config keys by replacing "." with "_", uppercased
// (e.g. tls.cert -> TLS_CERT, listeners.external.jwt.mode ->
// LISTENERS_EXTERNAL_JWT_MODE). Every scalar leaf present in the defaults tree
// is overridable this way, including nested listener fields. Slice-valued keys
// (e.g. trustedIssuers) accept a comma-separated env value:
// LISTENERS_EXTERNAL_JWT_TRUSTEDISSUERS=https://a,https://b.
func Load[T any](path string, defaults *T) (*T, error) {
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := seedDefaults(v, defaults); err != nil {
		return nil, err
	}

	if path != "" {
		v.SetConfigFile(path)
		if err := v.MergeInConfig(); err != nil {
			return nil, fmt.Errorf("reading config %q: %w", path, err)
		}
	}

	// viper's AutomaticEnv makes env values visible to Get(key) but NOT to
	// Unmarshal, which only walks AllSettings(). BindEnv registers each leaf so
	// Unmarshal picks it up. Bind after seeding so we know every key path.
	for _, key := range v.AllKeys() {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("binding env for %q: %w", key, err)
		}
	}

	// StringToSlice hook lets a comma-separated env var populate a []string
	// field (env values always arrive as a single string).
	decodeHook := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	))
	if err := v.Unmarshal(defaults, decodeHook); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}
	return defaults, nil
}

// seedDefaults registers every default key with viper by decoding defaults into
// a map and merging it in. This is required because AutomaticEnv/BindEnv only
// affect keys viper already knows about.
//
// It decodes via mapstructure (not yaml) so the seeded key paths match the
// `mapstructure` tags viper's Unmarshal decodes back into — including
// `,squash`ed embedded structs (BaseConfig), whose fields must appear at the
// parent level, not under a "baseconfig" key.
func seedDefaults(v *viper.Viper, defaults any) error {
	tree := map[string]any{}
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &tree,
		Squash:  true,
		TagName: "mapstructure",
	})
	if err != nil {
		return fmt.Errorf("building defaults decoder: %w", err)
	}
	if err := dec.Decode(defaults); err != nil {
		return fmt.Errorf("encoding defaults: %w", err)
	}
	if err := v.MergeConfigMap(tree); err != nil {
		return fmt.Errorf("seeding defaults: %w", err)
	}
	return nil
}

// ListenersConfig declares the (at most two) listeners a server runs, keyed by
// role. Internal is the in-cluster trust zone (typically K8s auth); External
// faces outside (typically JWT). Either may be omitted.
type ListenersConfig struct {
	Internal *server.ListenerConfig `mapstructure:"internal,omitempty"`
	External *server.ListenerConfig `mapstructure:"external,omitempty"`
}

// Validate fails closed on misconfiguration that would otherwise only surface
// as a panic at server construction. It requires at least one listener and
// validates each present one with its role (internal listeners may omit a k8s
// accessConfig; external ones may not). Call it once right after Load.
func (l ListenersConfig) Validate() error {
	if l.Internal == nil && l.External == nil {
		return fmt.Errorf("at least one listener (internal or external) must be configured")
	}
	if l.Internal != nil {
		if err := l.Internal.Validate(true); err != nil {
			return fmt.Errorf("internal listener: %w", err)
		}
	}
	if l.External != nil {
		if err := l.External.Validate(false); err != nil {
			return fmt.Errorf("external listener: %w", err)
		}
	}
	return nil
}

// BaseConfig is the shared config spine every server embeds via
// `mapstructure:",squash"`.
type BaseConfig struct {
	Log       LogConfig             `mapstructure:"log"`
	TLS       *server.TLSFileConfig `mapstructure:"tls,omitempty"`
	Listeners ListenersConfig       `mapstructure:"listeners,omitempty"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

// LoadOrDie panics on error
func LoadOrDie[T any](path string, defaults *T) *T {
	cfg, err := Load(path, defaults)
	if err != nil {
		panic(err)
	}
	return cfg
}
