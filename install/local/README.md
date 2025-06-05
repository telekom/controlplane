<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->


# Local Installation 

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
kubectl wait --for=condition=Ready -n default zones/zone-a
```

## Install Organisational CRs

To install the organisational CRs, you can use the following command. This will create a group `group-sample` and a team `team-sample` in that group. Both are in the environment `default`.

```bash
kubectl apply -k resources/org

# Verify
kubectl wait --for=condition=Ready -n default teams/group-sample--team-sample
```

## Install Rover CRs

To install the rover CRs, you can use the following command. This will create a rover `rover-sample` in the team namespace `default--group-sample--team-sample`. The rover will be assigned to the zone `zone-a` in the environment `default`.

```bash
kubectl apply -k resources/rover

# Verify 
kubectl wait --for=condition=Ready -n default--group-sample--team-sample rovers/rover-sample
```

