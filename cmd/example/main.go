package main

import (
	"log"
	"net/http"
	"reflect"
	"strings"

	"github.com/altlimit/restruct"
	"github.com/go-playground/validator/v10"
)

type MyService struct {
	validate *validator.Validate
}

// extending bind to support validation with go validator
func (m *MyService) bind(r *http.Request, src interface{}, methods ...string) error {
	if err := restruct.Bind(r, src, methods...); err != nil {
		return err
	}
	if src == nil {
		return nil
	}
	if err := m.validate.Struct(src); err != nil {
		valErrors := make(map[string]string)
		for _, err := range err.(validator.ValidationErrors) {
			valErrors[err.Namespace()] = err.Tag()
		}
		return restruct.Error{Message: "validation error", Data: valErrors}
	}

	return nil
}

// Direct response, you would usually have interface{} returns so if you get an error you will
// only have to return it and ResponseWriter handles the rest.
func (m *MyService) ViewPdf(r *http.Request) restruct.Response {
	return restruct.Response{
		Status:      http.StatusOK,
		ContentType: "application/pdf",
		Content:     []byte("PDFcontent"),
	}
}

// Create new product with validation using bind and validate, in the above bind we extend
// the existing bind and added our validator. A validation error will return
// {
//   "data": {
//     "age": "min",
//     "name": "required",
//     "photos[0].url": "required"
//   },
//   "error": "validation error"
// }
func (m *MyService) Products(r *http.Request) interface{} {
	type Photo struct {
		URL string `json:"url" validate:"required"`
	}
	var req struct {
		Name   string  `json:"name" validate:"required"`
		Price  int64   `json:"age" validate:"min=1"`
		Photos []Photo `json:"photos" validate:"required,dive,min=1"`
	}
	if err := m.bind(r, &req, http.MethodPost); err != nil {
		return err
	}
	return req
}

// Use number for parameters and get it using Param function, you can use named param when adding it to Handle in prefix.
func (m *MyService) Products_0(r *http.Request) interface{} {
	params := restruct.Params(r)
	log.Println("Params", params)
	productID := params["0"]
	tag := params["tag"]
	if err := m.bind(r, nil, http.MethodGet); err != nil {
		return err
	}
	return "Product " + productID + " tag: " + tag
}

// You can emulate a standard handler by using request and response in your parameter
func (m *MyService) StandardHandler(r *http.Request, w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello"))
}

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
	my := &MyService{validate: validator.New()}
	my.validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		tags := []string{"json", "query"}
		for _, tag := range tags {
			name := strings.SplitN(fld.Tag.Get(tag), ",", 2)[0]
			if name != "-" && name != "" {
				return name
			}
		}
		return fld.Name
	})
	// wrap your service with NewHandler, this will
	svc := restruct.NewHandler(my)
	// you can add additional service and prefix it with param or just direct paths
	svc.AddService("/{tag}/", &Calculator{})
	// add this service using Handle
	restruct.Handle("/api/v1/", svc)
	port := "8090"
	log.Println("Listening " + port)
	http.ListenAndServe(":"+port, nil)
}
