// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonconfig "github.com/telekom/controlplane/common/pkg/config"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"
)

const controlTimeout = 10 * time.Second

type routeRequest struct {
	Path     string `json:"path"`
	Host     string `json:"host"`
	Issuer   string `json:"issuer,omitempty"`
	Consumer string `json:"consumer,omitempty"`
}

type publicationResult struct {
	Generation uint64 `json:"generation"`
	Digest     string `json:"digest"`
	Activated  bool   `json:"activated"`
	Idempotent bool   `json:"idempotent"`
	Rejected   bool   `json:"rejected,omitempty"`
	Code       string `json:"code,omitempty"`
	Error      string `json:"error,omitempty"`
}

type statusResult struct {
	TargetID      string           `json:"targetId"`
	Generation    uint64           `json:"generation"`
	Digest        string           `json:"digest"`
	Activated     bool             `json:"activated"`
	Converged     bool             `json:"converged"`
	ConnectedNode []string         `json:"connectedNodes"`
	Observations  []map[string]any `json:"observations"`
}

func (s *operatorState) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /targetz", s.targetHealth)
	mux.HandleFunc("GET /status", s.getStatus)
	mux.HandleFunc("PUT /routes/demo", s.putRoute)
	mux.HandleFunc("DELETE /routes/demo", s.deleteRoute)
	mux.HandleFunc("POST /publish/idempotent", s.publishIdempotent)
	mux.HandleFunc("POST /publish/invalid", s.publishInvalid)
	mux.HandleFunc("POST /publish/nack", s.publishNACK)
	return mux
}

func (s *operatorState) targetHealth(response http.ResponseWriter, _ *http.Request) {
	if !s.targetReady.Load() {
		http.Error(response, "target identity is not ready", http.StatusServiceUnavailable)
		return
	}
	response.WriteHeader(http.StatusOK)
}

func (s *operatorState) health(response http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		http.Error(response, "operator has not published its initial bundle", http.StatusServiceUnavailable)
		return
	}
	response.WriteHeader(http.StatusOK)
}

func (s *operatorState) putRoute(response http.ResponseWriter, request *http.Request) {
	input := routeRequest{}
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(response, http.StatusBadRequest, fmt.Errorf("decoding route: %w", err))
		return
	}
	if !strings.HasPrefix(input.Path, "/") || input.Host == "" {
		writeError(response, http.StatusBadRequest, fmt.Errorf("path must start with / and host is required"))
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), controlTimeout)
	defer cancel()
	key := client.ObjectKey{Namespace: demoNamespace, Name: "demo-route"}
	route := &gatewayv1.Route{}
	err := s.client.Get(ctx, key, route)
	creating := apierrors.IsNotFound(err)
	if err != nil && !creating {
		writeError(response, http.StatusInternalServerError, fmt.Errorf("reading route: %w", err))
		return
	}
	if creating {
		route.ObjectMeta = metav1.ObjectMeta{
			Name: key.Name, Namespace: key.Namespace,
			Labels: map[string]string{commonconfig.EnvironmentLabelKey: demoEnvironment},
		}
	}
	route.Spec = demoRouteSpec(input)
	if creating {
		err = s.client.Create(ctx, route)
	} else {
		err = s.client.Update(ctx, route)
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, fmt.Errorf("writing route: %w", err))
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{"created": creating, "generation": route.Generation})
}

func demoRouteSpec(input routeRequest) gatewayv1.RouteSpec {
	secured := input.Issuer != ""
	security := gatewayv1.Security{RealmName: "xdsdemo"}
	if secured {
		security.TrustedIssuers = []string{input.Issuer}
		security.DefaultConsumers = []string{input.Consumer}
	}
	return gatewayv1.RouteSpec{
		GatewayRef: commontypes.ObjectRef{Name: demoGatewayName, Namespace: demoNamespace},
		Type:       gatewayv1.RouteTypePrimary, Hostnames: []string{input.Host}, Paths: []string{input.Path},
		PassThrough: !secured,
		Backend: gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{{
			Scheme: "http", Hostname: "172.30.0.10", Port: 80, Path: "/anything",
		}}},
		Security: security,
	}
}

