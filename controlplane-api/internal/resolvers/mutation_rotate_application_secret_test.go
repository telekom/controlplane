// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	gqlmodel "github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type stubApplicationService struct {
	payload *gqlmodel.RotateApplicationSecretPayload
}

func (s stubApplicationService) RotateApplicationSecret(context.Context, service.ResourceRef) (*gqlmodel.RotateApplicationSecretPayload, error) {
	return s.payload, nil
}

var _ = Describe("rotateApplicationSecret mutation", func() {
	It("returns service errors in the payload", func() {
		db := testutil.NewTestClient(GinkgoT())
		DeferCleanup(db.Close)
		seed := testutil.SeedStandard(db)

		svc := stubApplicationService{payload: &gqlmodel.RotateApplicationSecretPayload{
			Errors: []gqlmodel.MutationError{{Code: gqlmodel.ErrorCodeForbidden, Message: "rotation not supported"}},
		}}
		server := handler.New(resolvers.NewExecutableSchema(resolvers.Config{
			Resolvers: resolvers.NewResolver(db, service.Services{Application: svc}, nil, ""),
		}))
		server.AddTransport(transport.POST{})

		body, err := json.Marshal(map[string]string{"query": `mutation { rotateApplicationSecret(applicationId: "` +
			strconv.Itoa(seed.AppAlpha.ID) + `") { accepted application { id } errors { code message field } } }`})
		Expect(err).NotTo(HaveOccurred())
		req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(viewer.NewContext(context.Background(), &viewer.Viewer{Admin: true}))
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(recorder.Body.String()).To(MatchJSON(`{"data":{"rotateApplicationSecret":{
			"accepted":false,"application":null,
			"errors":[{"code":"FORBIDDEN","message":"rotation not supported","field":null}]
		}}}`))
	})
})
