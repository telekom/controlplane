<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->


# Local Installation 

## Updating resources

Before installing the operators and CRDs, ensure that you have updated the resources in the `resources\admin\zones`
directory. This is important, because they contain configuration for the identity provider and the gateway. You either
need to provide them on your local machine or use existing resources from a running environment.

Please update `dataplane1.yaml` and `dataplane2.yaml` in the `resources/admin/zones` directory with your identity 
provider and gateway configuration:

**IdentityProvider**
```yaml
  identityProvider:
    admin:
      clientId: admin-cli
      userName: admin
      password: somePassword
    url: https://my-idp.example.com/
```

**Gateway**
```yaml
  gateway:
    admin:
      clientSecret: someSecret
      url: https://my-gateway-admin.example.com/admin-api
    url: https://my-gateway.example.com/
```


## Installing Operators

To install all operators and CRDs locally, you can use the following command:

```bash
# cd install/local
kubectl apply -k .
```

After all operators and CRDs are installed, you can verify the installation by checking the status of the operators:

```bash
kubectl get pods -A -l control-plane=controller-manager
```

## Install Admin CRs

To install the admin CRs, you can use the following command. This will create an environment `default` and a zone `zone-a` in that environment.

```bash
kubectl apply -k resources/admin

# Verify
kubectl wait --for=condition=Ready -n controlplane zones/dataplane1
```

## Install Organisational CRs

To install the organisational CRs, you can use the following command. This will create a group `group-sample` and a team `team-sample` in that group. Both are in the environment `default`.

```bash
kubectl apply -k resources/org

# Verify
kubectl wait --for=condition=Ready -n controlplane teams/phoenix--firebirds
```

## Install Rover CRs

To install the rover CRs, you can use the following command. This will create a rover `rover-sample` in the team namespace `default--group-sample--team-sample`. The rover will be assigned to the zone `zone-a` in the environment `default`.

```bash
kubectl apply -k resources/rover

# Verify 
kubectl wait --for=condition=Ready -n controlplane--phoenix--firebirds rovers/rover-echo-v1
```

