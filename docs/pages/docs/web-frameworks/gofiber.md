---
sidebar_position: 1
---

# Gofiber Web Framework

The Control Plane uses Gofiber v2.52.8 as its primary HTTP framework, particularly in the file-manager and secret-manager components.

## Introduction to Gofiber

[Gofiber](https://gofiber.io/) is an Express-inspired web framework for Go that's built on top of Fasthttp, the fastest HTTP engine for Go. It provides a robust set of features with minimal overhead and maximum performance.

## Why Gofiber?

Gofiber was chosen for several reasons:

- **Performance** - Built on Fasthttp, offering high throughput
- **Low memory footprint** - Efficient resource utilization
- **Middleware support** - Extensible middleware system
- **Express-like API** - Familiar for developers with Node.js experience
- **Route grouping** - Structured API organization

## Basic Usage Example

Here's a simplified example of how Gofiber is used in the Control Plane:

```go
func setupRoutes(app *fiber.App, controller *Controller) {
    // Group API routes
    api := app.Group("/api/v1")
    
    // Apply middleware to all routes in this group
    api.Use(middleware.JWT())
    api.Use(middleware.Logger())
    
    // Define routes
    api.Get("/files/:id", controller.GetFile)
    api.Put("/files/:id", controller.UploadFile)
}

// Handler function
func (c *Controller) GetFile(ctx *fiber.Ctx) error {
    fileID := ctx.Params("id")
    
    file, err := c.service.GetFile(ctx.Context(), fileID)
    if err != nil {
        return fiber.NewError(fiber.StatusNotFound, "File not found")
    }
    
    return ctx.Send(file)
}
```

## JWT Authentication

The Control Plane uses Gofiber's JWT middleware for authentication:

```go
import "github.com/gofiber/contrib/jwt"

func setupAuth(app *fiber.App) {
    app.Use(jwt.New(jwt.Config{
        SigningKey:    []byte(os.Getenv("JWT_SECRET")),
        ErrorHandler: jwtError,
    }))
}

func jwtError(c *fiber.Ctx, err error) error {
    return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
        "error": "Unauthorized",
        "message": "Invalid or expired JWT",
    })
}
```

## Custom Middleware

The project includes several custom middleware components built on Gofiber:

- Request logging with contextual information
- Metrics collection for Prometheus
- Panic recovery
- CORS handling
- Business context injection

## Integration with OpenAPI

The Gofiber routes are generated from OpenAPI specifications using code generators:

```go
// Generated server from OpenAPI spec
func RegisterHandlers(app *fiber.App, si ServerInterface) {
    app.Get("/v1/files/:fileId", func(c *fiber.Ctx) error {
        var err error
        
        // Parameter bindings
        var fileId string
        fileId = c.Params("fileId")
        
        // Call handler function
        err = si.DownloadFile(c, fileId)
        return err
    })
    
    app.Put("/v1/files/:fileId", func(c *fiber.Ctx) error {
        var err error
        
        // Parameter bindings
        var fileId string
        fileId = c.Params("fileId")
        
        // Call handler function
        err = si.UploadFile(c, fileId)
        return err
    })
}
```

## Error Handling

Gofiber provides a standardized way to handle errors:

```go
func errorHandler(ctx *fiber.Ctx, err error) error {
    // Default 500 status code
    code := fiber.StatusInternalServerError
    
    // Check if it's a fiber error
    if e, ok := err.(*fiber.Error); ok {
        code = e.Code
    }
    
    // Format as API Problem (RFC 7807)
    problem := &Problem{
        Type:   "https://example.org/problems/general-error",
        Title:  "An error occurred",
        Status: code,
        Detail: err.Error(),
    }
    
    return ctx.Status(code).JSON(problem)
}
```

## Performance Considerations

The Gofiber framework is configured for optimal performance:

```go
app := fiber.New(fiber.Config{
    // Pre-allocate memory for the server
    Prefork:               false,
    ServerHeader:          "Controlplane",
    StrictRouting:         true,
    CaseSensitive:         true,
    DisableStartupMessage: true,
    
    // JSON Configuration
    JSONEncoder: json.Marshal,
    JSONDecoder: json.Unmarshal,
    
    // Error Handler
    ErrorHandler: errorHandler,
})
```