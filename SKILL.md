---
name: Restruct
description: Guide for using the Restruct package for struct-based routing and rendering.
---

# Restruct Package Guide

Restruct is a Go package that provides struct-based routing, automatic parameter extraction, and a view engine with template support. It relies on reflection to map struct methods to HTTP routes and assumes a specific project structure.

## Core Concepts

- **Service**: A struct that represents a group of related routes.
- **Method**: A method on a Service struct that handles a specific HTTP request.
- **View**: A component that handles rendering HTML templates from a filesystem.
- **Handler**: The central component that manages services, middleware, and request/response lifecycles.

## Routing

Restruct maps struct methods to HTTP routes using naming conventions and struct tags.

### Struct Tags
Use the `route` tag on struct fields to define the base path for a service.
- `route:"users"` -> Maps the struct to `/users`
- `route:"-"` -> Ignores the field.
- `route:""` -> Uses the field name (kebab-cased) as the path.

### Method Naming Conventions
Method names are automatically converted to route paths:
- `CamelCase` -> `camel-case`
- `Snake_Case` -> `/` separator
- `Index` -> Root of the service (`/`)
- `Any` -> Catch-all wildcard `/{any*}`
- `_0`, `_1`, etc. -> Path parameters `/{0}`, `/{1}`

**Examples:**
- `func (s *User) CreateUser()` -> POST `.../create-user` (if `CreateUser` is in `Routes()` as POST)
- `func (s *Service) Hello_World()` -> `.../hello/world`
- `func (s *Service) Item_0()` -> `.../item/{0}`

### Explicit Routing (`Routes` method)
Implement the `Routes()` method on your struct to explicitly define routes, methods, and middleware.

```go
func (s *User) Routes() []rs.Route {
    return []rs.Route{
        {Handler: "Create", Path: "/", Methods: []string{http.MethodPost}},
        {Handler: "Get", Path: "{id}", Methods: []string{http.MethodGet}},
    }
}
```

## Handlers

Handlers are methods on your service structs. Restruct supports dependency injection for handler arguments.

### Signatures
Common arguments injected automatically:
- `*http.Request`
- `http.ResponseWriter`
- `context.Context`
- Struct pointers (for request body binding)

**Return Values:**
- `error`: Returns an error response (handled by `ErrorHandler`).
- `any` / `interface{}`: Serialized to JSON by default.
- `*rs.Render`: Triggers HTML template rendering.
- `*rs.Response`: Manual control over status, content-type, and body.

### Request Binding
To bind request data (JSON body, query params, etc.) to a struct, simply add the struct pointer as an argument to your handler.

```go
type CreateRequest struct {
    Name string `json:"name"`
}

func (s *Service) Create(ctx context.Context, req *CreateRequest) any {
    // req is populated automatically
    return req
}
```

To access path parameters:
```go
rs.Vars(ctx)["id"] // or rs.Vars(ctx)["0"] for auto-generated numeric params
```

## Views & Rendering

The `rs.View` struct implements `rs.ResponseWriter` and handles template rendering.

### Setup
```go
view := &rs.View{
    FS:      publicFS,              // embed.FS or os.DirFS
    Layouts: []string{"layout/*.html"}, // Glob patterns for layouts
    Error:   "error.html",          // Template for errors
    Data:    func(r *http.Request) map[string]any { ... }, // Global data injector
}
```

### Auto-Routing
If `FS` implements `fs.ReadDirFS`, `rs.View` will automatically register routes for `.html` and `.tmpl` files.
- `public/index.html` -> `/`
- `public/about.html` -> `/about`

### Explicit Rendering
Return `*rs.Render` from a handler to render a specific template.

```go
return &rs.Render{
    Path: "user/profile.html",
    Data: map[string]any{"User": user},
}
```

## Middleware

Middleware is a standard `func(http.Handler) http.Handler`.

- **Global**: `h.Use(middleware)` in `Init`.
- **Service-level**: Implement `Middlewares() []rs.Middleware`.
- **Route-level**: Specify `Middlewares` in the `Routes()` return.

## Example Usage

```go
package main

import (
    "net/http"
    rs "github.com/altlimit/restruct"
)

type API struct {
    User User `route:"users"`
}

type User struct {}

func (u *User) Routes() []rs.Route {
    return []rs.Route{
        {Handler: "Get", Path: "{id}", Methods: []string{"GET"}},
    }
}

func (u *User) Get(ctx context.Context) any {
    id := rs.Vars(ctx)["id"]
    return map[string]string{"id": id}
}

func main() {
    rs.Handle("/", &API{})
    http.ListenAndServe(":8080", nil)
}
```
