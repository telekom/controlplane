// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

type HandlerHookStage string

const (
	PreRequestHook  HandlerHookStage = "pre-request"
	PostRequestHook HandlerHookStage = "post-request"
)

type HttpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// BaseHandler provides common functionality for all resource handlers
type BaseHandler struct {
	Kind       string
	APIVersion string
	// Resource is the resource type this handler manages, e.g. rovers, apispecifications, ...
	Resource string

	SupportsInfo bool
	priority     int

	serverURL string

	logger     logr.Logger
	httpClient HttpDoer

	MakeResourceName func(obj types.Object) string

	// Hooks allow to register functions that are called before or after requests
	Hooks map[HandlerHookStage][]func(ctx context.Context, obj types.Object) error

	applyStatusPoller  *StatusPoller
	deleteStatusPoller *StatusPoller
}

func NewBaseHandler(apiVersion, kind, resource string, priority int) *BaseHandler {
	handler := &BaseHandler{
		APIVersion: apiVersion,
		Kind:       kind,
		Resource:   resource,
		priority:   priority,
		logger:     log.L().WithName(fmt.Sprintf("%s-handler", resource)),
		Hooks:      make(map[HandlerHookStage][]func(ctx context.Context, obj types.Object) error),
	}
	handler.applyStatusPoller = NewStatusPoller(handler, nil, 30*time.Second, 1*time.Second)
	handler.deleteStatusPoller = NewStatusPoller(handler, func(ctx context.Context, status types.ObjectStatus) (continuePolling bool, err error) {
		return !status.IsGone(), nil
	}, 30*time.Second, 1*time.Second)

	return handler
}

func (h *BaseHandler) Setup(ctx context.Context) *config.Token {
	token := config.FromContextOrDie(ctx)
	if h.httpClient == nil {
		h.httpClient = NewAuthorizedHttpClient(ctx, token.TokenUrl, token.ClientId, token.ClientSecret)
	}
	if h.serverURL == "" {
		h.serverURL = token.ServerUrl
	}
	return token
}

func (h *BaseHandler) getResourceName(obj types.Object) string {
	if h.MakeResourceName != nil {
		return h.MakeResourceName(obj)
	}
	return obj.GetName()
}

func (h *BaseHandler) GetRequestUrl(groupName, teamName, resourceName string, subResources ...string) string {
	var url string
	if resourceName == "" {
		url = fmt.Sprintf("%s/%s", h.serverURL, h.Resource)
	} else {
		url = fmt.Sprintf("%s/%s/%s--%s--%s", h.serverURL, h.Resource, groupName, teamName, resourceName)
	}
	if len(subResources) > 0 {
		url += "/" + subResources[0]
		for _, subResource := range subResources[1:] {
			url += "/" + subResource
		}
	}
	return url
}

func (h *BaseHandler) Priority() int {
	// Default priority is 0, can be overridden by specific handlers
	return h.priority
}

