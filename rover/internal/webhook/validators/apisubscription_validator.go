// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package validators

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	rover "github.com/telekom/controlplane/rover/api/v1"
	apihandler "github.com/telekom/controlplane/rover/internal/handler/rover/api"
	"io"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	apiSubWebhookEndpoint = "https://api-operator-webhook-service.api-operator-system.svc:443/validate-api-cp-ei-telekom-de-v1-apisubscription"
)

// CallApiSubscriptionWebhook Will call the ApiSubscription validating webhook and return a result and a reason
func CallApiSubscriptionWebhook(ctx context.Context, c client.ScopedClient, rover rover.Rover, apiSubscription rover.ApiSubscription) (bool, error) {
	log := log.FromContext(ctx)

	httpClient := buildHttpClient()

	ar, err := buildAdmissionReview(ctx, c, rover, apiSubscription)
	if err != nil {
		return false, errors.Wrap(err, "failed to build admission review")
	}
	body, err := json.Marshal(ar)
	if err != nil {
		return false, errors.Wrap(err, "Failed to marshal AdmissionReview")
	}

	// call the ApiSubscription webhook
	req, err := http.NewRequest("POST", apiSubWebhookEndpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "Failed to execute request to ApiSubscriptionWebhook")
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Error(err, "Failed to close response body after calling ApiSubscriptionWebhook")
		}
	}(resp.Body)

	// parse the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, errors.Wrap(err, "Failed to read response body")
	}

	var admissionResponse admissionv1.AdmissionReview
	err = json.Unmarshal(respBody, &admissionResponse)
	if err != nil {
		return false, errors.Wrap(err, "Failed to unmarshal AdmissionReview response after calling ApiSubscriptionWebhook")
	}

	if admissionResponse.Response.Allowed {
		return true, nil
	} else {
		return false, errors.New("ApiSubscription webhook rejected the ApiSubscription because '" + admissionResponse.Response.Result.Message + "'")
	}
}

func buildAdmissionReview(ctx context.Context, c client.ScopedClient, rover rover.Rover, apiSubscription rover.ApiSubscription) (*admissionv1.AdmissionReview, error) {
	apiSub, mutator, err := apihandler.BuildApiSubscription(ctx, c, rover, apiSubscription)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to build ApiSubscription admission review")
	}
	err = mutator()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to apply mutate function to ApiSubscription admission review")
	}

	apiSubJson, err := json.Marshal(apiSub)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to marshal ApiSubscription to JSON for admission review")
	}

	ar := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Request: &admissionv1.AdmissionRequest{
			UID: uuid.NewUUID(),
			Kind: metav1.GroupVersionKind{
				Group:   apiSub.GroupVersionKind().Group,
				Version: apiSub.GroupVersionKind().Version,
				Kind:    apiSub.GroupVersionKind().Kind,
			},
			Resource: metav1.GroupVersionResource{
				Group:    apiSub.GroupVersionKind().Group,
				Version:  apiSub.GroupVersionKind().Version,
				Resource: "apisubscriptions",
			},
			Object: runtime.RawExtension{
				Raw: apiSubJson, // marshaled JSON of the Domain2 CR you want to validate
			},
			Operation: admissionv1.Create, // or Update
		},
	}

	return &ar, nil
}

// buildHttpClient WARN skips TLS verification, but its ok, because its inside the cluster and we trust each other
func buildHttpClient() http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	return *client
}
