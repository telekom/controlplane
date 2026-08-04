// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
)

func (c *kongClient) CreateOrReplaceConsumer(ctx context.Context, consumer CustomConsumer) (*kong.Consumer, error) {
	consumerName := consumer.GetConsumerName()
	tags := []string{
		BuildTag("env", contextutil.EnvFromContextOrDie(ctx)),
		BuildTag("consumer", consumerName),
	}

	body := kong.CreateConsumerJSONRequestBody{
		CustomId: consumerName,
		Username: consumerName,
		Tags:     normalizeSet(&tags),
	}

	kongConsumer, _, err := reconcile(ctx, consumerEntity{client: c.client, name: consumerName}, body)
	if err != nil {
		return nil, err
	}
	if kongConsumer.Id == nil {
		return nil, fmt.Errorf("consumer response ID is missing")
	}

	isInGroup, err := c.isConsumerInGroup(ctx, consumerName)
	if err != nil {
		return nil, err
	}
	if !isInGroup {
		if err := c.addConsumerToGroup(ctx, consumerName); err != nil {
			return nil, fmt.Errorf("failed to add consumer to group: %w", err)
		}
	}

	consumer.SetId(*kongConsumer.Id)
	return kongConsumer, nil
}

func (c *kongClient) DeleteConsumer(ctx context.Context, consumer CustomConsumer) error {
	response, err := c.client.DeleteConsumerWithResponse(ctx, consumer.GetConsumerName())
	if err != nil {
		return fmt.Errorf("failed to delete consumer: %w", HandleClientError(err))
	}
	if err := CheckStatusCode(response, http.StatusOK, http.StatusNoContent, http.StatusNotFound); err != nil {
		return fmt.Errorf("failed to delete consumer (%d): %s: %w", response.StatusCode(), summarizeBody(response.Body), err)
	}
	return nil
}

func (c *kongClient) addConsumerToGroup(ctx context.Context, consumerName string) error {
	groupName := consumerName
	response, err := c.client.AddConsumerToGroupWithResponse(ctx, consumerName, kong.AddConsumerToGroupJSONRequestBody{
		Group: &groupName,
	})
	if err != nil {
		return fmt.Errorf("failed to add consumer to group: %w", HandleClientError(err))
	}
	if err := CheckStatusCode(response, http.StatusOK, http.StatusCreated); err != nil {
		return fmt.Errorf("failed to add consumer to group (%d): %s: %w", response.StatusCode(), summarizeBody(response.Body), err)
	}

	return nil
}

func (c *kongClient) isConsumerInGroup(ctx context.Context, consumerName string) (bool, error) {
	response, err := c.client.ViewGroupConsumerWithResponse(ctx, consumerName)
	if err != nil {
		return false, fmt.Errorf("error occurred when getting consumer group: %w", HandleClientError(err))
	}

	if err := CheckStatusCode(response, http.StatusOK); err != nil {
		return false, fmt.Errorf("error occurred when getting consumer group: %w", err)
	}
	if response.JSON200 == nil {
		return false, fmt.Errorf("consumer group response body is missing")
	}
	if response.JSON200.Data == nil {
		return false, fmt.Errorf("consumer group response data is missing")
	}

	return len(*response.JSON200.Data) > 0, nil
}

type consumerEntity struct {
	client KongAdminApi
	name   string
}

var _ entity[kong.CreateConsumerJSONRequestBody, kong.Consumer] = consumerEntity{}

func (e consumerEntity) Name() string { return "consumer" }

func (e consumerEntity) Get(ctx context.Context) (*kong.Consumer, bool, error) {
	response, err := e.client.GetConsumerWithResponse(ctx, e.name)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get consumer: %w", HandleClientError(err))
	}
	return readOne("consumer", readResult[kong.Consumer]{response.StatusCode(), response.Body, response.JSON200})
}

func (e consumerEntity) Project(current *kong.Consumer) (kong.CreateConsumerJSONRequestBody, error) {
	return kong.CreateConsumerJSONRequestBody{
		CustomId: valueOrZero(current.CustomId),
		Username: valueOrZero(current.Username),
		Tags:     normalizeSet(current.Tags),
	}, nil
}

func (e consumerEntity) Write(ctx context.Context, desired *kong.CreateConsumerJSONRequestBody) (*kong.Consumer, error) {
	response, err := e.client.UpsertConsumerWithResponse(ctx, e.name, *desired)
	if err != nil {
		return nil, fmt.Errorf("failed to write consumer: %w", HandleClientError(err))
	}
	// The API spec declares a wrong type for this response body, so it is decoded here.
	var written *kong.Consumer
	if response.StatusCode() == http.StatusOK {
		if err := json.Unmarshal(response.Body, &written); err != nil {
			return nil, fmt.Errorf("failed to unmarshal consumer response: %w", err)
		}
	}
	return writeOne("consumer", readResult[kong.Consumer]{response.StatusCode(), response.Body, written}, http.StatusOK)
}

// --- upstream --------------------------------------------------------------