func (s *operatorState) deleteRoute(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), controlTimeout)
	defer cancel()
	before, err := s.activeGeneration(ctx)
	if err != nil {
		writeError(response, http.StatusBadGateway, err)
		return
	}
	if revisionErr := s.advanceDeletionRevision(ctx); revisionErr != nil {
		writeError(response, http.StatusInternalServerError, revisionErr)
		return
	}
	markerGeneration, err := s.waitForGeneration(ctx, before)
	if err != nil {
		writeError(response, http.StatusGatewayTimeout, err)
		return
	}
	route := &gatewayv1.Route{ObjectMeta: metav1.ObjectMeta{Name: "demo-route", Namespace: demoNamespace}}
	if err := s.client.Delete(ctx, route); err != nil && !apierrors.IsNotFound(err) {
		writeError(response, http.StatusInternalServerError, fmt.Errorf("deleting route: %w", err))
		return
	}
	if _, err := s.waitForGeneration(ctx, markerGeneration); err != nil {
		writeError(response, http.StatusGatewayTimeout, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (s *operatorState) advanceDeletionRevision(ctx context.Context) error {
	key := client.ObjectKey{Namespace: demoNamespace, Name: "deletion-revision"}
	consumer := &gatewayv1.Consumer{}
	err := s.client.Get(ctx, key, consumer)
	creating := apierrors.IsNotFound(err)
	if err != nil && !creating {
		return fmt.Errorf("reading deletion revision: %w", err)
	}
	if creating {
		consumer.ObjectMeta = metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}
	}
	consumer.Spec = gatewayv1.ConsumerSpec{
		Gateway: commontypes.ObjectRef{Name: demoGatewayName, Namespace: demoNamespace},
		Name:    fmt.Sprintf("deletion-%d", time.Now().UnixNano()),
	}
	if creating {
		err = s.client.Create(ctx, consumer)
	} else {
		err = s.client.Update(ctx, consumer)
	}
	if err != nil {
		return fmt.Errorf("advancing deletion revision: %w", err)
	}
	return nil
}

func (s *operatorState) waitForGeneration(ctx context.Context, previous uint64) (uint64, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		generation, err := s.activeGeneration(ctx)
		if err == nil && generation > previous {
			return generation, nil
		}
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("waiting for generation after %d: %w", previous, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s *operatorState) activeGeneration(ctx context.Context) (uint64, error) {
	current, err := s.status.GetStatus(ctx, &xdsapi.GetStatusRequest{TargetId: s.targetID})
	if err != nil {
		return 0, fmt.Errorf("reading active generation: %w", err)
	}
	return current.PersistedGeneration, nil
}

func (s *operatorState) publishIdempotent(response http.ResponseWriter, request *http.Request) {
	bundle := s.bundle()
	if bundle == nil {
		writeError(response, http.StatusServiceUnavailable, fmt.Errorf("no bundle has been published"))
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), controlTimeout)
	defer cancel()
	result, err := s.publisher.Publish(ctx, bundle)
	if err != nil {
		writeError(response, http.StatusBadGateway, err)
		return
	}
	writeJSON(response, http.StatusOK, publicationResult{
		Generation: result.PersistedGeneration, Digest: result.Digest,
		Activated: result.Activated, Idempotent: result.Idempotent,
	})
}

func (s *operatorState) publishInvalid(response http.ResponseWriter, request *http.Request) {
	bundle := s.bundle()
	if bundle == nil || len(bundle.Listeners) == 0 {
		writeError(response, http.StatusServiceUnavailable, fmt.Errorf("no listener is available to duplicate"))
		return
	}
	bundle.PublisherGeneration += "-invalid"
	duplicate, ok := proto.Clone(bundle.Listeners[0]).(*anypb.Any)
	if !ok {
		writeError(response, http.StatusInternalServerError, fmt.Errorf("cloning listener resource"))
		return
	}
	bundle.Listeners = append(bundle.Listeners, duplicate)
	if digestErr := xdsapi.SetDigest(bundle); digestErr != nil {
		writeError(response, http.StatusInternalServerError, digestErr)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), controlTimeout)
	defer cancel()
	_, err := s.publisher.Publish(ctx, bundle)
	if err == nil {
		writeError(response, http.StatusInternalServerError, fmt.Errorf("invalid publication unexpectedly succeeded"))
		return
	}
	writeJSON(response, http.StatusOK, publicationResult{
		Rejected: true, Code: status.Code(err).String(), Error: status.Convert(err).Message(),
	})
}

