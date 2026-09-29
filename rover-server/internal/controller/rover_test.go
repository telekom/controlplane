// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/test/mocks"
)

var _ = Describe("Rover Controller", func() {
	Context("GetAll rover resources", func() {
		It("should return all rovers successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers", nil)

			responseTeam, err := ExecuteRequest(req, teamToken)
			ExpectStatusOk(responseTeam, err)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusOk(responseGroup, err)
		})

		It("should return an empty list if no rovers exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers", nil)

			responseTeam, err := ExecuteRequest(req, teamToken)
			ExpectStatusWithBody(responseTeam, err, http.StatusOK, "application/json")
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})
	})

	Context("Get rover resource", func() {
		It("should get a rover successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/eni--hyperion--rover-local-sub", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusOk(responseGroup, err)
		})

		It("should fail to get a non-existent rover", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/eni--hyperion--blabla", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get a rover from a different team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/rovers/other--team--rover", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatus(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Get rover application info", func() {
		It("should return application info successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/eni--hyperion--rover-local-sub/info", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get application info for a non-existent rover", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/eni--hyperion--blabla/info", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get application info from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/other--team--rover/info", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Get all rover applications info", func() {
		teamToken := mock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:team:all"})
		It("should return all applications info successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/info", nil)
			responseTeam, err := ExecuteRequestWithToken(req, teamToken)
			ExpectStatusOk(responseTeam, err)
		})

		It("should fail to get applications info for unauthorized client type", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/info", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusBadRequest, "application/problem+json")
		})

		It("should filter applications by names query parameter", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/info?names=rover-local-sub", nil)
			responseTeam, err := ExecuteRequestWithToken(req, teamToken)
			ExpectStatusOk(responseTeam, err)
		})

		It("should return empty list when names filter matches no rovers", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/info?names=nonexistent", nil)
			responseTeam, err := ExecuteRequestWithToken(req, teamToken)
			Expect(err).To(BeNil())
			Expect(responseTeam.StatusCode).To(Equal(http.StatusOK))

			var resp api.RoverInfoResponse
			err = json.NewDecoder(responseTeam.Body).Decode(&resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.Applications).To(BeEmpty())
		})
	})

	Context("Get rover status", func() {
		It("should return the status of a rover successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/eni--hyperion--rover-local-sub/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get the status of a non-existent rover", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/eni--hyperion--blabla/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get the status from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/other--team--rover/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Delete rover resource", func() {
		It("should delete a rover successfully", func() {
			req := httptest.NewRequest(http.MethodDelete, "/rovers/eni--hyperion--rover-local-sub", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			// Hint: The expected content-type is empty, because there is no response body for DELETE
			ExpectStatus(responseGroup, err, http.StatusNoContent, "")
		})

		It("should fail to delete a non-existent rover", func() {
			req := httptest.NewRequest(http.MethodDelete, "/rovers/eni--hyperion--blabla", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to delete a rover from a different team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/rovers/other--team--rover", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatus(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Create rover resource", func() {
		It("should create a rover successfully", func() {
			body := api.RoverCreateRequest{
				Name: "rover-demo",
				Zone: "dataplane1",
			}
			jsonBody, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/rovers", bytes.NewReader(jsonBody))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusNotImplemented(responseGroup, err)
		})

		It("should fail to create a rover with invalid input", func() {
			body := map[string]string{
				"invalidField": "invalidValue",
			}
			jsonBody, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/rovers", bytes.NewReader(jsonBody))

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusBadRequest, "application/problem+json")
		})
	})

	Context("Update rover resource", func() {
		validEnumValues := []struct {
			name  string
			path  string
			value string
		}{
			{name: "approval AUTO", path: "approval", value: "AUTO"},
			{name: "approval SIMPLE", path: "approval", value: "SIMPLE"},
			{name: "approval FOUREYES", path: "approval", value: "FOUREYES"},
			{name: "approval auto", path: "approval", value: "auto"},
			{name: "approval simple", path: "approval", value: "simple"},
			{name: "approval foureyes", path: "approval", value: "foureyes"},
			{name: "client auth NONE", path: "clientAuthMethod", value: "NONE"},
			{name: "client auth POST", path: "clientAuthMethod", value: "POST"},
			{name: "client auth BASIC", path: "clientAuthMethod", value: "BASIC"},
			{name: "client auth none", path: "clientAuthMethod", value: "none"},
			{name: "client auth post", path: "clientAuthMethod", value: "post"},
			{name: "client auth BODY", path: "clientAuthMethod", value: "BODY"},
			{name: "client auth body", path: "clientAuthMethod", value: "body"},
			{name: "client auth basic", path: "clientAuthMethod", value: "basic"},
			{name: "grant type PASSWORD", path: "grantType", value: "PASSWORD"},
			{name: "grant type CLIENT_CREDENTIALS", path: "grantType", value: "CLIENT_CREDENTIALS"},
			{name: "grant type REFRESH_TOKEN", path: "grantType", value: "REFRESH_TOKEN"},
			{name: "grant type password", path: "grantType", value: "password"},
			{name: "grant type client_credentials", path: "grantType", value: "client_credentials"},
			{name: "grant type refresh_token", path: "grantType", value: "refresh_token"},
			{name: "visibility ENTERPRISE", path: "visibility", value: "ENTERPRISE"},
			{name: "visibility WORLD", path: "visibility", value: "WORLD"},
			{name: "visibility ZONE", path: "visibility", value: "ZONE"},
			{name: "visibility enterprise", path: "visibility", value: "enterprise"},
			{name: "visibility world", path: "visibility", value: "world"},
			{name: "visibility zone", path: "visibility", value: "zone"},
			{name: "claim PROVIDER_CLIENT_ID", path: "valueFrom", value: "PROVIDER_CLIENT_ID"},
			{name: "claim CONSUMER_CLIENT_ID", path: "valueFrom", value: "CONSUMER_CLIENT_ID"},
			{name: "claim BASE_PATH", path: "valueFrom", value: "BASE_PATH"},
		}

		for _, test := range validEnumValues {
			test := test
			It("should accept "+test.name+" at the HTTP boundary", func() {
				body := roverEnumRequest(test.path, test.value)
				jsonBody, err := json.Marshal(body)
				Expect(err).NotTo(HaveOccurred())

				req := httptest.NewRequest(http.MethodPut, "/rovers/eni--hyperion--rover-local-sub", bytes.NewReader(jsonBody))
				response, err := ExecuteRequest(req, groupToken)
				ExpectStatus(response, err, http.StatusAccepted, "application/json")
			})
		}

		invalidEnumValues := []struct {
			name  string
			path  string
			value string
		}{
			{name: "approval mixed case", path: "approval", value: "Auto"},
			{name: "approval unknown", path: "approval", value: "manual"},
			{name: "client auth mixed case", path: "clientAuthMethod", value: "Post"},
			{name: "client auth unknown", path: "clientAuthMethod", value: "header"},
			{name: "grant type mixed case", path: "grantType", value: "Client_Credentials"},
			{name: "grant type unknown", path: "grantType", value: "authorization_code"},
			{name: "visibility mixed case", path: "visibility", value: "World"},
			{name: "visibility unknown", path: "visibility", value: "public"},
			{name: "claim provider mixed case", path: "valueFrom", value: "Provider_Client_Id"},
			{name: "claim provider lowercase", path: "valueFrom", value: "provider_client_id"},
			{name: "claim consumer mixed case", path: "valueFrom", value: "Consumer_Client_Id"},
			{name: "claim consumer lowercase", path: "valueFrom", value: "consumer_client_id"},
			{name: "claim base path mixed case", path: "valueFrom", value: "Base_Path"},
			{name: "claim base path lowercase", path: "valueFrom", value: "base_path"},
			{name: "claim old provider value", path: "valueFrom", value: "PROVIDERCLIENTID"},
			{name: "claim old consumer value", path: "valueFrom", value: "CONSUMERCLIENTID"},
			{name: "claim old base path value", path: "valueFrom", value: "BASEPATH"},
			{name: "claim unknown", path: "valueFrom", value: "SUBJECT"},
		}

		for _, test := range invalidEnumValues {
			test := test
			It("should reject "+test.name+" at the HTTP boundary", func() {
				body := roverEnumRequest(test.path, test.value)
				jsonBody, err := json.Marshal(body)
				Expect(err).NotTo(HaveOccurred())

				req := httptest.NewRequest(http.MethodPut, "/rovers/eni--hyperion--rover-local-sub", bytes.NewReader(jsonBody))
				response, err := ExecuteRequest(req, groupToken)
				ExpectStatus(response, err, http.StatusBadRequest, "application/problem+json")
			})
		}

		It("should apply defaults before the controller persists the rover", func() {
			body := map[string]any{
				"zone":           "dataplane1",
				"authentication": map[string]any{},
				"exposures": []map[string]any{{
					"type":     "api",
					"basePath": "/test",
					"upstream": "https://example.com",
				}},
			}
			jsonBody, err := json.Marshal(body)
			Expect(err).NotTo(HaveOccurred())

			roverStore := stores.RoverStore.(*mocks.MockObjectStore[*roverv1.Rover])
			callsBefore := len(roverStore.Calls)
			req := httptest.NewRequest(http.MethodPut, "/rovers/eni--hyperion--rover-local-sub", bytes.NewReader(jsonBody))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")

			var persisted *roverv1.Rover
			for _, call := range roverStore.Calls[callsBefore:] {
				if call.Method == "CreateOrReplace" {
					persisted = call.Arguments.Get(1).(*roverv1.Rover)
				}
			}
			Expect(persisted).NotTo(BeNil())
			Expect(persisted.Spec.Authentication).To(BeNil())
			Expect(persisted.Spec.Exposures[0].Api.Approval.Strategy).To(Equal(roverv1.ApprovalStrategySimple))
			Expect(persisted.Spec.Exposures[0].Api.Visibility).To(Equal(roverv1.VisibilityEnterprise))
		})

		It("should update a rover successfully", func() {
			body := api.RoverUpdateRequest{
				Zone: "dataplane1",
			}
			jsonBody, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPut, "/rovers/eni--hyperion--rover-local-sub", bytes.NewReader(jsonBody))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")
		})

		It("should fail to update a rover with invalid input", func() {
			body := api.RoverUpdateRequest{
				Id:   "rover-demo", // Invalid field for update, because it is read-only
				Zone: "dataplane1",
			}
			jsonBody, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPut, "/rovers/eni--hyperion--rover-local-sub", bytes.NewReader(jsonBody))

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusBadRequest, "application/problem+json")
		})

		It("should fail to update a rover for a non-existent rover", func() {
			body := api.RoverUpdateRequest{
				Zone: "dataplane1",
			}
			jsonBody, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPut, "/rovers/eni--hyperion--blabla", bytes.NewReader(jsonBody))

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to update a rover from a different team", func() {
			body := api.RoverUpdateRequest{
				Zone: "dataplane1",
			}
			jsonBody, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPut, "/rovers/other--team--blabla", bytes.NewReader(jsonBody))

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})

		It("should accept clientAuthMethod BASIC as produced by rover-ctl", func() {
			body := api.RoverUpdateRequest{
				Zone: "dataplane1",
				Authentication: api.Authentication{
					ClientAuthMethod: api.AuthenticationClientAuthMethodBASIC,
				},
			}
			jsonBody, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPut, "/rovers/eni--hyperion--rover-local-sub", bytes.NewReader(jsonBody))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")
		})

		It("should accept clientAuthMethod POST as produced by rover-ctl", func() {
			body := api.RoverUpdateRequest{
				Zone: "dataplane1",
				Authentication: api.Authentication{
					ClientAuthMethod: api.AuthenticationClientAuthMethodPOST,
				},
			}
			jsonBody, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPut, "/rovers/eni--hyperion--rover-local-sub", bytes.NewReader(jsonBody))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")
		})
	})

	Context("Reset rover secret", func() {
		It("should reset the rover secret successfully", func() {
			req := httptest.NewRequest(http.MethodPatch, "/rovers/eni--hyperion--rover-local-sub/secret", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")
		})

		It("should fail to reset the secret for a non-existent rover", func() {
			req := httptest.NewRequest(http.MethodPatch, "/rovers/eni--hyperion--blabla/secret", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to reset the secret from a different team", func() {
			req := httptest.NewRequest(http.MethodPatch, "/rovers/other--team--rover/secret", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

})

func roverEnumRequest(path, value string) map[string]any {
	body := map[string]any{"zone": "dataplane1"}
	switch path {
	case "clientAuthMethod":
		body["authentication"] = map[string]any{"clientAuthMethod": value}
	case "approval", "visibility":
		exposure := map[string]any{
			"type": "api", "basePath": "/test", "upstream": "https://example.com",
		}
		if path == "approval" {
			exposure["approval"] = value
			exposure["visibility"] = "WORLD"
		} else {
			exposure["approval"] = "AUTO"
			exposure["visibility"] = value
		}
		body["exposures"] = []map[string]any{exposure}
	case "grantType":
		body["exposures"] = []map[string]any{{
			"type": "api", "basePath": "/test", "upstream": "https://example.com", "approval": "AUTO", "visibility": "WORLD",
			"security": map[string]any{"type": "oauth2", "tokenEndpoint": "https://example.com/token", "grantType": value},
		}}
	case "valueFrom":
		body["exposures"] = []map[string]any{{
			"type": "api", "basePath": "/test", "upstream": "https://example.com", "approval": "AUTO", "visibility": "WORLD",
			"security": map[string]any{"type": "oauth2", "claims": map[string]any{"aud": map[string]any{"valueFrom": value}}},
		}}
	}
	return body
}
