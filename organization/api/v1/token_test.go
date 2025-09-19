// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/organization/api/v1"
)

func TestEncodeTeamToken(t *testing.T) {
	RegisterTestingT(t)
	expectedEncodedToken := "env--group--team.eyJjbGllbnRfaWQiOiJjbGllbnRfaWQiLCJjbGllbnRfc2VjcmV0IjoiY2xpZW50X3NlY3JldCIsImVudmlyb25tZW50IjoiZW52IiwiZ2VuZXJhdGVkX2F0IjoxNzQ0MDk4NjEyLCJzZXJ2ZXJfdXJsIjoiaHR0cHM6Ly9leGFtcGxlLmNvbS9zZXJ2ZXIiLCJ0b2tlbl91cmwiOiJodHRwczovL2V4YW1wbGUuY29tL3Rva2VuIn0="
	got, err := v1.EncodeTeamToken(
		v1.TeamToken{
			ClientId:     "client_id",
			ClientSecret: "client_secret",
			Environment:  "env",
			GeneratedAt:  time.Date(2025, 4, 8, 7, 50, 12, 0, time.UTC).Unix(),
			ServerUrl:    "https://example.com/server",
			TokenUrl:     "https://example.com/token",
		}, "group", "team")
	Expect(err).NotTo(HaveOccurred())
	Expect(got).To(Equal(expectedEncodedToken))

	stringToken, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(got, "env--group--team.", ""))
	Expect(err).NotTo(HaveOccurred())
	Expect(string(stringToken)).To(Equal("{\"client_id\":\"client_id\",\"client_secret\":\"client_secret\",\"environment\":\"env\",\"generated_at\":1744098612,\"server_url\":\"https://example.com/server\",\"token_url\":\"https://example.com/token\"}"))

	expectedToken, gotToken := v1.TeamToken{
		ClientId:     "client_id",
		ClientSecret: "client_secret",
		Environment:  "env",
		GeneratedAt:  1744098612,
		ServerUrl:    "https://example.com/server",
		TokenUrl:     "https://example.com/token",
	}, v1.TeamToken{}
	err = json.Unmarshal(stringToken, &gotToken)
	Expect(err).NotTo(HaveOccurred())
	Expect(gotToken).To(Equal(expectedToken))
}

func TestDecodeToken(t *testing.T) {
	RegisterTestingT(t)
	// #nosec G101
	given := "eyJjbGllbnRfaWQiOiJjbGllbnRfaWQiLCJjbGllbnRfc2VjcmV0IjoiY2xpZW50X3NlY3JldCIsImVudmlyb25tZW50IjoiZW52IiwiZ2VuZXJhdGVkX2F0IjoxNzQ0MDk4NjEyLCJzZXJ2ZXJfdXJsIjoiaHR0cHM6Ly9leGFtcGxlLmNvbS9zZXJ2ZXIiLCJ0b2tlbl91cmwiOiJodHRwczovL2V4YW1wbGUuY29tL3Rva2VuIn0="
	decodedToken, err := v1.DecodeTeamToken(given)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("failed to decode token. `env--group--team.` prefix is not present")))

	given = "env--group--team." + given

	decodedToken, err = v1.DecodeTeamToken(given)
	Expect(err).NotTo(HaveOccurred())
	Expect(decodedToken).To(Equal(v1.TeamToken{
		ClientId:     "client_id",
		ClientSecret: "client_secret",
		Environment:  "env",
		GeneratedAt:  time.Date(2025, 4, 8, 7, 50, 12, 0, time.UTC).Unix(),
		ServerUrl:    "https://example.com/server",
		TokenUrl:     "https://example.com/token",
	}))

	_, err = v1.DecodeTeamToken("")
	Expect(err).To(HaveOccurred())
}
