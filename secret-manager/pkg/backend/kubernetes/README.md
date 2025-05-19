<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Kubernetes Backend

This backend stores the secrets in Kubernetes Secret (K8S-Secret) resources (`core/v1:Secret`).

## Onboarding

### Environment

When onboarding an environment, it will create a new K8S-Secret with the name `Secrets` in the namespace of the environment `${envId}`.

### Team

When onboarding a team, it will create a new K8S-Secret with the name `team-secrets` in the namespace of the team `${envId}--${teamId}`.

Right now, the team will only contain two secrets:

- `clientSecret`: Which is the secret that is used to authenticate the IDP-client for the team
- `teamToken`: Which is generated from the `clientSecret` and is used to authenticate the team using our CLIs

### Application

When onboarding an application, it will create a new K8S-Secret with the name `${appId}` in the namespace of the application `${envId}--${teamId}`.

Right now, the application will only contain two secrets:
- `clientSecret`: Which is the secret that is used to authenticate the IDP-client for the application
- `externalSecrets`: Which will contains the secrets that are dynamically provided by the user.

The `externalSecrets` are stored as a JSON-object in the K8S-Secret. The JSON-object will look like this:

```json
{
    "secretId": "secretValue",
    "foo": "bar",
}
```

## Secrets 

All secrets can be retrieved using the normal Secret-Manager API.
This Backend also supports fetching nested secrets from the `externalSecrets` JSON-object.

A secretId might then look like this:
```yaml
# <envId>:<teamId>:<appId>:<secretId>:<checksum>
my-env:my-team:my-app:externalSecrets/foo:checksum
```

> The checksum of the secret is calculcated from the resourceVersion of the underlying K8S-Secret. 