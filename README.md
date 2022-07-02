![Run Tests](https://github.com/altlimit/restruct/actions/workflows/run-tests.yaml/badge.svg)

# restruct

RESTruct is a go rest framework based on structs. The goal of this project is to automate routing, request and response based on struct methods.

---
* [Install](#install)
* [Router](#router)
* [Examples](#examples)
* [Response Writer](#response-writer)
* [Request Reader](#request-reader)
* [Middleware](#middleware)
* [Nested Structs](#nested-structs)
* [Utilities](#utilities)
---

## Install

```sh
go get github.com/altlimit/restruct
```

## Router

Exported struct methods will be your handlers and will be routed like the following.

```
UpperCase turns to upper-case
With_Underscore to with/underscore
HasParam_0 to has-param/{0}
HasParam_0_AndMore_1 to has-param/{0}/and-more/{1}
```

There are multiple ways to process a request and a response, such as strongly typed parameters and returns or with `*http.Request` or `http.ResponseWriter` parameters. You can also use the `context.Context` parameter. Any other parameters will use the `DefaultReader` which you can override in your `Handler.Reader`.

```go
type Calculator struct {
}

func (c *Calculator) Add(r *http.Request) interface{} {
    var req struct {
        A int64 `json:"a"`
        B int64 `json:"b"`
    }
    if err := restruct.Bind(r, &req, http.MethodPost); err != nil {
        return err
    }
    return req.A + req.B
}

func (c *Calculator) Subtract(a, b int64) int64 {
    return a - b
}

func (c *Calculator) Divide(a, b int64) (int64, error) {
    if b == 0 {
        return 0, errors.New("divide by 0")
    }
    return a / b
}

func (c *Calculator) Multiply(r struct {
    A int64 `json:"a"`
    B int64 `json:"b"`
}) int64 {
    return r.A * r.B
}

func main() {
	restruct.Handle("/api/v1/", &Calculator{})
	http.ListenAndServe(":8080", nil)
}
```

We have registered the `Calculator` struct here as our service and we should now have available endpoints which you can send json request and response to.

```json
// POST http://localhost:8080/api/v1/add
{
    "a": 10,
    "b": 20
}
// -> 20
// -> or any errors such as 400 {"error": "Bad Request"}

// POST http://localhost:8080/api/v1/subtract
// Since this is a non-request, response, context parameter
// it will be coming from json array request as a default behavior from DefaultReader
[
    20,
    10
]
// -> 10

// POST http://localhost:8080/api/v1/divide
// You can also have the ability to have a strongly typed handlers in your parameters and return types.
// Default behaviour from DefaultWriter is if multiple returns with last type is an error with value then it writes it.
[
    1,
    0
]
// -> 500 {"error": "Internal Server Error"}

// POST http://localhost:8080/api/v1/multiple
// With a single struct as a parameter, it will be similar to Add's implementation where it uses Bind internally to populate it. You can change your Bind with DefaultReader{Bind:...} to add your validation library.
{
    "a": 2,
    "b": 5
}
// -> 10
```

You can override default method named routes using `Router` interface. Implement Router in your service and return a slice `Route`.

```go
func (c *Calculator) Routes() []Route {
    return []Route{
        Route{Handler: "Add", Path:"addition", Methods: []string{http.MethodPost}},
        Route{Handler: "Subtract", Path:"subtraction", Methods: []string{http.MethodPost}},
    }
}
```


## Examples

Here are more ways to create handlers.

```go
type Blob struct {
    Internal bool
}

func (b *Blob) Routes() []Route {
    return []Route{
        {Handler: "Download", Path: "blob/{path:.+}", methods: []string{http.MethodGet}}
    }
}

// Will be available at /blob/{path:.+} since we overwrite it in Routes
func (b *Blob) Download(w http.ResponseWriter, r *http.Request) {
    path := restruct.Params(r)["path"]
    // handle your struct like normal
}
```

Here we use `Router` interface to add a regular expression. The path param on the download Route will accept anything even an additional nested paths `/` and it also has a standard handler definition.

To register the above service:

```go
func main() {
	restruct.Handle("/api/v1/", &Blob{})
	http.ListenAndServe(":8080", nil)
}
```

You can create additional service with a different prefix by calling `NewHandler` on your struct then adding it with `AddService`.

```go
h := restruct.NewHandler(&Blob{})
h.AddService("/internal/{tag}/", &Blob{Internal: true})
restruct.Handle("/api/v1/", h)
```

All your services will now be at `/api/v1/internal/{tag}`. You can also register the returned Handler in a third party router but make sure you call `WithPrefix(...)` on it if it's not a root route.

```go
http.Handle("/", h)
// or if it's a not a root route
http.Handle("/api/v1/", h.WithPrefix("/api/v1/"))
```

You can have parameters with method using number and access them using `restruct.Params()`:

```go
// Will be available at /upload/{0}
func (b *Blob) Upload_0(r *http.Request) interface{} {
    uploadType := restruct.Params(r)["0"]
    // handle your request normally
    fileID := ...
    return fileID
}
```

Refer to cmd/example for some advance usage.

## Response Writer

The default `ResponseWriter` is `DefaultWriter` which uses json.Encoder().Encode to write outputs. This also handles errors and status codes. You can modify the output by implementing the ResponseWriter interface and set it in your `Handler.Writer`.

```go
type TextWriter struct {}

func (tw *TextWriter) Write(w http.ResponseWriter, r *http.Request, types []reflect.Type, vals []reflect.Value) {
    // types - slice of return types
    // vals - slice of actual returned values
    // this writer we simply write anything returned as text
    var out []interface{}
    for _, val := range vals {
        out = append(out, val.Interface())
    }
    w.WriteHeader(http.StatusOK)
    w.Header().Set("Content-Type", "text/plain")
    w.Write([]byte(fmt.Sprintf("%v", out)))
}

h := restruct.NewHandler(&Blob{})
h.Writer = &TextWriter{}
```

## Request Reader

A handler can have any or no parameters, but the default parameters that doesn't go through request reader are: `context.Context`, `*http.Request` and `http.ResponseWriter`, these parameters will not be passed in `RequestReader.Read` interface.

```go
// use form for urlencoded post
type login struct {
    Username string `json:"username" form:"username"`
    Password string `json:"password" from:"password"`
}

func (b *Blob) Login(l *login) interface{} {
    log.Println("Login", l.Username, l.Password)
    return "OK"
}
```

This uses the `DefaultReader` which by default can unmarshal single struct and use default bind(`restruct.Bind`), you can use your own Bind with `DefaultReader{Bind:yourBinder}` if you want to add validation libraries. The Bind reads the body with json.Encoder, or form values. If you have multiple paramters you will need to send a json array body.

```json
[
    "FirstParam",
    2,
    {"third":"param"}
]
```

This is the default behaviour of `DefaultReader`. You can implement `RequestReader` interface which will allow you to control your own parameter parsing.

```go
type CustomReader struct {}
func (cr *CustomReader) Read(r *http.Request, types []reflect.Type) (vals []reflect.Value, err error) {
    // types are the paramter types in order of your handler you must return equal number of vals to args.
    // You'll only get types that is not *http.Request, http.ResponseWriter, context.Context
    // You can return Error{} type here to return ResponseWriter errors/response and wrap your errors inside Error{Err:...}
    return
}

```
## Middleware

Uses standard middleware and add by `handler.Use(...)` or you can add it under `Route` when using the `Router` interface.

```go
func auth(next http.Handler) http.Handler {
    // you can use your h.Writer here if it's accessible somewhere
	wr := rs.DefaultWriter{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "abc" {
			wr.WriteJSON(w, rs.Error{Status: http.StatusUnauthorized})
			return
		}
		next.ServeHTTP(w, r)
	})
}

h := restruct.NewHandler(&Blob{})
h.Use(auth)
```

## Nested Structs

Nested structs are automatically routed. You can use route tag to customize or add `route:"-"` to skip exported structs.

```go
type (
    V1 struct {
        Users User
        DB DB `route:"-"`
    }

    User struct {

    }
)

func (v *V1) Drop() {}
func (u *User)  SendEmail() {}

func main() {
    restruct.Handle("/api/v1/", &V1{})
    http.ListenAndServe(":8080", nil)
}
```

Will generate route: `/api/v1/drop` and `/api/v1/users/send-email`

## Utilities

Available helper utilities for processing requests and response.

```go
// Adding context values in middleware such as logged in userID
auth := r.Header.Get("Authorization") == "some-key-or-jwt"
if userID, ok := UserIDFromAuth(auth); ok {
    r = restruct.SetValue(r, "userID", userID)
}
// then access it from anywhere or a private method for getting your user record
if userID, ok := restruct.GetValue(r, "userID").(int64); ok {
    user, err := DB.GetUserByID(ctx, userID)
    // do something with user
}

// Bind helps read your json and form requests into a struct, you can add tag "query"
// to bind query strings at the same time. You can also add tag "form" to bind form posts from
// urlencoded or multipart. You can also use explicit functions BindQuery or BindForm.
var loginReq struct {
    Username string `json:"username"`
    Password string `json:"password"`
}
if err := restruct.Bind(r, &loginReq, http.MethodPost); err != nil {
    return err
}

// Reading path parameters with Params /products/{0}
params := restruct.Params(r)
productID := params["0"]
```

## License

MIT