func (s *operatorState) publishNACK(response http.ResponseWriter, request *http.Request) {
	bundle := s.bundle()
	if bundle == nil || len(bundle.Endpoints) == 0 {
		writeError(response, http.StatusServiceUnavailable, fmt.Errorf("no endpoint is available to invalidate"))
		return
	}
	endpoint := &endpointv3.ClusterLoadAssignment{}
	if err := bundle.Endpoints[0].UnmarshalTo(endpoint); err != nil {
		writeError(response, http.StatusInternalServerError, fmt.Errorf("decoding endpoint: %w", err))
		return
	}
	socket := endpoint.GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress()
	if socket == nil {
		writeError(response, http.StatusInternalServerError, fmt.Errorf("endpoint has no socket address"))
		return
	}
	socket.Address = "invalid host"
	packed, err := anypb.New(endpoint)
	if err != nil {
		writeError(response, http.StatusInternalServerError, fmt.Errorf("encoding endpoint: %w", err))
		return
	}
	bundle.Endpoints[0] = packed
	bundle.PublisherGeneration += "-nack"
	if digestErr := xdsapi.SetDigest(bundle); digestErr != nil {
		writeError(response, http.StatusInternalServerError, digestErr)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), controlTimeout)
	defer cancel()
	result, err := s.publisher.Publish(ctx, bundle)
	if err != nil {
		writeError(response, http.StatusBadGateway, err)
		return
	}
	writeJSON(response, http.StatusOK, publicationResult{
		Generation: result.PersistedGeneration, Digest: result.Digest,
		Activated: result.Activated, Idempotent: result.Idempotent,
	})
}

func (s *operatorState) getStatus(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), controlTimeout)
	defer cancel()
	current, err := s.status.GetStatus(ctx, &xdsapi.GetStatusRequest{TargetId: s.targetID})
	if err != nil {
		writeError(response, http.StatusBadGateway, err)
		return
	}
	writeJSON(response, http.StatusOK, statusView(current))
}

func statusView(current *xdsapi.GetStatusResponse) statusResult {
	result := statusResult{
		TargetID: current.TargetId, Generation: current.PersistedGeneration, Digest: current.Digest,
		Activated: current.Activated, Converged: current.Converged, ConnectedNode: current.ConnectedNodeIds,
		Observations: make([]map[string]any, 0, len(current.Observations)),
	}
	for _, observation := range current.Observations {
		result.Observations = append(result.Observations, map[string]any{
			"nodeId": observation.NodeId, "typeUrl": observation.TypeUrl,
			"generation": observation.Generation, "state": observation.State.String(),
			"error": observation.ErrorDetail,
		})
	}
	return result
}

func (s *operatorState) bundle() *xdsapi.Bundle {
	s.bundleMu.RLock()
	defer s.bundleMu.RUnlock()
	if s.lastBundle == nil {
		return nil
	}
	clone, ok := proto.Clone(s.lastBundle).(*xdsapi.Bundle)
	if !ok {
		return nil
	}
	return clone
}

func writeJSON(response http.ResponseWriter, code int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(code)
	if err := json.NewEncoder(response).Encode(value); err != nil {
		ctrl.Log.Error(err, "encoding control API response")
	}
}

func writeError(response http.ResponseWriter, code int, err error) {
	writeJSON(response, code, map[string]string{"error": err.Error()})
}
