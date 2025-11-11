<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

<p align="center">
  <h1 align="center">File Manager</h1>
</p>

<p align="center">
  File Manager (FM) is a RESTful API for managing files. It allows for storing and retrieving files.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#architecture">Architecture</a> •
  <a href="#backends">Backends</a> •
  <a href="#security">Security</a> •
  <a href="#code-integration">Code Integration</a>
  <a href="#deployment-integration">Deployment Integration</a>
</p>

## About

File Manager (FM) is a RESTful API for managing files. It allows for storing and retrieving files.
The main goal of this is to store larger amounts of data (for example Open Api Specifications) outside k8s. The file is then identified via a reference string that is stored in the Custom resource and its contents can be retrieved if needed.

### Uploading a file
![about](docs/upload.drawio.svg)

The general upload flow is as follows:
1. Any domain can access the FM to store a file, if configured to do so by calling the upload endpoint.
2. In the request, the domain will send a reference **fileId**, which is a unique identifier for the file, the file contents, along with some metadata like the file content type and checksum. If the file content type is not specified, it will be assumed by the file manager and a flag will be added in the metadata that the content type was detected.
3. The file manager will then store the file in the configured backend with some metadata like the files content type and checksum.
4. The file manager will return the fileId to the calling domain.
5. The domain operators will then use this reference (fileId) in their Custom-Resources (CRs) instead of the actual file's content.

### Downloading a file
![about](docs/download.drawio.svg)

The general download flow is as follows:
1. Any domain can access the FM to retrieve a file, if configured to do so by calling the download endpoint.
2. In the request, the domain will send a reference **fileId**, which is a unique identifier for the file.
3. The file manager will then retrieve the file from the configured backend along with its metadata.
4. The file manager will return the fileId, its content and metadata to the calling domain.
5. The domain operators will then use this reference (fileId) and the metadata to access the original file's contents.

### Deleting a file
![about](docs/delete.drawio.svg)

The general delete flow is as follows:
1. Any domain can access the FM to delete a file, if configured to do so by calling the delete endpoint.
2. In the request, the domain will send a reference **fileId**, which is a unique identifier for the file to be deleted.
3. The file manager will validate the fileId format and convert it to the appropriate path format.
4. The file manager will then delete the file from the configured backend.
5. The file manager will return a success response (HTTP 204 No Content) to the calling domain.
6. The file manager will return a not found response (HTTP 404 Not found) if the file does not exist. If applicable, the operation can still be treated as successful since the desired state (file not existing) is achieved.

## Architecture

The following diagram provides a high-level overview of how the FM is integrated into the Controlplane.

![Architecture Diagram](docs/overview.drawio.svg)

## Backends

The FM itself does not store anything. This is done using backend implementations.
Currently, the FM supports the following backends.

### Amazon S3

This backend uses [Amazon S3](https://aws.amazon.com/s3/) to store files. As long as you are able to connect to S3, this backend is available to you.
As this is using a 3rd party storage solution the costs need to be considered.

```yaml
backend:
  type: buckets
  endpoint: s3.eu-central-1.amazonaws.com
  bucket_name: my-bucket
  sts_endpoint: https://sts.amazonaws.com
  role_arn: arn:aws:iam::12345:role/my-sample-role
```

### MinIO

This backend uses [MinIO](https://min.io/) to store files. MinIO is an open-source, self-hosted object storage solution that is compatible with the Amazon S3 API.

```bash
helm repo add minio https://charts.min.io/
helm repo update

kubectl create namespace minio
helm install minio minio/minio -n minio 
```

```yaml
backend:
  type: buckets
  endpoint: minio.minio.svc.cluster.local:9000
  bucket_name: my-bucket
  access_key: myAccessKey # Copy these from MinIO console
  secret_key: mySecret
```

## Security

### Network Policies

Additionally, traffic towards the FM is further protected by [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/),
so that only the services that are registered in advance can access the FM.
See the [Deployment Integration](#deployment-integration) section for more information on how to integrate the FM into your custom operator deployment.

## Code Integration
We've included an [OpenAPI spec](./api/openapi.yaml) that can be used to generate client code for the FM.

However, we also provide a basic Go implementation that can be used to **easily** integrate the FM into your code.
Please take a look at that [api/README.md](./api/README.md) for more information on how to use it.

## Deployment Integration

To integrate the following [Deployment and Namespaces Patches](./config/patches) into your custom operator deployment, so that the new operator can communicate with the FM.
Otherwise, the communication to the FM will be blocked on a [network policy](https://kubernetes.io/docs/concepts/services-networking/network-policies/) level in k8s.

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/).    
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.    
You can find a guide for developers at https://telekom.github.io/reuse-template/.