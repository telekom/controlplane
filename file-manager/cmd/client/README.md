<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->


# File Manager CLI Client

# About


You can use this client to debug, test, and interact with the File Manager service.


## Local Deployment
If the File Manager service is running locally (`http://localhost:8443/api`), you can use the following command to run the client:

```bash
# First, start the File Manager service locally
go run cmd/server/server.go --disable-tls

# Then, run the client against the local service
go run cmd/client/client.go --file path/to/file --env my-file-id
```

This assumes that the service has been started without TLS and is accessible at `http://localhost:8443/api`.


## Remote Deployment
If the File Manager service is deployed remotely, you can specify the base URL of the service using the `--url` flag.
If the service is also secured using the k8s-authenticator, you will need to provide a valid access token using the `--token`  or `--token-file` flag.

```bash
# First, create a proxy to the remote service
kubectl -n controlplane-system port-forward svc/file-manager 8443:443

# Then, get an access token for the relevant service account
export NAMESPACE="rover-system"
export SERVICE_ACCOUNT="rover-controller-manager"
export TOKEN=$(kubectl create token -n $NAMESPACE $SERVICE_ACCOUNT --duration 10m)

# Finally, run the client with the remote URL and token
go run cmd/client/client.go --url https://localhost:8443/api --token $TOKEN --file path/to/file --env my-file-id
```