// Apply implements the generic Apply operation using REST API
func (h *BaseHandler) Apply(ctx context.Context, obj types.Object) error {
	if obj == nil {
		return errors.New("object cannot be nil")
	}
	token := h.Setup(ctx)
	url := h.GetRequestUrl(token.Group, token.Team, h.getResourceName(obj))

	// Send the request
	resp, err := h.SendRequest(ctx, obj, http.MethodPut, url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response
	err = CheckResponseCode(resp, http.StatusOK, http.StatusAccepted)
	if err != nil {
		return err
	}

	return nil
}

// Delete implements the generic Delete operation using REST API
func (h *BaseHandler) Delete(ctx context.Context, obj types.Object) error {
	if obj == nil {
		return errors.New("object cannot be nil")
	}
	token := h.Setup(ctx)
	url := h.GetRequestUrl(token.Group, token.Team, h.getResourceName(obj))

	// Send the request
	resp, err := h.SendRequest(ctx, obj, http.MethodDelete, url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response
	err = CheckResponseCode(resp, http.StatusOK, http.StatusNoContent, http.StatusNotFound)
	if err != nil {
		return err
	}

	return nil
}

// Get implements the generic Get operation using REST API
func (h *BaseHandler) Get(ctx context.Context, name string) (any, error) {
	token := h.Setup(ctx)
	url := h.GetRequestUrl(token.Group, token.Team, name)

	// Send the request (no obj, so no hooks will be executed)
	resp, err := h.SendRequest(ctx, nil, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response
	err = CheckResponseCode(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}

	// Parse response
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}

	return result, nil
}

// List implements the generic List operation using REST API
func (h *BaseHandler) List(ctx context.Context) ([]any, error) {
	items := make([]any, 0)
	h.Setup(ctx)
	nextCursor := ""
	for {
		h.logger.V(1).Info("Listing resources", "resource", h.Resource, "cursor", nextCursor)
		resp, err := h.ListWithCursor(ctx, nextCursor)
		if err != nil {
			return nil, err
		}
		items = append(items, resp.Items...)
		if resp.Links.Next == "" {
			h.logger.V(1).Info("No more items to list", "resource", h.Resource)
			break
		}
		nextCursor = resp.Links.Next
	}

	return items, nil
}

func (h *BaseHandler) ListWithCursor(ctx context.Context, cursor string) (*ListResponse, error) {
	h.Setup(ctx)
	url := h.GetRequestUrl("", "", "")

	// Add cursor parameter if provided
	if cursor != "" {
		url += "?cursor=" + cursor
	}

	// Send the request (no obj, so no hooks will be executed)
	resp, err := h.SendRequest(ctx, nil, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response
	err = CheckResponseCode(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}

	// Parse response
	var response ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}

	return &response, nil
}

func (h *BaseHandler) Status(ctx context.Context, name string) (types.ObjectStatus, error) {
	token := h.Setup(ctx)
	url := h.GetRequestUrl(token.Group, token.Team, name, "status")

	// Send the request (no obj, so no hooks will be executed)
	resp, err := h.SendRequest(ctx, nil, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response
	err = CheckResponseCode(resp, http.StatusOK, http.StatusNotFound)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		h.logger.V(1).Info("Status not found", "name", name)
		return &ObjectStatusResponse{Gone: true}, nil
	}

	// Parse response
	var status ObjectStatusResponse
	err = json.NewDecoder(resp.Body).Decode(&status)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse status response")
	}

	h.logger.V(1).Info("Status response", "status", status)

	return &status, nil
}

func (h *BaseHandler) Info(ctx context.Context, name string) (any, error) {
	if !h.SupportsInfo {
		return nil, errors.Errorf("info operation is not supported for %s", h.Resource)
	}
	token := h.Setup(ctx)
	url := h.GetRequestUrl(token.Group, token.Team, name, "info")

	// Send the request (no obj, so no hooks will be executed)
	resp, err := h.SendRequest(ctx, nil, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response
	err = CheckResponseCode(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}

	// Parse response
	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}

	return info, nil
}

func (h *BaseHandler) AddHook(stage HandlerHookStage, hook func(ctx context.Context, obj types.Object) error) {
	if h.Hooks == nil {
		h.Hooks = make(map[HandlerHookStage][]func(ctx context.Context, obj types.Object) error)
	}
	h.Hooks[stage] = append(h.Hooks[stage], hook)
}

func (h *BaseHandler) RunHooks(stage HandlerHookStage, ctx context.Context, obj types.Object) error {
	if hooks, ok := h.Hooks[stage]; ok {
		for _, hook := range hooks {
			if err := hook(ctx, obj); err != nil {
				return errors.Wrapf(err, "hook failed at stage %s", stage)
			}
		}
	}
	return nil
}

// SendRequest handles common request operations including running hooks
func (h *BaseHandler) SendRequest(ctx context.Context, obj types.Object, method, url string) (*http.Response, error) {

	// Run pre-request hooks if object is provided
	if obj != nil {
		if err := h.RunHooks(PreRequestHook, ctx, obj); err != nil {
			return nil, err
		}
	}

	var body io.ReadWriter
	if obj != nil {
		content := obj.GetContent()
		buf := bytes.NewBuffer(nil)
		err := json.NewEncoder(buf).Encode(content)
		if err != nil {
			return nil, errors.Wrap(err, "failed to encode request body")
		}
		body = buf

		if viper.GetBool("debug") {
			h.logger.V(1).Info("Request details", "method", method, "url", url, "body", buf.String())
		}
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	// Set content type for requests with body
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	h.logger.V(1).Info("Sending request", "method", method, "url", url)

	// Send the request
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}

	h.logger.V(1).Info("Received response", "status", resp.Status)

	// Run post-request hooks if object is provided
	if obj != nil {
		if err := h.RunHooks(PostRequestHook, ctx, obj); err != nil {
			return nil, err
		}
	}

	return resp, nil
}

func (h *BaseHandler) WaitForReady(ctx context.Context, name string) (types.ObjectStatus, error) {
	h.logger.Info("Waiting for readiness", "name", name)
	status, err := h.applyStatusPoller.Start(ctx, name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for readiness")
	}

	return status, nil
}

func (h *BaseHandler) WaitForDeleted(ctx context.Context, name string) (types.ObjectStatus, error) {
	h.logger.Info("Waiting for deletion", "name", name)
	status, err := h.deleteStatusPoller.Start(ctx, name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for deletion")
	}

	return status, nil
}

func CheckResponseCode(resp *http.Response, expectedCodes ...int) error {
	if slices.Contains(expectedCodes, resp.StatusCode) {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "failed to read response body: %s", resp.Status)
	}

	apiErr := &ApiError{}
	if err := json.Unmarshal(body, apiErr); err != nil {
		return &ApiError{
			Type:     "UnknownError",
			Status:   resp.StatusCode,
			Title:    "Unexpected Response",
			Detail:   string(body),
			Instance: fmt.Sprintf("%s/%s", resp.Request.Method, resp.Request.URL),
		}
	}

	return apiErr
}
