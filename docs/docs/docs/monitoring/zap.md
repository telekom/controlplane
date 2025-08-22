---
sidebar_position: 1
---

# Zap Logger

The Control Plane uses Uber's [Zap](https://github.com/uber-go/zap) library for structured, high-performance logging.

## Overview

Zap is a fast, structured logging library for Go, designed with performance in mind. The Control Plane uses Zap for:

- Structured JSON logging
- Contextual log fields
- Hierarchical logging
- Performance-critical logging needs

## Integration with Kubernetes

The Control Plane integrates Zap with the Kubernetes [logr](https://github.com/go-logr/logr) interface using [zapr](https://github.com/go-logr/zapr):

```go
import (
	"go.uber.org/zap"
	"github.com/go-logr/zapr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func SetupLogging() {
	// Create a Zap logger
	zapLog, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("Failed to init zap logger: %v", err))
	}
	defer zapLog.Sync()
	
	// Use zapr to convert the Zap logger to a logr.Logger
	logger := zapr.NewLogger(zapLog)
	
	// Set the logr.Logger as the global logger
	log.SetLogger(logger)
}
```

## Log Levels

The Control Plane uses different log levels for different environments:

- **Production**: Info level and above
- **Development**: Debug level and above

```go
func getLogger(env string) *zap.Logger {
	var logger *zap.Logger
	var err error
	
	switch env {
	case "production":
		logger, err = zap.NewProduction()
	case "development":
		logger, err = zap.NewDevelopment()
	default:
		config := zap.NewProductionConfig()
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
		logger, err = config.Build()
	}
	
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize zap logger: %v", err))
	}
	
	return logger
}
```

## Structured Logging

Zap's structured logging capabilities are used throughout the Control Plane:

```go
func (c *Controller) ProcessRequest(ctx context.Context, req *Request) error {
	logger := log.FromContext(ctx).WithValues(
		"requestID", req.ID,
		"userID", req.UserID,
		"operation", req.Operation,
	)
	
	logger.Info("Processing request")
	
	// Business logic...
	
	if err != nil {
		logger.Error("Failed to process request", "error", err)
		return err
	}
	
	logger.Info("Request processed successfully")
	return nil
}
```

## Performance Considerations

The Control Plane uses Zap's performance optimizations:

- Pre-allocated fields
- Minimal allocations
- Fast marshaling
- Log level checking

```go
// Pre-allocated fields
var baseLogger = zap.NewNop().With(
	zap.String("service", "file-manager"),
	zap.String("version", "1.0.0"),
)

func getContextLogger(ctx context.Context) *zap.Logger {
	if requestID, ok := ctx.Value("requestID").(string); ok {
		return baseLogger.With(zap.String("requestID", requestID))
	}
	return baseLogger
}
```

## Logging Best Practices

The Control Plane follows these logging best practices:

1. **Log Levels**
   - `Debug`: Fine-grained development information
   - `Info`: General operational events
   - `Warning`: Non-critical issues
   - `Error`: Runtime errors that require attention
   - `Fatal`: Critical errors causing application shutdown

2. **Contextual Information**
   - Include request IDs
   - Add user information when available
   - Include resource identifiers
   - Add operation names

3. **Standardized Format**
   - JSON format for machine parsing
   - Consistent field names
   - ISO 8601 timestamps
   - Hierarchical logger names