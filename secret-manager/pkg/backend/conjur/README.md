<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Conjur Backend

This backend stores the secrets in Conjur.

## Onboarding

### Environment

When onboarding an environment, it will create a new Conjur policy with the name `${envId}`.

```yaml
- !policy
  id: ${envId}
```

### Team

When onboarding a team, it will create a new Conjur policy under the environment `${envId}` with the name `${teamId}`.

```yaml
- !policy
  id: ${teamId}
  body:
  - !variable clientSecret
  - !variable teamToken
```

Right now, the team will only contain two secrets:
- `clientSecret`: Which is the secret that is used to authenticate the IDP-client for the team
- `teamToken`: Which is generated from the `clientSecret` and is used to authenticate the team using our CLIs



### Application

When onboarding an application, it will create a new Conjur policy under the environment `${envId}` and team `${teamId}` with the name `${appId}`.

```yaml
- !policy
  id: ${appId}
  body:
  - !variable clientSecret
  - !variable externalSecrets
```

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

> The checksum of the secret is calculcated from the hash of the underlying secret value (reduced to 6 bytes)