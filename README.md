## Overview

RESTruct is a go rest framework based on structs. The goal of this project is to automate routing, request and response based on struct methods.

### Method Named Routing

We using method names to route our endpoints. The names below is how it translates to url paths.

```
UpperCase turns to upper-case
With_Underscore to with/underscore
HasParam_0 to has-param/{0}
```

### Create a simple service

There are utility methods included in this package to simplify building web services. You can refer to cmd/example for a more detailed examples.

```go
package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/altlimit/restruct"
)

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

func main() {
    // post {"a": 1, "b": 2} to http://localhost:8080/api/v1/add and you'll get a response
	restruct.Handle("/api/v1/", restruct.NewHandler(&Calculator{}))
	http.ListenAndServe(":8080", nil)
}
```

### Suggestions

If you have any suggestions, found a bug, create a pull request or open an issue.