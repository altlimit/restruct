package main

import (
	"bytes"
	"context"
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
	V1 struct {
		validate *validator.Validate
		DB       struct{} `route:"-"`

		User  User `route:"users"`
		Blobs Blob
	}

	User struct {
	}

	Blob struct {
	}
)

var (
	errBadRequest = errors.New("bad request")
	errAuth       = fmt.Errorf("not logged in")

	v1     *V1
	writer = &rs.DefaultWriter{
		Errors: map[error]rs.Error{
			errAuth:       {Status: http.StatusUnauthorized},
			errBadRequest: {Status: http.StatusBadRequest},
		},
	}
)

func init() {
	v1 = &V1{validate: validator.New()}
	v1.validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		tags := []string{"json", "query"}
		for _, tag := range tags {
			name := strings.SplitN(fld.Tag.Get(tag), ",", 2)[0]
			if name != "-" && name != "" {
				return name
			}
		}
		return fld.Name
	})
}

// extending bind to support validation with go validator
func (v *V1) bind(r *http.Request, src interface{}, methods ...string) error {
	// we still use default bind but add in our custom validator library below
	if err := rs.Bind(r, src, methods...); err != nil {
		return err
	}
	if src == nil {
		return nil
	}
	if err := v.validate.Struct(src); err != nil {
		valErrors := make(map[string]string)
		for _, err := range err.(validator.ValidationErrors) {
			valErrors[err.Namespace()] = err.Tag()
		}
		return rs.Error{Status: http.StatusBadRequest, Message: "validation error", Data: valErrors}
	}

	return nil
}

func (v *V1) user(r *http.Request) (int64, error) {
	if userID, ok := rs.GetValue(r, "userID").(int64); ok {
		// you could be doing DB to get current user here
		return userID, nil
	}
	return 0, errAuth
}

// limit request size middleware
func limitsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var maxBodyLimit int64 = 1 << 20
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload") {
			maxBodyLimit = 128 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyLimit)
		next.ServeHTTP(w, r)
	})
}

// auth middleware
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "admin" {
			writer.WriteJSON(w, r, rs.Error{Status: http.StatusUnauthorized})
			return
		}
		// use SetValue/GetValue to easily sets and get values from context
		r = rs.SetValue(r, "userID", int64(1))
		next.ServeHTTP(w, r)
	})
}

func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.Method, " - ", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func catchAllHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("Caught", r.URL.Path)
	}
}

// Add middleware to this service without changing their paths
func (b *Blob) Routes() []rs.Route {
	// todo maybe ability to somehow put middleware to a whole nested struct
	auth := []rs.Middleware{authMiddleware}
	return []rs.Route{
		{Handler: "Download_0", Methods: []string{http.MethodGet}, Middlewares: auth},
		{Handler: "Upload", Middlewares: auth},
	}
}

func (b *Blob) Middlewares() []rs.Middleware {
	return []rs.Middleware{loggerMiddleware}
}

// Standard handler, you must handle your own response
func (b *Blob) Download_0(w http.ResponseWriter, r *http.Request) {
	user, err := v1.user(r)
	if err != nil {
		writer.WriteJSON(w, r, err)
		return
	}
	blobID := rs.Params(r)["0"]
	blob := fmt.Sprintf("BlobByUser: %d -> %s", user, blobID)
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "text/plain")
	w.Write([]byte(blob))
}

type uploadRequest struct {
	Name string                `form:"name" validate:"required"`
	File *multipart.FileHeader `form:"file" validate:"required"`
}

func (b *Blob) Upload(ctx context.Context, upload *uploadRequest) interface{} {
	f, err := upload.File.Open()
	if err != nil {
		return err
	}
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	return map[string]interface{}{
		"filename": upload.File.Filename,
		"size":     upload.File.Size,
		"content":  string(buf),
	}
}

func (u *User) Login(login struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}) (bool, error) {
	if login.Username == "admin" && login.Password == "admin" {
		return true, nil
	}
	return false, rs.Error{Status: http.StatusForbidden, Message: "Invalid login"}
}

func main() {
	h := rs.NewHandler(v1)
	// still defaultreader but used our bind to add validation errors
	h.Reader = &rs.DefaultReader{Bind: v1.bind}
	// still defaultwriter but with options to map custom errors
	h.Writer = writer
	// add middleware
	h.Use(limitsMiddleware)
	// this is same as http.Handle("/api/v1/", v1.WithPrefix("/api/v1/"))
	rs.Handle("/api/v1/", h)
	http.Handle("/", catchAllHandler())
	port := "8090"
	var buf bytes.Buffer
	buf.WriteString("Endpoints:")
	for _, r := range h.Routes() {
		buf.WriteString("\n> " + r)
	}
	log.Println(buf.String())
	log.Println("Listening", port)
	http.ListenAndServe(":"+port, nil)
}
