// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/gkampitakis/go-snaps/match"
	. "github.com/onsi/ginkgo/v2"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"

	"github.com/telekom/controlplane/rover-server/internal/api"
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
	})

	Context("Reset rover secret", func() {
		It("should reset the rover secret successfully", func() {
			req := httptest.NewRequest(http.MethodPatch, "/rovers/eni--hyperion--rover-local-sub/secret", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json", match.Any("secret"))
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
