## Overview

RESTruct is a go rest framework based on structs. The goal of this project is to automate routing, request and response based on struct methods.

### Method name to routing

```
CamelCase turns to camel-case
With_Underscore to with/underscore
HasParam_0 to has-param/{0}
```
You can add multiple service in a single handler.

### Example Usage

Methods can only have *http.Request and/or http.ResponseWriter as a parameter. A single return value will be sent to the ResponseWriter. By default we have a DefaultWriter that uses Json and handles errors. You can customize this by implementing ResponseWriter interface. No return will still send it to response writer with nil value. Multiple returns will send it as an array of interface.

```go
package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/altlimit/restruct"
)

type MyService struct {
}

func (m *MyService) CustomResponse(r *http.Request) restruct.Response {
	return restruct.Response{
		Status:  http.StatusBadRequest,
		Content: map[string]string{"Hello": "worl"},
	}
}

func (m *MyService) MultiPle(r *http.Request) (int, error) {
	return 1, errors.New("Hi")
}

func (m *MyService) WithBinding(r *http.Request) interface{} {
	var req struct {
		Test  []string `query:"test"`
		TestI []int    `query:"t"`
		Limit int      `query:"limit"`
		Hello string   `json:"hello"`
		Data  int64    `json:"data"`
	}
	if err := restruct.Bind(r, &req, http.MethodPost); err != nil {
		return err
	}
	return req
}

func (m *MyService) WithParams_0(r *http.Request, w http.ResponseWriter) interface{} {
	return "Product " + restruct.Param(r.Context(), "0") + " TAG: " + restruct.Param(r.Context(), "tag")
}

func main() {
	svc := restruct.NewHandler(&MyService{})
	svc.AddService("/{tag}/", &MyService{})
	restruct.Handle("/api/v1/", svc)
	http.ListenAndServe(":8080", nil)
}
```

### Suggestions

If you have any suggestions bug fix, just create a pull request or open an issue.