// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package security

// JWTConfig is the shared, declarative config for the JWT security family.
type JWTConfig struct {
	Mode           Mode      `mapstructure:"mode"`
	TrustedIssuers []string  `mapstructure:"trustedIssuers"`
	DefaultScope   string    `mapstructure:"defaultScope"`
	ScopePrefix    string    `mapstructure:"scopePrefix"`
	LMS            LMSConfig `mapstructure:"lms"`
}

type LMSConfig struct {
	BasePath string `mapstructure:"basePath"`
}

// ToSecurityOpts builds SecurityOpts from the declarative config.
//
// It sets Mode and the JWT/BusinessContext option builders. CheckAccessOpts and
// templates are intentionally NOT set here — those are server-specific and must
// be supplied by the caller at the ConfigureSecurity call site.
func (c JWTConfig) ToSecurityOpts() SecurityOpts {
	return SecurityOpts{
		Mode: c.Mode,
		JWTOpts: []Option[*JWTOpts]{
			WithTrustedIssuers(c.TrustedIssuers),
			WithLmsCheck(c.LMS.BasePath),
		},
		BusinessContextOpts: []Option[*BusinessContextOpts]{
			WithDefaultScope(c.DefaultScope),
			WithScopePrefix(c.ScopePrefix),
		},
	}
}
