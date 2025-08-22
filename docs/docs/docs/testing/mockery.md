---
sidebar_position: 2
---

# Mockery

[Mockery](https://github.com/vektra/mockery) is a mock code generation tool used in the Control Plane to generate mocks for interfaces.

## Overview

Mockery automatically generates mock implementations of Go interfaces to use in tests. This helps:

- Isolate components during testing
- Control behavior of dependencies
- Verify interactions with dependencies

## Integration in the Control Plane

The Control Plane uses Mockery to generate mocks for various interfaces:

- Backend interfaces (FileUploader, FileDownloader)
- Client interfaces (MinioClient)
- Service interfaces

## Directory Structure

Mocks in the Control Plane follow a consistent pattern:

```
pkg/
  backend/
    interface.go      # Contains the interface definitions
    mock/
      mock_backend.go # Generated mock implementations
```

## Code Generation

Mocks are generated using the Mockery CLI tool. Configuration is stored in `mockery.yaml` files:

```yaml
with-expecter: true
packages:
  github.com/telekom/controlplane/file-manager/pkg/backend:
    interfaces:
      FileUploader:
        config:
          dir: "pkg/backend/mock"
      FileDownloader:
        config:
          dir: "pkg/backend/mock"
```

The `tools/generate.go` file ensures consistent mock generation:

```go
//go:generate mockery --dir ../pkg/backend --name FileUploader --output ../pkg/backend/mock
//go:generate mockery --dir ../pkg/backend --name FileDownloader --output ../pkg/backend/mock
```

## Using Mocks in Tests

Mocks are used extensively in the Control Plane test suite:

```go
func TestUploadController(t *testing.T) {
    // Create a mock uploader
    mockUploader := mocks.NewFileUploader(t)
    
    // Set expectations
    mockUploader.On("UploadFile", mock.Anything, "test-id", mock.Anything).
        Return(nil).
        Once()
    
    // Create controller with mock dependency
    controller := NewUploadController(mockUploader)
    
    // Call the controller method
    err := controller.HandleUpload(context.Background(), "test-id", strings.NewReader("test content"))
    
    // Assertions
    assert.NoError(t, err)
    
    // Verify all expectations were met
    mockUploader.AssertExpectations(t)
}
```

## Best Practices Used in Control Plane

- Generate mocks for all public interfaces
- Use `mock.Anything` for arguments that don't need specific matching
- Verify mock expectations in tests
- Use `Once()`, `Times(n)` to verify call counts