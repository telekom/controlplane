<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->


# File Manager CLI Client

## About

You can use this client to debug, test, and interact with the File Manager service. The client supports three main operations:
- **Upload**: Store files in the File Manager
- **Download**: Retrieve files from the File Manager
- **Delete**: Remove files from the File Manager

## Usage Scenarios

### Upload a File

Upload a file to the File Manager with a specific file ID:

```bash
# Upload a file with file ID and file path
go run cmd/client/client.go --id poc--eni--team--my-file.txt --file path/to/local/file.txt

# Upload with authentication token
go run cmd/client/client.go --url https://localhost:8443/api --token $TOKEN --id poc--eni--team--my-file.txt --file path/to/local/file.txt

# Upload without checksum validation
go run cmd/client/client.go --id poc--eni--team--my-file.txt --file path/to/local/file.txt --no-checksum
```

**Note**: The file ID must follow the format `<env>--<group>--<team>--<filename>` (e.g., `poc--eni--hyperion--config.yaml`).

### Download a File

Download a file from the File Manager using its file ID:

```bash
# Download to stdout
go run cmd/client/client.go --id poc--eni--team--my-file.txt

# Download to a specific file
go run cmd/client/client.go --id poc--eni--team--my-file.txt --output path/to/save/file.txt

# Download with authentication token
go run cmd/client/client.go --url https://localhost:8443/api --token $TOKEN --id poc--eni--team--my-file.txt --output downloaded-file.txt
```

### Delete a File

Delete a file from the File Manager:

```bash
# Delete a file by its ID
go run cmd/client/client.go --id poc--eni--team--my-file.txt --delete

# Delete with authentication token
go run cmd/client/client.go --url https://localhost:8443/api --token $TOKEN --id poc--eni--team--my-file.txt --delete
```

**Note**: If the file doesn't exist (404), the operation is treated as successful since the desired state (file not existing) is achieved.

## Local Deployment

If the File Manager service is running locally (`http://localhost:8443/api`), you can use the following command to run the client:

```bash
# First, start the File Manager service locally
go run cmd/server/server.go --disable-tls

# Then, run the client against the local service (upload example)
go run cmd/client/client.go --id poc--eni--team--my-file.txt --file path/to/file.txt
```

This assumes that the service has been started without TLS and is accessible at `http://localhost:8443/api`.


## Remote Deployment

If the File Manager service is deployed remotely, you can specify the base URL of the service using the `--url` flag.
If the service is also secured using the k8s-authenticator, you will need to provide a valid access token using the `--token` or `--token-file` flag.

```bash
# First, create a proxy to the remote service
kubectl -n file-manager-system port-forward svc/file-manager 8443:443

# Then, get an access token for the relevant service account
export NAMESPACE="rover-system"
export SERVICE_ACCOUNT="rover-controller-manager"
export TOKEN=$(kubectl create token -n $NAMESPACE $SERVICE_ACCOUNT --duration 10m)

# Upload a file
go run cmd/client/client.go --url https://localhost:8443/api --token $TOKEN --id poc--eni--team--my-file.txt --file path/to/file.txt

# Download a file
go run cmd/client/client.go --url https://localhost:8443/api --token $TOKEN --id poc--eni--team--my-file.txt --output downloaded-file.txt

# Delete a file
go run cmd/client/client.go --url https://localhost:8443/api --token $TOKEN --id poc--eni--team--my-file.txt --delete
```

## Available Flags

| Flag            | Description                                                          | Required   |
|-----------------|----------------------------------------------------------------------|------------|
| `--url`         | API URL (default: `http://localhost:8443/api` or in-cluster service) | No         |
| `--token`       | API access token                                                     | No         |
| `--token-file`  | Path to file containing API access token                             | No         |
| `--id`          | File ID in format `<env>--<group>--<team>--<filename>`               | Yes        |
| `--file`        | Local file path (for upload)                                         | For upload |
| `--output`      | Output file path (for download, defaults to stdout)                  | No         |
| `--delete`      | Delete the file with the specified ID                                | For delete |
| `--no-checksum` | Skip checksum validation                                             | No         |
