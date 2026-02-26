---
name: Restruct
description: Guide for using the Restruct package for struct-based routing and rendering.
---

# Restruct Package Guide

Restruct is a Go package that provides struct-based routing, automatic parameter extraction, and a view engine with template support. It relies on reflection to map struct methods to HTTP routes and assumes a specific project structure.

## Core Concepts

- **Service**: A struct that represents a group of related routes.
- **Method**: A method on a Service struct that handles a specific HTTP request.
- **View**: A component that handles rendering HTML templates and serving static files from an `fs.FS`.
- **Handler**: The central component that manages services, middleware, routing (via a Trie-based router), and request/response lifecycles.

## Routing

Restruct maps struct methods to HTTP routes using naming conventions and struct tags.

### Struct Tags
Use the `route` tag on struct fields to define the base path for a nested service.
- `route:"users"` -> Maps the struct to `/users`
- `route:"-"` -> Ignores the field (won't be registered as a service).
- `route:""` -> Uses the field name (kebab-cased) as the path.

### Method Naming Conventions
Method names are automatically converted to route paths:
- `CamelCase` -> `camel-case`
- `Underscore_Separated` -> `/` separator (e.g., `Hello_World` -> `hello/world`)
- `Index` -> Root of the service (`/`)
- `Any` -> Catch-all wildcard `/{any*}`
- `Suffix_Any` -> Wildcard after path (e.g., `Files_Any` -> `files/{any*}`)
- `_0`, `_1`, etc. -> Path parameters `/{0}`, `/{1}` (e.g., `Item_0` -> `item/{0}`)

**Examples:**
- `func (s *Svc) CreateUser()` -> `.../create-user`
- `func (s *Svc) Hello_World()` -> `.../hello/world`
- `func (s *Svc) Item_0()` -> `.../item/{0}`
- `func (s *Svc) Any()` -> `.../{any*}`
- `func (s *Svc) Link_Any()` -> `.../link/{any*}`

### Explicit Routing (`Router` interface)
Implement the `Routes() []rs.Route` method on your struct to explicitly define routes, HTTP methods, and per-route middleware.

```go
func (u *User) Routes() []rs.Route {
    return []rs.Route{
        {Handler: "CreateUser", Path: ".", Methods: []string{http.MethodPost}},
        {Handler: "ReadUser", Path: "{id}", Methods: []string{http.MethodGet}},
        {Handler: "UpdateUser", Path: "{id}", Methods: []string{http.MethodPut}},
        {Handler: "DeleteUser", Path: "{id}", Methods: []string{http.MethodDelete}},
    }
}
```

- `Path: "."` maps to the service root (e.g., `POST /users` instead of `POST /users/create-user`).
- `Path: "{id}"` adds a parameter segment.
- Omitting `Path` uses the default naming convention for the handler method name.
- Omitting `Methods` allows all HTTP methods.
- `Middlewares` on a Route applies only to that specific route.

## Handlers

Handlers are methods on your service structs. Restruct supports dependency injection for handler arguments.

### Signatures
Common arguments injected automatically:
- `*http.Request`
- `http.ResponseWriter`
- `context.Context`
- Struct pointers or values (for request body binding via `RequestReader`)

**Return Values:**
- No return: Response is not written (you handle it via `http.ResponseWriter`).
- `error`: Returns an error response (handled by `ResponseWriter`).
- `any` / `interface{}`: Serialized to JSON by the `DefaultWriter`.
- `*rs.Render`: Triggers HTML template rendering at a specific path.
- `*rs.Response`: Manual control over status, content-type, headers, and body.
- `*rs.Json`: JSON response with a custom status code.
- `(int, any, error)`: Status code, response body, and error.
- `(any, error)`: Response body and error.

### Request Binding
To bind request data (JSON body, query params, form data) to a struct, add the struct pointer or value as an argument to your handler.

```go
type CreateRequest struct {
    Name string `json:"name"`
}

func (s *Service) Create(ctx context.Context, req *CreateRequest) any {
    // req is populated automatically via RequestReader
    return req
}
```

Inline struct arguments are also supported:
```go
func (c *Calculator) Add(req struct {
    A int `json:"a"`
    B int `json:"b"`
}) int {
    return req.A + req.B
}
```

### Path Parameters
Access path parameters via context:
```go
rs.Params(r)["id"]       // from *http.Request
rs.Vars(ctx)["id"]       // from context.Context
rs.Vars(ctx)["0"]        // for auto-generated numeric params (_0, _1, etc.)
rs.Vars(ctx)["any"]      // for wildcard catch-all routes
```

### Context Values
Store and retrieve arbitrary values from the request context (e.g., in middleware):
```go
// Using *http.Request (returns new *http.Request)
r = rs.SetValue(r, "userID", int64(1))
userID := rs.GetValue(r, "userID").(int64)
vals := rs.GetValues(r) // map[string]interface{}

// Using context.Context directly
ctx = rs.SetVal(ctx, "key", "value")
val := rs.GetVal(ctx, "key")
vals := rs.GetVals(ctx) // map[string]interface{}
```

## Views & Rendering

The `rs.View` struct implements `ResponseWriter` and handles template rendering and static file serving.

### Setup
Implement the `Writer` interface on your service to associate a `View`:
```go
func (s *Server) Writer() rs.ResponseWriter {
    f, _ := fs.Sub(publicFS, "public")
    return &rs.View{
        FS:      f,                          // fs.FS (embed.FS, os.DirFS, etc.)
        Funcs:   template.FuncMap{...},      // Custom template functions
        Skips:   regexp.MustCompile("^layout"), // Skip files matching regex from routing
        Layouts: []string{"layout/*.html"},  // Glob patterns for layout templates
        Error:   "error.html",              // Template for error pages
        Data:    func(r *http.Request) map[string]any { ... }, // Global template data
        Writer:  &rs.DefaultWriter{},       // Fallback writer for non-view responses
    }
}
```

### View Struct Fields
| Field     | Type                                      | Description                                                    |
|-----------|-------------------------------------------|----------------------------------------------------------------|
| `FS`      | `fs.FS`                                   | Source file system (required)                                  |
| `Funcs`   | `template.FuncMap`                         | Custom template functions                                     |
| `Skips`   | `*regexp.Regexp`                           | Regex to skip files from being routed                          |
| `Layouts` | `[]string`                                 | Glob patterns for layout/partial templates                     |
| `Error`   | `string`                                   | Error template path (rendered on errors if `Any` route matched)|
| `Data`    | `func(*http.Request) map[string]any`       | Callback for default template data                             |
| `Writer`  | `ResponseWriter`                           | Fallback writer for non-template responses                     |

### Template Data
Templates receive a `map[string]any` with:
- `Request`: The current `*http.Request`
- Path parameters merged in (e.g., `{{.id}}`, `{{.profile}}`)
- Data from the `Data` callback
- Handler return data: merged if `map[string]any`, otherwise available as `{{.Data}}`

### Auto-Routing
If `FS` implements `fs.ReadDirFS`, `View` automatically registers routes for `.html` and `.tmpl` files:
- `index.html` -> `/`
- `about.html` -> `/about`
- `blog/post.html` -> `/blog/post`

### Explicit Rendering
Return `*rs.Render` from a handler to render a specific template, regardless of the URL:

```go
func (s *Service) Dashboard(ctx context.Context) (*rs.Render, error) {
    return &rs.Render{
        Path: "dashboard/main.html",
        Data: map[string]interface{}{
            "Title": "Dashboard",
        },
    }, nil
}
```

## Request Reader

The `RequestReader` interface controls how request data is bound to handler arguments.

```go
type RequestReader interface {
    Read(*http.Request, []reflect.Type) ([]reflect.Value, error)
}
```

The `DefaultReader` binds JSON body, URL-encoded forms, and multipart forms. Customize the `Bind` function to add validation:

```go
h.Reader = &rs.DefaultReader{Bind: func(r *http.Request, out interface{}, methods ...string) error {
    if err := rs.Bind(r, out, methods...); err != nil {
        return err
    }
    return validate.Struct(out)
}}
```

### Bind Functions
- `rs.Bind(r, out, methods...)` — Main bind: dispatches to JSON, form, or query based on content type.
- `rs.BindJson(r, out)` — Bind JSON body.
- `rs.BindQuery(r, out)` — Bind query string params (uses `query` struct tag).
- `rs.BindForm(r, out)` — Bind form/multipart data (uses `form` struct tag).

## Response Writer

The `ResponseWriter` interface controls how handler return values are sent to the client.

```go
type ResponseWriter interface {
    Write(http.ResponseWriter, *http.Request, []reflect.Type, []reflect.Value)
}
```

### DefaultWriter
Handles JSON output with error mapping. Configurable via:
- `ErrorHandler func(error) any` — Custom error formatting. Return `*rs.Response` for full control.
- `Errors map[error]Error` — Map known errors to custom HTTP statuses/messages.
- `EscapeJsonHtml bool` — Whether to escape HTML in JSON output.

### Response Types
- **`rs.Response`**: Full control over status, headers, content-type, and body bytes.
- **`rs.Json`**: JSON response with a custom status code: `rs.Json{Status: 201, Content: obj}`.
- **`rs.Error`**: Error with status, message, data, and wrapped error.

## Middleware

Middleware is the standard `func(http.Handler) http.Handler` signature.

- **Global**: `h.Use(middleware)` in `Init`.
- **Service-level**: Implement `Middlewares() []rs.Middleware` on the service struct.
- **Route-level**: Specify `Middlewares` in the `Route` struct returned from `Routes()`.

### Built-in Middleware
- `rs.Recovery` — Panic recovery middleware; logs stack trace and returns 500 error.

## Handler Configuration

### Init Interface
Implement `Init(*rs.Handler)` to configure the handler after creation:

```go
func (s *Server) Init(h *rs.Handler) {
    h.Writer = &rs.DefaultWriter{ErrorHandler: myErrorHandler}
    h.Reader = &rs.DefaultReader{Bind: myBind}
    h.Use(loggingMiddleware)
}
```

### Handler Methods
- `rs.Handle(pattern, svc)` — Register a service on `http.DefaultServeMux`.
- `rs.NewHandler(svc)` — Create a Handler without registering on a mux.
- `h.WithPrefix(prefix)` — Set a URL prefix for the handler.
- `h.AddService(path, svc)` — Add a sub-service at runtime.
- `h.Routes()` — List all registered routes (useful for debugging/docs).
- `h.Use(middleware...)` — Add global middleware.

### Global Variables
- `rs.MaxBodySize` — Maximum request body size for `BindJson` (default: 10MB).

## Example Usage

```go
package main

import (
    "context"
    "net/http"
    rs "github.com/altlimit/restruct"
)

type API struct {
    User User `route:"users"`
}

type User struct{}

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

## Sub-packages

### `structtag`
Utility package for struct tag parsing with caching. Used internally for `query` and `form` tag binding.

- `structtag.GetFieldsByTag(obj, tagName)` — Returns `[]*StructField` for all fields with the given tag.
- `structtag.NewStructField(index, tag)` — Parses a comma-separated `key=value` tag string.
