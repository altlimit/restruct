package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"

	rs "github.com/altlimit/restruct"
	"github.com/go-playground/validator/v10"
)

type (
	MyService struct {
		validate *validator.Validate

		CalcuLator  Calculator
		Calculator2 Calculator  `route:"-"`
		Calculator3 *Calculator `route:"calc/{abc}"`
		NotService  int
	}

	LoginRequest struct {
		Username string `json:"username" validate:"required"`
		Password string `json:"password" validate:"required"`
	}
)

var (
	errBadRequest = errors.New("bad request")
	errAuth       = fmt.Errorf("not logged in")

	writer = &rs.DefaultWriter{
		Errors: map[error]rs.Error{
			errAuth:       {Status: http.StatusUnauthorized},
			errBadRequest: {Status: http.StatusBadRequest},
		},
	}
)

// extending bind to support validation with go validator
func (m *MyService) bind(r *http.Request, src interface{}, methods ...string) error {
	if err := rs.Bind(r, src, methods...); err != nil {
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
		return rs.Error{Status: http.StatusBadRequest, Message: "validation error", Data: valErrors}
	}

	return nil
}

func (m *MyService) Login(login *LoginRequest) string {
	return login.Username
}

// Direct response, you would usually have interface{} returns so if you get an error you will
// only have to return it and ResponseWriter handles the rest.
func (m *MyService) ViewPdf(r *http.Request) rs.Response {
	return rs.Response{
		Status:      http.StatusOK,
		ContentType: "application/pdf",
		Content:     []byte("PDFcontent"),
	}
}

// Create new product with validation using bind and validate, in the above bind we extend
// the existing bind and added our validator. A validation error will return
// {
//   "data": {
//     "price": "min",
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
		Price  int64   `json:"price" validate:"min=1"`
		Photos []Photo `json:"photos" validate:"required,dive,min=1"`
	}
	if err := m.bind(r, &req, http.MethodPost); err != nil {
		return err
	}
	return req
}

func (m *MyService) FormSample(r *http.Request) interface{} {
	var req struct {
		Name  string                `form:"name"`
		Price int64                 `form:"price"`
		File  *multipart.FileHeader `form:"file"`
	}
	if err := m.bind(r, &req, http.MethodPost); err != nil {
		return err
	}
	if req.File != nil {
		f, err := req.File.Open()
		if err != nil {
			return err
		}
		defer f.Close()
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}
		return string(b)
	}

	return req
}

// Use number for parameters and get it using Param function, you can use named param when adding it to Handle in prefix.
func (m *MyService) Products_0(r *http.Request) interface{} {
	params := rs.Params(r)
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

type Nested struct{}

func (n *Nested) Routes() []rs.Route {
	return []rs.Route{
		{Handler: "Sample", Path: ".custom./routed/{id}", Methods: []string{http.MethodGet}},
		{Handler: "Sample2", Path: ".custom./routed/{id}", Methods: []string{http.MethodPost}},
	}
}

func (n *Nested) Sample() {
	log.Println("Sample")
}

func (n *Nested) Sample2() {
	log.Println("Sample2")
}

type Calculator struct {
	Another Nested
}

func (c *Calculator) Add(r *http.Request) interface{} {
	var req struct {
		A int64 `json:"a"`
		B int64 `json:"b"`
	}
	if err := rs.Bind(r, &req, http.MethodPost); err != nil {
		return err
	}
	return req.A + req.B
}

func (c Calculator) NonPointer() int {
	return 5
}

func (c *Calculator) Err(r *http.Request) interface{} {
	return errAuth
}

func limitsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("Limits reader")
		var maxBodyLimit int64 = 1 << 20
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload") {
			maxBodyLimit = 128 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyLimit)
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.Handler) http.Handler {
	wr := rs.DefaultWriter{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("Auth")
		if strings.HasSuffix(r.URL.Path, "/standard-handler") && r.Header.Get("Authorization") != "abc" {
			log.Println("Failed Auth")
			wr.WriteJSON(w, r, rs.Error{Status: http.StatusUnauthorized})
			return
		}
		// use SetValue/GetValue to easily sets and get values from context
		r = rs.SetValue(r, "loggedIn", true)
		next.ServeHTTP(w, r)
	})
}

func catchAllHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("Caught", r.URL.Path)
	}
}

func main() {
	my := &MyService{validate: validator.New(), Calculator3: &Calculator{}}
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
	// wrap your service with NewHandler
	v1 := rs.NewHandler(my)
	v1.Reader = &rs.DefaultReader{Bind: my.bind}
	// you can add additional service and prefix it with param or just direct paths
	v1.AddService("/{tag}/", &Calculator{})
	// add middleware
	v1.Use(limitsMiddleware, authMiddleware)
	// custom writer
	v1.Writer = writer
	// this is same as http.Handle("/api/v1/", v1.WithPrefix("/api/v1/"))
	rs.Handle("/api/v1/", v1)
	http.Handle("/", catchAllHandler())
	port := "8090"
	var buf bytes.Buffer
	buf.WriteString("Endpoints:")
	for _, r := range v1.Routes() {
		buf.WriteString("\n> " + r)
	}
	log.Println(buf.String())
	log.Println("Listening", port)
	http.ListenAndServe(":"+port, nil)
}
