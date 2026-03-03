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
* [Views & Template Rendering](#views--template-rendering)
    * [Writer Interface](#writer-interface)
    * [Using embed.FS](#using-embedfs)
    * [Explicit Template Rendering](#explicit-template-rendering)
* [Middleware](#middleware)
* [Context Values](#context-values)
* [Benchmarks](#benchmarks)
* [License](#license)

---

## Key Features

*   **Struct-Based Routing**: Exported methods are automatically mapped to routes via a Trie-based router.
*   **Hierarchical Services**: Nest structs to create path hierarchies (e.g., `/api/v1/...`).
*   **Smart Binding**: Auto-bind JSON, Form, Query, and Multipart parameters to struct arguments.
*   **Interface Driven**: Customize behavior via `Router`, `Writer`, `Init`, and `Middlewares` interfaces.
*   **View Engine**: Integrated template engine with `fs.FS` support, layout templates, and error page fallbacks.
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
*   `func (s *Svc) Files_Any()` -> `/files/{any*}` (Scoped wildcard)

### Routing & Parameters

You can define path parameters and wildcards using special method naming conventions or the `Router` interface.

**Method Naming**:
*   `Find_0_User` -> `/find/{0}/user` (path parameter)
*   `Files_Any` -> `/files/{any*}` (Wildcard catch-all)

**Explicit Routing (`Router` Interface)**:
Recommended for cleaner method names and explicit HTTP method restrictions.

`Handler` accepts a **string** (method name) or a **func** (used directly):

```go
func (u *User) Routes() []restruct.Route {
    return []restruct.Route{
        {Handler: u.CreateUser, Path: ".", Methods: []string{"POST"}},
        {Handler: "ReadUser", Path: "{id}", Methods: []string{"GET"}},
        {Handler: "UpdateUser", Path: "{id}", Methods: []string{"PUT"}},
        {Handler: "DeleteUser", Path: "{id}", Methods: []string{"DELETE"}},
    }
}
```

*   `Handler: "MethodName"` — Resolves to the struct method by name.
*   `Handler: u.MethodName` or `Handler: myFunc` — Uses the func directly.
*   `Path: "."` maps to the service root path (useful for CRUD on collection endpoints).
*   Omitting `Path` uses the default naming convention.
*   Omitting `Methods` allows all HTTP methods.
*   `Middlewares` on a Route applies per-route middleware.

Path parameters can be accessed via `restruct.Params(r)["id"]` or `restruct.Vars(ctx)["id"]`.

### Nested Services

Services can be nested to create API versions or groups.

```go
type Server struct {
    ApiV1 V1 `route:"api/v1"` // Mounts V1 at /api/v1
    Admin AdminService `route:"admin"`
    DB    struct{} `route:"-"` // Ignored
}
```

## Request & Response

### Binding & Validation

`restruct` binds request data (JSON body, Form data, Query params) to method arguments automatically via the `RequestReader` interface. The `DefaultReader` dispatches to the `Bind` function which supports:

*   `application/json` -> `BindJson` (uses `json` struct tag)
*   `application/x-www-form-urlencoded` / `multipart/form-data` -> `BindForm` (uses `form` struct tag)
*   Query parameters -> `BindQuery` (uses `query` struct tag)

You can extend the `DefaultReader` with a custom `Bind` function to add validation (e.g., using `go-playground/validator`):

```go
func (s *Server) Init(h *restruct.Handler) {
    h.Reader = &restruct.DefaultReader{
        Bind: func(r *http.Request, out interface{}, methods ...string) error {
            if err := restruct.Bind(r, out, methods...); err != nil {
                return err
            }
            return validate.Struct(out)
        },
    }
}
```

### Response Writers

The `ResponseWriter` interface controls how handler return values are written to the response.

Handlers can return:
*   `struct` / `map` / `slice`: Encoded as JSON by `DefaultWriter`.
*   `string` / `[]byte`: Sent as raw response.
*   `error`: Converted to appropriate HTTP error status.
*   `*restruct.Response`: Complete control over Status, Headers, ContentType, and Content bytes.
*   `*restruct.Json`: JSON response with a custom status code (e.g., `restruct.Json{Status: 201, Content: obj}`).
*   `*restruct.Render`: Force rendering a specific template path (see [Explicit Template Rendering](#explicit-template-rendering)).
*   `(int, any, error)`: Status code, response body, and error.
*   `(any, error)`: Response body with error handling.

The `DefaultWriter` supports:
*   `ErrorHandler func(error) any` — Custom error formatting. Return `*restruct.Response` for full control.
*   `Errors map[error]Error` — Map known errors to custom HTTP statuses/messages.
*   `EscapeJsonHtml bool` — Control HTML escaping in JSON output.

## Views & Template Rendering

### Writer Interface

Implement the `Writer` interface to enable HTML rendering and asset serving for your service. This associates a `ResponseWriter` (typically a `*View`) with the service.

```go
func (s *Server) Writer() restruct.ResponseWriter {
    return &restruct.View{
        FS:      publicFS,                         // fs.FS interface
        Funcs:   template.FuncMap{"upper": strings.ToUpper},
        Skips:   regexp.MustCompile("^layout"),    // Skip layout files from routing
        Layouts: []string{"layout/*.html"},        // Layout template patterns
        Error:   "error.html",                     // Error page template
        Data: func(r *http.Request) map[string]any {
            return map[string]any{"SiteName": "My Site"}
        },
    }
}
```

*   If a method returns a struct/map, `restruct` first checks for a matching template (e.g., `index.html` for `Index` method).
*   If no template is found, it delegates to the fallback `Writer` or falls back to JSON.
*   Templates receive `{{.Request}}`, path parameters, handler return data, and data from the `Data` callback.

### Using embed.FS

You can easily serve embedded assets.

```go
//go:embed public
var publicFS embed.FS

func (s *Server) Writer() restruct.ResponseWriter {
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
*   `Data`: Data passed to the template (merged if `map[string]any`, otherwise accessible as `{{.Data}}`).

## Middleware

Middleware can be applied globally, per-service, or per-route using the standard `func(http.Handler) http.Handler` signature.

```go
func (s *Server) Init(h *restruct.Handler) {
    h.Use(restruct.Recovery) // Built-in panic recovery middleware
    h.Use(loggingMiddleware)
}
```

**Per-service middleware** via the `Middlewares` interface:
```go
func (b *Blob) Middlewares() []restruct.Middleware {
    return []restruct.Middleware{loggerMiddleware}
}
```

**Per-route middleware** via the Route struct:
```go
{Handler: "Upload", Middlewares: []restruct.Middleware{authMiddleware}}
```

**Built-in**: `restruct.Recovery` — Panic recovery middleware that logs the stack trace and returns a 500 error.

## Context Values

Store and retrieve values in the request context (useful for passing data from middleware to handlers):

```go
// Using *http.Request
r = restruct.SetValue(r, "userID", int64(1))
userID := restruct.GetValue(r, "userID").(int64)
allVals := restruct.GetValues(r) // map[string]interface{}

// Using context.Context
ctx = restruct.SetVal(ctx, "key", value)
val := restruct.GetVal(ctx, "key")
allVals := restruct.GetVals(ctx) // map[string]interface{}
```

## Handler Setup

```go
// Register on http.DefaultServeMux
restruct.Handle("/api/", &MyService{})

// Or create a Handler for use with custom routers
h := restruct.NewHandler(&MyService{})
h.WithPrefix("/api/")
h.AddService("extra/", &ExtraService{})

// List all registered routes (useful for debugging)
for _, route := range h.Routes() {
    fmt.Println(route)
}
```

## Benchmarks

High performance with minimal overhead. (See `bench_test.go` for latest results).

## License

MIT