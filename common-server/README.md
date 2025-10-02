<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

<p align="center">
  <img src="./docs/icon.jpeg" alt="Common Server logo" width="200">
  <h1 align="center">Common Server</h1>
</p>

<p align="center">
  The Common Server is a library for building HTTP servers in Go. 
  It provides a set of common components and utilities for building HTTP servers for Kubernetes resources.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#standalone-server">Standalone Server</a> •
  <a href="#library">Library</a> •
  <a href="#getting-started">Getting Started</a> •
  <a href="#known-issues">Known Issues</a> •
</p>


## About

This module provides shared functionality for all HTTP-Servers. It can either be used as a [standalone server](#standalone-server) or as a [library](#library).

It contains the following components:

- **Server**: The server implements the HTTP server. It contains common logic for all servers, such as middlewares. See [server](#server) for more information.
- **Controller**: The controller implements the logic for the server. It contains the business logic and handles the requests.See [controller](#controller) for more information.
- **Store**: The store is an abstraction for the Kubernetes API. It provides a common interface for basic CRUD operations on Kubernetes resources. See [store](#store) for more information.


<div align="center">
    <img src="docs/overview.drawio.svg" />
</div>


## Standalone Server

In this scenario the default containerimage is used and configured using a yaml file. It can be deployed using the provided [helm-chart](./helm/Chart.yaml).
For an example config see [default.yaml](./config/default.yaml) and [values.yaml](./helm/values.yaml#L90).

## Library

In this scenario the server is used as a library and the logic is implemented in the application itself.

### Server

The server is a generic HTTP server built on top of [fiber](https://github.com/gofiber/fiber). 
It provides a set of common middlewares and utilities for building HTTP servers.

### Controller

The controllers are used to define common logic for the server.
They implement the `Controller` interface.

- **Resource**: Provides a CRUD interface for Kubernetes resources. It uses a [Store](#Store) to access the resources. It also supports filtering, sorting and pagination.
- **Predefined**: Provides a similar set of functions as the `Resource` controller, but has predefined config that are always applied, e.g. filters.
- **Openapi**: All controllers can be used to generate an OpenAPI spec. This spec is then exposed using this controller.
- **Probes**: Basic [probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/) that can be extended with custom checks.


### Store

The store stores one kind of custom resources (CR). It implement the EventHandler interface and - per default - uses a `SharedIndexInformer` to watch for changes in the CRs, see [informer](./internal/informer/informer.go) for more information.
All CRs are stored in a [badger](https://github.com/hypermodeinc/badger) inmemory database. This enables fast lookup and filtering using json-paths.

## Security

The server is secured using [OAuth2](https://oauth.net/2/) and [OpenID Connect](https://openid.net/connect/).
It consists of the following layers:

1. **JWT**: This middleware checks the validity of the JWT token. It can be configured with different trusted issuers.
2. **Business Context**: This middleware extracts the business context from the JWT token and writes it into the request context. The business context is used to identify the user and the access-scope.
3. **Check Access**: This middleware uses the business context to check if the user has access to the requested resource. 
4. **Audit Logging**: This middleware logs all requests and responses with the business context.

### Security Templating

Templates are used to define dynamic access control rules based on the context of a request. 
They allow for flexible configuration of expected resource structures and user input patterns.

For reference, see the [Template-Example](#template-example).

If no values are applied, the default values are used. Keep in mind, that stating any configuration will overwrite this default. The default values are:
```yaml
security:
  templates:
    - scope: team
      expectedTemplate: "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}/"
      userInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}"
      matchType: prefix
    - scope: group
      expectedTemplate: "{{ .B.Environment }}--{{ .B.Group }}--"
      userInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}"
      matchType: prefix
    - scope: admin
      expectedTemplate: "{{ .B.Environment }}--"
      userInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}"
      matchType: prefix
```

#### Configuration

Templates are configured per client type (scope) (e.g., `team`, `group`, `admin`) and consist of the following components:
- **ExpectedTemplate**: Specifies the structure of the resource that the client is allowed to access. The structure is the expected structure of the resource in [Store](#Store).
- **UserInputTemplate**: Defines the structure of the resource being requested by the client.
- **MatchType**: Determines the matching behavior. The supported match types are:
  - `equal`: Checks if the expected template matches the user input template exactly.
  - `prefix`: Checks if the expected template is a prefix of the user input template.

These templates are defined in a map where each client type is associated with its respective configuration. 
The `MatchType` ensures that the comparison logic aligns with the desired access control rules.

Only defined scopes are supported. For example, if you do not define a scope `team` in the templating configuration, any request with the scope `team` will be rejected.



#### Behavior / Configuration Options

Templates are evaluated using a context that combines the business context (e.g., environment, group, team) and request parameters (e.g., namespace, name). 
Please note, that the placeholders in the template are case-sensitive.
 
- `.B` refers to the **Business Context** (i.e. the authentication token). This data is extracted from the JWT or other authentication mechanisms. 
  - `.B.Environment` refers to the environment (e.g., `dev`, `prod`).
  - `.B.Group` refers to the group (e.g., `telekom`).
  - `.B.Team` refers to the team (e.g., `team1`).
- `.P` refers to the **Path Parameters**, which are extracted from the request's URL. These parameters represent the user-provided input for the resource being accessed. The following are supported:
  - `.P.Namespace` refers to the namespace of the resource.
  - `.P.Name` refers to the name of the resource.

The evaluation process ensures that the access control logic is applied consistently across different client types and resource structures.

#### Custom Functions

Templates support custom functions to enhance their flexibility. These functions can be used for string manipulation and conditional logic. The following are currently supported functions:
- `lastPart`: Retrieves the last part of a string based on a specified delimiter. This is useful for extracting specific segments from a resource name.
- `contains`: Checks if a string contains a specified substring. This is useful for validating the presence of certain elements in the resource name.

Furthermore, take a look at the go package [text/template](https://pkg.go.dev/text/template). There, you can find a list of available default functions.

#### Use Cases

Templates can be tailored to various use cases, such as:
- Restricting access to specific namespaces or resources based on the client's role.
- Allowing hierarchical access control, where broader roles (e.g., admin) have access to more resources than narrower roles (e.g., team).
- Dynamically adjusting access rules based on request parameters or business context.

By configuring templates appropriately, you can implement fine-grained access control that aligns with your application's security requirements.

#### Template Example

```yaml
security:
  enabled: true
  defaultScope: "telekom:team:read"
  scopePrefix: "telekom:"
  templates:
  - expectedTemplate: "{{ .B.Environment }}/{{ .B.Group }}--{{ .B.Team }}"
    userInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}"
    scope: team
    matchType: equal
  - expectedTemplate: "{{ .B.Environment }}/{{ .B.Group }}{{ if contains .P.Name \"--\"}}--{{ lastPart .P.Name \"--\"}}{{ end }}"
    userInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}"
    scope: group
    matchType: equal
  - expectedTemplate: "{{ .B.Environment }}"
    userInputTemplate: "{{ .B.Environment }}"
    scope: admin
    matchType: prefix
```


## Getting Started

For any relevant commands, you can use the provided [Makefile](./Makefile) to build and test the project.

### Using the Server

You can use the server as a library. The server is a generic HTTP server and can be used to serve any kind of resource.
It provides a `Controller` interface that can be used to implement the logic for the server. 
There already are some default controllers implemented, see [controller](#controller) for more information.

```go
appCfg := server.NewAppConfig()
appCfg.EnableMetrics = true
app := server.NewAppWithConfig(appCfg)

app.Get("/hello", func(c *fiber.Ctx) error {
  return c.SendString("Hello, World!")
})

svr := server.NewServerWithApp(app)
svr.RegisterController(server.NewProbesController(), server.ControllerOpts{})

svr.Start(":8080")
// or
// serve.ServeTLS(context.Background(), app, ":8080", "tls.crt", "tls.key")
```

### Using the Store

You can initialize the store using the `inmemory` package. The store is a generic store for Kubernetes resources and can be used to store any kind of resource.
Here we are using the `*unstructured.Unstructured` type, which is a generic type for Kubernetes resources. This could be any type that implements [Object](./pkg/store/store.go#L181).

```go
dynamicClient := dynamic.NewForConfigOrDie(kubeCfg)
storeOpts := inmemory.StoreOpts{
    Client: dynamicClient,
    GVR:    gvr,
    GVK:    gvk,
}
inmemoryStore := inmemory.NewOrDie[*unstructured.Unstructured](ctx, storeOpts)

listRes, err := inmemoryStore.List(ctx, store.ListOpts{
    Filters: []store.Filter{
        {
            Path:  "metadata.name",
            Op:    store.OpEqual,
            Value: "test",
        },
    },
})
```

The store can also be sorted, see [sortable](./pkg/store/inmemory/sorted_store.go) for more information.


## Known Issues

- The server duplicates all resource in memory. This can lead to high memory usage if there are many resources. 
  (1) it uses a kubernetes informer to watch for changes in the resources and (2) it stores all resources in a badger database.
  However, it is possible to store the badger database on disk, see [StoreOpts](./pkg/store/inmemory/inmemory_store.go#L47) for more information. In the future, this could be improved by using only the badger database and not the informer or replacing the badger-database with an external database.