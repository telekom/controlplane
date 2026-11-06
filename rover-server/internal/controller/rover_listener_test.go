// Copyright 2026 Deutsche Telekom IT GmbH
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
)

var _ = Describe("Rover Controller listeners", func() {
	listener := func(fields map[string]any) map[string]any {
		l := map[string]any{"consumer": "eni--hyperion--consumer", "provider": "eni--hyperion--provider"}
		for k, v := range fields {
			l[k] = v
		}
		return l
	}

	putListeners := func(listeners ...map[string]any) *http.Response {
		body, err := json.Marshal(map[string]any{"zone": "dataplane1", "listeners": listeners})
		Expect(err).NotTo(HaveOccurred())
		req := httptest.NewRequest(http.MethodPut, "/rovers/eni--hyperion--rover-local-sub", bytes.NewReader(body))
		response, err := ExecuteRequest(req, groupToken)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	DescribeTable("rejects listeners that rover-server cannot provision",
		func(listeners []map[string]any, detail string) {
			response := putListeners(listeners...)
			ExpectStatus(response, nil, http.StatusBadRequest, "application/problem+json")

			var problem map[string]any
			Expect(json.NewDecoder(response.Body).Decode(&problem)).To(Succeed())
			Expect(problem).To(HaveKeyWithValue("detail", detail))
		},
		Entry("an eventType next to an apiBasePath",
			[]map[string]any{listener(map[string]any{"apiBasePath": "/echo/v1", "eventType": "de.telekom.test.v1"})},
			"listeners[0].eventType: event listeners are not supported yet"),
		Entry("an eventType without an apiBasePath",
			[]map[string]any{listener(map[string]any{"eventType": "de.telekom.test.v1"})},
			"listeners[0].eventType: event listeners are not supported yet"),
		Entry("an eventType on a later listener",
			[]map[string]any{listener(map[string]any{"apiBasePath": "/echo/v1"}), listener(map[string]any{"eventType": "de.telekom.test.v1"})},
			"listeners[1].eventType: event listeners are not supported yet"),
		Entry("an eventFilter next to an apiBasePath",
			[]map[string]any{listener(map[string]any{"apiBasePath": "/echo/v1", "eventFilter": map[string]any{"payload": []string{"orderId"}}})},
			"listeners[0].eventFilter: event listeners are not supported yet"),
		Entry("a listener without an apiBasePath",
			[]map[string]any{listener(nil)},
			"listeners[0].apiBasePath: must be set"),
	)

	It("accepts an API listener", func() {
		response := putListeners(listener(map[string]any{"apiBasePath": "/echo/v1"}))
		ExpectStatus(response, nil, http.StatusAccepted, "application/json")
	})
})
