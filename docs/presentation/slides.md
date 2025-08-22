---
theme: default
background: https://source.unsplash.com/collection/94734566/1920x1080
class: text-center
highlighter: shiki
lineNumbers: false
info: |
  ## Controlplane Technical Framework Overview
  Presentation for the Open Telekom Integration Platform
drawings:
  persist: false
transition: slide-left
title: Controlplane Technical Framework Overview
---

# Controlplane Technical Framework Overview
## Technical Stack & Architecture

---
layout: image-right
image: './images/architecture.drawio.svg'
---

# Architecture Overview

The Control Plane is the central management layer for the Open Telekom Integration Platform.

- **Kubernetes Operators** - Custom controllers for complex domain apps
- **API Servers** - RESTful interfaces for k8s resources
- **Libraries** - Shared code modules

---
layout: two-cols
---

# Core Technologies

<v-clicks>

- **Go** (v1.24.4)
  - Modern, concurrent language
  - Strong standard library
  - Built-in testing support

- **Kubernetes Operators**
  - controller-runtime v0.21.0
  - Custom resource definitions

- **Kubebuilder**
  - Framework for building operators
  - Scaffolding and code generation

</v-clicks>

::right::

```go
// Example controller pattern
func (r *FileManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("filemanager", req.NamespacedName)
	
	// Fetch the FileManager resource
	fileManager := &apiV1.FileManager{}
	if err := r.Get(ctx, req.NamespacedName, fileManager); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Business logic here
	// ...

	return ctrl.Result{}, nil
}
```

---

# Web & API Frameworks

<v-clicks>

- **Gofiber** (v2.52.8)
  - Express-inspired web framework
  - High-performance & low memory footprint
  - Middleware support

- **OpenAPI/Swagger**
  - API specifications in YAML
  - Code generation for clients/servers
  - Self-documenting APIs

</v-clicks>

---
layout: two-cols
---

# Storage & Backend

<v-clicks>

- **MinIO S3 Client**
  - S3-compatible object storage
  - Secure credential management
  - Bucket and object operations

- **Backend Interfaces**
  - Clean separation of concerns
  - Implementation agnostic
  - Testable design

</v-clicks>

::right::

```go
// MinIO S3 client for object storage
client, err := minio.New(endpoint, &minio.Options{
    Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
    Secure: useSSL,
})

// File-Manager Backend Interface
type FileUploader interface {
    UploadFile(ctx context.Context, fileID string, 
               file io.Reader) error
}

type FileDownloader interface {
    DownloadFile(ctx context.Context, fileID string) 
               (io.ReadCloser, error)
}
```

---
layout: statement
---

# Testing Frameworks

<v-clicks>

- **Testify** - Rich assertion library
- **Mockery** - Auto-generated mocks
- **Controller-Runtime Test Environment** - K8s API testing

</v-clicks>

---

# Authentication & Security

<v-clicks>

- **JWT**
  - Bearer token authentication
  - Role-based access control

- **Middleware**
  - Gofiber JWT middleware
  - Request validation

</v-clicks>

---

# Infrastructure & Deployment

<v-clicks>

- **Kubernetes Native**
  - Deployments & Services
  - ConfigMaps for configuration
  - Network policies for security

- **Helm Charts**
  - Templated deployments
  - Environment configuration

</v-clicks>

---
layout: two-cols
---

# Logging & Monitoring

<v-clicks>

- **Zap Logger**
  - Structured, performant logging
  - Log levels & contextual fields

- **Prometheus Metrics**
  - Custom metrics for services
  - Service monitors
  - Integration with Grafana

</v-clicks>

::right::

```go
// Logger setup
zapLog, err := zap.NewProduction()
defer zapLog.Sync()
log := zapr.NewLogger(zapLog)
```

---
layout: end
---

# Thank You

Explore the detailed documentation for each component