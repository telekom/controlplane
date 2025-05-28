// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identity_client

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildIdentityClientObj(t *testing.T) {
	RegisterTestingT(t)
	team := &organizationv1.Team{
		Spec: organizationv1.TeamSpec{
			Name:  "team",
			Group: "group",
		},
		Status: organizationv1.TeamStatus{
			Namespace: "env--group--team",
		},
	}

	expected := &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "group--team--team-user",
			Namespace: "env--group--team",
		},
	}
	got := buildIdentityClientObj(team)
	Expect(got).To(Equal(expected))
}

func TestBuildToken(t *testing.T) {
	RegisterTestingT(t)
	expectedBase64 := "eyJjbGllbnRfaWQiOiJjbGllbnRfaWQiLCJjbGllbnRfc2VjcmV0IjoiY2xpZW50X3NlY3JldCIsImVudmlyb25tZW50IjoiZW52IiwiZ2VuZXJhdGVkX2F0IjoxNzQ0MDk4NjEyfQ=="
	gotBase64, err := buildToken("client_id", "client_secret", "env", time.Date(2025, 4, 8, 7, 50, 12, 0, time.UTC))
	Expect(err).NotTo(HaveOccurred())
	Expect(gotBase64).To(Equal(expectedBase64))

	stringToken, err := base64.StdEncoding.DecodeString(gotBase64)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(stringToken)).To(Equal("{\"client_id\":\"client_id\",\"client_secret\":\"client_secret\",\"environment\":\"env\",\"generated_at\":1744098612}"))

	expectedToken, gotToken := token{
		ClientId:     "client_id",
		ClientSecret: "client_secret",
		Environment:  "env",
		GeneratedAt:  1744098612,
	}, token{}
	err = json.Unmarshal(stringToken, &gotToken)
	Expect(err).NotTo(HaveOccurred())
	Expect(gotToken).To(Equal(expectedToken))
}

func TestDecodeToken(t *testing.T) {
	RegisterTestingT(t)
	// #nosec G101
	given := "eyJjbGllbnRfaWQiOiJjbGllbnRfaWQiLCJjbGllbnRfc2VjcmV0IjoiY2xpZW50X3NlY3JldCIsImVudmlyb25tZW50IjoiZW52IiwiZ2VuZXJhdGVkX2F0IjoxNzQ0MDk4NjEyfQ=="
	expectedToken := token{
		ClientId:     "client_id",
		ClientSecret: "client_secret",
		Environment:  "env",
		GeneratedAt:  time.Date(2025, 4, 8, 7, 50, 12, 0, time.UTC).Unix(),
	}

	decodedToken, err := decodeToken(given)
	Expect(err).NotTo(HaveOccurred())
	Expect(decodedToken).To(Equal(expectedToken))

	_, err = decodeToken("")
	Expect(err).To(HaveOccurred())
}
