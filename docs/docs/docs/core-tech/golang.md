---
sidebar_position: 1
---

# Go Language

The Control Plane is primarily implemented in Go version 1.24.4, taking advantage of the language's concurrency features, strong typing, and excellent standard library.

## Why Go?

Go was chosen for the Control Plane implementation for several reasons:

- **Cloud-native ecosystem** - Go is the de-facto standard for Kubernetes and cloud-native applications
- **Concurrency model** - Goroutines and channels provide efficient handling of concurrent operations
- **Static typing** - Catches errors at compile time
- **Cross-compilation** - Easy to build for different platforms
- **Rich standard library** - Reduces dependency on third-party packages
- **Built-in testing** - Native support for unit tests and benchmarking

## Key Go Patterns Used

### Context Propagation

The Control Plane extensively uses Go's context package for propagating deadlines, cancellation signals, and request-scoped values.

```go
func (c *Controller) ProcessFile(ctx context.Context, fileID string) error {
    // Context is passed through the call stack
    logger := log.FromContext(ctx).WithValues("fileID", fileID)
    
    // Create child context with timeout
    ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    
    return c.backend.ProcessFile(ctxWithTimeout, fileID)
}
```

### Error Handling

We follow idiomatic Go error handling patterns:

```go
func (s *service) UploadFile(ctx context.Context, fileID string, content []byte) error {
    if fileID == "" {
        return errors.New("fileID cannot be empty")
    }
    
    err := s.validateFileContent(content)
    if err != nil {
        return fmt.Errorf("invalid file content: %w", err)
    }
    
    return s.storage.Store(ctx, fileID, content)
}
```

### Interfaces

Interfaces are defined by consumers rather than implementers, following Go's interface philosophy:

```go
// Defined by the consumer
type FileStorage interface {
    Store(ctx context.Context, id string, content []byte) error
    Retrieve(ctx context.Context, id string) ([]byte, error)
}

// Implemented elsewhere
type S3Storage struct {
    client *minio.Client
    bucket string
}

func (s *S3Storage) Store(ctx context.Context, id string, content []byte) error {
    // Implementation
}
```

## Go Modules and Dependencies

The Control Plane uses Go modules for dependency management. Key dependencies include:

- **sigs.k8s.io/controller-runtime** - Framework for building Kubernetes operators
- **github.com/gofiber/fiber/v2** - Fast HTTP framework
- **github.com/minio/minio-go/v7** - S3-compatible storage client
- **github.com/stretchr/testify** - Testing utilities
- **go.uber.org/zap** - Structured logging