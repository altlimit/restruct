# restruct

RESTruct is a go rest framework based on structs. The goal of this project is to automate routing, request and response based on struct methods.

---
* [Install](#install)
* [Examples](#examples)
* [Route By Methods](#route-by-methods)
* [Response Writer](#response-writer)
* [Middleware](#middleware)
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

We define our services using struct methods. You can store db, caching, etc into your struct properties so it's easily accessible by your service. Here we define a single endpoint "Add" that is translated to "add" in the endpoint. We use our utility method Bind to restrict other methods and bind request body into our struct. You can ofcourse handle all this on your own and return any value or if you prefer have both r *http.Request and w http.ResponseWriter without a return and it will just be like a regular handler.

To register the above service:

```go
func main() {
	restruct.Handle("/api/v1/", restruct.NewHandler(&Calculator{}))
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

You can add multiple service on the returned handler with different prefix:

```go
h := restruct.NewHandler(&Calculator{})
h.AddService("/advance/{tag}/", &Calculator{Advance: true})
```
All your services will now be at /api/v1/advance/{tag}

You can have parameters with method using number and access them using `restruct.Params()`:

```go
func (c *Calculator) Edit_0(r *http.Request) interface{} {
    params := restruct.Params(r)
    log.Println("Edited", params["0"], "with tag", params["tag"])
    return "OK"
}
```

You can refer to cmd/example for some advance usage.


## Route By Methods

Public method will become routed using the pattern below.

```
UpperCase turns to upper-case
With_Underscore to with/underscore
HasParam_0 to has-param/{0}
HasParam_0_AndMore_1 to has-param/{0}/and-more/{1}
```

## Response Writer

The default ResponseWriter is DefaultWriter which uses json.Encoder().Encode to write outputs. This also handles errors and status codes. You can modify the output by implementing the ResponseWriter interface and adding it to your handler.

```go
type TextWriter struct {

}

func (tw *TextWriter) Write(w http.ResponseWriter, out interface{}) {
    w.WriteHeader(http.StatusOK)
    w.Header().Set("Content-Type", "text/plain")
    w.Write([]byte(fmt.Sprintf("%v", out)))
}

h := restruct.NewHandler(&Calculator{})
h.AddWriter("text/plain", &TextWriter{})
```

This will write all response in plain text. If no content type is found it will use the first response writer it finds.

## Middleware

Uses standard middleware and add by `handler.Use(...)`

```go
func auth(next http.Handler) http.Handler {
	wr := rs.DefaultWriter{} // or your preferred writer
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

## License

MIT licened. See the LICENSE file for details.