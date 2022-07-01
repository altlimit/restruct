![Run Tests](https://github.com/altlimit/restruct/actions/workflows/run-tests.yaml/badge.svg)

# restruct

RESTruct is a go rest framework based on structs. The goal of this project is to automate routing, request and response based on struct methods.

---
* [Install](#install)
* [Examples](#examples)
* [Route By Methods](#route-by-methods)
* [Response Writer](#response-writer)
* [Request Reader](#request-reader)
* [Middleware](#middleware)
* [Nested Structs](#nested-structs)
* [Custom Routes](#custom-routes)
* [Utilities](#utilities)
---

## Install

```sh
go get github.com/altlimit/restruct
```

## Examples

Let's create a calculator service:

```go
type Calculator struct {
    Advance bool
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
```

We define our services using struct methods. Here we define a single endpoint `Add` that is translated to "add" in the endpoint. We use our utility method Bind to restrict other methods and bind request body into our struct. You can ofcourse handle all this on your own and return any value or if you prefer have both r *http.Request and w http.ResponseWriter without a return and it will just be like a regular handler.

To register the above service:

```go
func main() {
	restruct.Handle("/api/v1/", &Calculator{})
	http.ListenAndServe(":8080", nil)
}
```

You can now try to do a post to http://localhost:8080/api/v1/add with body:

```json
{
    "a": 1,
    "b": 2
}
```

You can create additional service with a different prefix by call NewHandler on your struct then adding it with AddService.

```go
h := restruct.NewHandler(&Calculator{})
h.AddService("/advance/{tag}/", &Calculator{Advance: true})
restruct.Handle("/api/v1/", h)
```

All your services will now be at /api/v1/advance/{tag}. You can also register the returned Handler in a third party router but make sure you call `WithPrefix(...)` on it if it's not a root route.

```go
http.Handle("/api/v1/", h.WithPrefix("/api/v1/"))
```

You can have parameters with method using number and access them using `restruct.Params()`:

```go
func (c *Calculator) Edit_0(r *http.Request) interface{} {
    params := restruct.Params(r)
    log.Println("Edited", params["0"], "with tag", params["tag"])
    return "OK"
}
```

Refer to cmd/example for some advance usage.

## Route By Methods

Public method will become routed using the pattern below.

```
UpperCase turns to upper-case
With_Underscore to with/underscore
HasParam_0 to has-param/{0}
HasParam_0_AndMore_1 to has-param/{0}/and-more/{1}
```

## Response Writer

The default `ResponseWriter` is `DefaultWriter` which uses json.Encoder().Encode to write outputs. This also handles errors and status codes. You can modify the output by implementing the ResponseWriter interface and set it in your `Handler`.

```go
type TextWriter struct {}

func (tw *TextWriter) Write(w http.ResponseWriter, r *http.Request, out interface{}) {
    if err, ok := out.(error); ok {
        w.WriteHeader(http.StatusInternalServerError)
    } else {
        w.WriteHeader(http.StatusOK)
    }
    w.Header().Set("Content-Type", "text/plain")
    w.Write([]byte(fmt.Sprintf("%v", out)))
}

h := restruct.NewHandler(&Calculator{})
h.Writer = &TextWriter{}
```

## Request Reader

A handler can have any or no parameters, but the default parameters that doesn't go through request reader are: context.Context, *http.Request and http.ResponseWriter.

```go
// use form for urlencoded post
type login struct {
    Username string `json:"username" form:"username"`
    Password string `json:"password" from:"password"`
}

func (c *Calculator) Login(l *login) interface{} {
    log.Println("Login", l.Username, l.Password)
    return "OK"
}
```

This uses the DefaultReader which by default can unmarshal single struct and use default bind, you can use your own Bind with `DefaultReader{Bind:yourBinder}` if you want to add validation libraries. The Bind reads the body with json.Encoder, or form values. If you have multiple paramters you will need to send a json array body.

```json
[
    "FirstParam",
    2,
    {"third":"param"}
]
```

This is the default behaviour of DefaultReader. You can implement RequestReader interface which will allow you to control your own parameter parsing.

```go
type CustomReader struct {}
func (cr *CustomReader) Read(r *http.Request, args []reflect.Type) (vals []reflect.Value, err error) {
    // args are the paramter types in order of your handler
    // you must return equal number of vals to args.
    // You'll only get types that is not *http.Request, http.ResponseWriter, context.Context
    // You can return Error{} type here to return ResponseWriter errors/response and wrap your errors inside Error{Err:...}
    return
}

```
## Middleware

Uses standard middleware and add by `handler.Use(...)` or you can add it under `Router` interface{}.

```go
func auth(next http.Handler) http.Handler {
    // you can use your h.Writer here if it's accessible somewhere
	wr := rs.DefaultWriter{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "abc" {
			wr.Write(w, rs.Error{Status: http.StatusUnauthorized})
			return
		}
		next.ServeHTTP(w, r)
	})
}

h := restruct.NewHandler(&Calculator{})
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

Will generate route: /api/v1/drop and /api/v1/users/send-email

## Custom Routes

You can override default method named routes using `Router` interface. Implement Router in your service and return a map of method name to `Route`. You can also add regular expression in your params and restrict methods in the Route.

```go
func (v *V1) Routes() map[string]Route {
    return map[string]Route{"Drop": Route{Path:".custom/path/{here:.+}", Methods: []string{http.MethodGet}}}
}
```

This will change the path to /api/v1/.custom/path/{here}. The param `here` will match anything even with additional nested paths. It will also return a `Error{Status: http.StatusMethodNotAllowed}` if it's not a GET request.

You can restrict methods in multiple ways:

```go
func (c *Calculator) ReadFile(r *http.Request) interface{} {
    // using standard way
    if r.Method != http.MethodGet {
        return Error{Status: http.StatusNotFound}
    }
    // using the Bind method
    // if you support both get and post you add , http.MethodPost and a pointer to struct to bind request body.
    if err := restruct.Bind(r, nil, http.MethodGet); err != nil {
        return err
    }
}
```

or use the above method by implementing `Router` interface.

```go
func (v *V1) Routes() map[string]Route {
    // optional Path to use the default behavior "read-file"
    return map[string]Route{"ReadFile": Route{Methods: []string{http.MethodGet}}}
}
```

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

MIT licened. See the LICENSE file for details.