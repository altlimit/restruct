![Run Tests](https://github.com/altlimit/restruct/actions/workflows/run-tests.yaml/badge.svg) [![Go Reference](https://pkg.go.dev/badge/github.com/altlimit/restruct.svg)](https://pkg.go.dev/github.com/altlimit/restruct)

# restruct

**restruct** is a high-performance, service-oriented REST framework for Go. It allows you to build APIs by simply defining structs and methods, automating routing, parameter binding, validation, and view rendering.

---

* [Key Features](#key-features)
* [Install](#install)
* [Quick Start](#quick-start)
* [Core Concepts](#core-concepts)
    * [Services & Methods](#services--methods)
    * [Routing & Parameters](#routing--parameters)
    * [Nested Services](#nested-services)
* [Request & Response](#request--response)
    * [Binding & Validation](#binding--validation)
    * [Response Writers](#response-writers)
* [Views & Asset Serving](#views--asset-serving)
    * [Viewer Interface](#viewer-interface)
    * [Using embed.FS](#using-embedfs)
* [Middleware](#middleware)
* [Benchmarks](#benchmarks)
* [License](#license)

---

## Key Features

*   **Struct-Based Routing**: Exported methods are automatically mapped to routes.
*   **Hierarchical Services**: Nest structs to create path hierarchies (e.g., `/api/v1/...`).
*   **Smart Binding**: Auto-bind JSON, Form, and Query parameters to struct arguments.
*   **Interface Driven**: Customize behavior via `Router`, `Viewer`, `Init`, and `Middlewares` interfaces.
*   **View Engine**: Integrated minimal view engine with support for `fs.FS` and error page fallbacks.
*   **Zero-Boilerplate**: Focus on business logic, let the framework handle the plumbing.

## Install

```sh
go get github.com/altlimit/restruct
```

## Quick Start

```go
package main

import (
	"net/http"
	"github.com/altlimit/restruct"
)

type Calculator struct{}

// POST /add
// Payload: {"a": 10, "b": 20}
func (c *Calculator) Add(req struct {
	A int `json:"a"`
	B int `json:"b"`
}) int {
	return req.A + req.B
}

func main() {
	restruct.Handle("/", &Calculator{})
	http.ListenAndServe(":8080", nil)
}
```

## Core Concepts

### Services & Methods

Any exported struct can be a service. Public methods become endpoints.

*   `func (s *Svc) Index()` -> `/`
*   `func (s *Svc) Users()` -> `/users`
*   `func (s *Svc) Users_0()` -> `/users/{0}`
*   `func (s *Svc) UsersExport()` -> `/users-export`
*   `func (s *Svc) Any()` -> `/{any*}` (Wildcard catch-all)

### Routing & Parameters

You can define path parameters and wildcards using special method naming conventions or the `Router` interface.

**Method Naming**:
*   `Find_0_User` -> `/find/{blobID}/user` (where 0 maps to the first variable)
*   `Files_Any` -> `/files/{any*}` (Wildcard catch-all)

**Explicit Routing (`Router` Interface)**:
Recommended for cleaner method names.

```go
func (s *Service) Routes() []restruct.Route {
    return []restruct.Route{
        {Handler: "Download", Path: "files/{id}", Methods: []string{"GET"}},
        {Handler: "Upload", Path: "files", Methods: []string{"POST"}},
    }
}
```

Path parameters can be accessed via `restruct.Params(r)["id"]`.

### Nested Services

Services can be nested to create API versions or groups.

```go
type Server struct {
    ApiV1 V1 `route:"api/v1"` // Mounts V1 at /api/v1
    Admin AdminService `route:"admin"`
}
```

## Request & Response

### Binding & Validation

`restruct` binds request data (JSON body, Form data, Query params) to method arguments automatically. You can extend the default binder to add validation (e.g., using `go-playground/validator`).

```go
type CreateRequest struct {
    Email string `json:"email" validate:"required,email"`
}

func (s *Service) Create(r *http.Request, req CreateRequest) error {
    // req is already bound and valid (if custom binder set up)
    return nil
}
```

### Response Writers

Handlers can return:
*   `struct` / `map` / `slice`: Encoded as JSON by default.
*   `string` / `[]byte`: Sent as raw response.
*   `error`: Converted to appropriate HTTP error status.
*   `*restruct.Response`: Complete control over Status, Headers, and Content.
*   `*restruct.Render`: Force rendering a specific template path (see [Explicit Template Rendering](#explicit-template-rendering)).

## Views & Asset Serving

### Viewer Interface

Implement the `Viewer` interface to enable HTML rendering and asset serving for your service. This isolates views to specific services.

```go
func (s *MyService) View() *restruct.View {
    return &restruct.View{
        FS:    publicFS, // fs.FS interface
        Error: "error.html", // Validation/Error template
    }
}
```

*   If a method returns a struct/map, `restruct` first checks for a matching template (e.g., `index.html` for `Index` method).
*   If no template is found, it falls back to the configured `Error` template (if `Any` route matched) or JSON/DefaultWriter (if specific route matched).

### Using embed.FS

You can easily serve embedded assets.

```go
//go:embed public
var publicFS embed.FS

func (s *Server) View() *restruct.View {
    sub, _ := fs.Sub(publicFS, "public")
    return &restruct.View{
        FS: sub,
        // ...
    }
}
```

### Explicit Template Rendering

Use `*restruct.Render` to force rendering a specific template, regardless of the request URL:

```go
func (s *Service) Dashboard(ctx context.Context) (*restruct.Render, error) {
    userID := restruct.Vars(ctx)["id"]
    if userID == "" {
        return nil, errors.New("user not found")
    }
    return &restruct.Render{
        Path: "dashboard/main.html",
        Data: map[string]interface{}{
            "UserID": userID,
            "Title":  "Dashboard",
        },
    }, nil
}
```

*   `Path`: Template path relative to the View's FS root.
*   `Data`: Data passed to the template (accessible as `.Data` or merged if map).

## Middleware

Middleware can be applied globally, per-service, or per-route.

```go
func (s *Service) Init(h *restruct.Handler) {
    h.Use(loggingMiddleware)
    h.Use(authMiddleware)
}
```

## Benchmarks

High performance with minimal overhead. (See `bench_test.go` for latest results).

## License

MIT