<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Secret Manager API

## Overview

The Secret Manager API provides a way to manage secrets in a secure and efficient manner. It allows you to get and set secrets, as well as list all secrets. The API is designed to be easy to use and integrate with other services.


## Components

This module offers the following components:

### Secrets API

This API allows you to manage secrets. You can get and set secrets. The API is designed to be easy to use and integrate with other services.

```go
// Global default API
api.Get(ctx, "{{poc:eni--hyperion:my-foo-app:clientSecret:<some-checksum>}}")
api.Set(ctx, "{{poc:eni--hyperion:my-foo-app:clientSecret:<some-checksum>}}", "my-new-value")

// API with custom options
// Note: This API needs the clean ID of the secret without the start and end tags
secretsApi := api.NewSecrets()
secretsApi.Set(ctx, "poc:eni--hyperion:my-foo-app:clientSecret:<some-checksum>", "my-new-value")
secretsApi.Get(ctx, "poc:eni--hyperion:my-foo-app:clientSecret:<some-checksum>")
```

The global API is automatically initialized with the default options. It is recommended to use the global API for most use cases.
It will detect if the service is running in a local or Kubernetes environment and use the appropriate configuration.

### Onboarding API

This API allows you to manage the onboarding process. You can create and delete organizational structures like `Environments`, `Teams` and `Applications`.
This API should only be used by special services that are responsible for onboarding new customers. It is not intended to be used by regular users or applications.

```go
onboardingApi := api.NewOnboarding()

// Create a new environment
onboardingApi.UpsertEnvironment(ctx, "poc")

// Create a new team
onboardingApi.UpsertTeam(ctx, "poc", "eni--hyperion")

// Create a new application
onboardingApi.UpsertApplication(ctx, "poc", "eni--hyperion", "my-foo-app")

// You are also able to set secret-values directly in the onboarding request using the options-pattern
options := []api.OnboardingOption{
	api.WithSecretValue("clientSecret", "my-custom-value"),
    api.WithSecretValue("externalSecrets/foo/password", "my-foo-password")
}
availableSecrets, err := onboardingApi.UpsertApplication(ctx, "poc", "eni--hyperion", "my-foo-app", options...)
// handle error
// If you now want the secretId of the created secrets, you can do the following:
secretRef, ok := api.FindSecretId(availableSecrets, "externalSecrets/foo/password")
if !ok {
    // handle error
}
// $<poc:eni--hyperion:my-foo-app:externalSecrets/foo/password:checksum>
fmt.Println("Secret Ref:", secretRef)
```

> [!IMPORTANT]
> You may only set secrets which were already defined in the `SecretManager` configuration.
> This is to ensure that the secrets are properly managed and do not conflict with existing secrets.
> Example: If the application has `externalSecrets` configured, you can set secrets like `externalSecrets/foo/password` or `externalSecrets/bar/username`.
> The character used to indicate the hierarchy is a `/`.

## Vocabulary

| Name                               | Description                                                                                                                         |
|------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `environment`                      | The environment is the top level organizational structure.                                                                          |
| `team`                             | The team is the second level organizational structure. It is dynamically configured per onboarded team.                             |
| `application`                      | The application is the third level organizational structure. It is dynamically configured per onboarded application of a team.      |
| ------------                       | -----------------                                                                                                                   |
| `secret`                           | The secret is the actual secret value. It can be owned by any structural element.                                                   |
| `secretId`                         | The secret ID is the unique identifier of the secret. It is used to reference the secret in the API.                                |
| `secretName`                       | The secret name that is part of the secret ID. It is used to identity the secret in the context of the structural element.          |
| `secretPlaceholder` or `secretRef` | The secret placeholder is a variation of the secret ID using prefix and suffix tags. It is used to immediately identify the secret. |
| ------------                       | -----------------                                                                                                                   |