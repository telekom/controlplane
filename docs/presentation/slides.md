---
theme: seriph
background: none
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
colorSchema: dark
themeConfig:
  primary: '#E20074'  # Telekom magenta
css: unocss
monaco: false  # set to true to enable Monaco editor
---

<style>
/* Add the platform element directly in the slides for simplicity */
.slidev-page::before {
  content: "";
  position: absolute;
  top: -100px;
  left: -100px;
  width: 300px;
  height: 300px;
  background: radial-gradient(circle, rgba(226, 0, 116, 0.1) 0%, rgba(226, 0, 116, 0) 70%);
  border-radius: 100%;
  z-index: 0;
  pointer-events: none;
}

.logo-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  margin-bottom: 2rem;
}

.logo {
  width: 180px;
  height: auto;
  margin-bottom: 1rem;
}

/* Architecture diagram scaling */
.slidev-layout[layout="image-right"] .slidev-image-container {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}

.slidev-layout[layout="image-right"] .slidev-image {
  max-width: 100%;
  max-height: 90%;
  object-fit: contain;
}
</style>

<div class="logo-container">
  <img src="/images/otip-logo.svg" alt="OTIP Logo" class="logo" />
</div>

# Controlplane Technical Framework Overview
## Technical Stack & Architecture

---
layout: image-right
image: './images/architecture.drawio.svg'
---

# Architecture Overview

The Control Plane is the central management layer for the Open Telekom Integration Platform.

<div class="glass-card">

- **Kubernetes Operators** - Custom controllers for complex domain apps
- **API Servers** - RESTful interfaces for k8s resources
- **Libraries** - Shared code modules

</div>

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

<div class="glass-card">
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
</div>

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
layout: center
class: text-center
---

# Testing Frameworks

<div class="glass-card">
<v-clicks>

- **Testify** - Rich assertion library
- **Mockery** - Auto-generated mocks
- **Controller-Runtime Test Environment** - K8s API testing

</v-clicks>
</div>

---

# Authentication & Security

<div class="glass-card">
<v-clicks>

- **JWT**
  - Bearer token authentication
  - Role-based access control

- **Middleware**
  - Gofiber JWT middleware
  - Request validation

</v-clicks>
</div>

---

# Infrastructure & Deployment

<div class="glass-card">
<v-clicks>

- **Kubernetes Native**
  - Deployments & Services
  - ConfigMaps for configuration
  - Network policies for security

- **Helm Charts**
  - Templated deployments
  - Environment configuration

</v-clicks>
</div>

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
layout: center
class: text-center
---

# Thank You

<div class="glass-card">
Explore the detailed documentation for each component
</div>