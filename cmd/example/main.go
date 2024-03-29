package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
		docs  []string
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

func (v *V1) Docs() []string {
	return v.docs
}

func (v *V1) Pages(r *http.Request) (code int, pages []string, err error) {
	code = http.StatusAccepted
	pages = append(pages, "hello", "world")
	if e := r.URL.Query().Get("err"); e != "" {
		err = errors.New(e)
	}
	return
}

func (v *V1) RawResponse() *rs.Response {
	return &rs.Response{
		Status:      http.StatusOK,
		ContentType: "text/html",
		Content:     []byte(`<html><body>Hi</body></html>`),
	}
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
			writer.WriteJSON(w, rs.Error{Status: http.StatusUnauthorized})
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
	auth := []rs.Middleware{authMiddleware}
	return []rs.Route{
		{Handler: "Download_0", Methods: []string{http.MethodGet}, Middlewares: auth},
		{Handler: "Upload", Middlewares: auth},
		{Handler: "Link", Path: "links/{path:.+}"},
	}
}

// Add middleware to the whole struct
func (b *Blob) Middlewares() []rs.Middleware {
	return []rs.Middleware{loggerMiddleware}
}

// Magic var 0Path means anything after /link/.+ without regex
func (b *Blob) Link_0Path(ctx context.Context) string {
	return rs.Vars(ctx)["0Path"]
}

func (b *Blob) Link(ctx context.Context) string {
	return rs.Vars(ctx)["path"]
}

// Standard handler, you must handle your own response
func (b *Blob) Download_0(w http.ResponseWriter, r *http.Request) {
	user, err := v1.user(r)
	if err != nil {
		writer.WriteJSON(w, err)
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
	buf, err := io.ReadAll(f)
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

// CRUD api with POST on api/v1/users and GET,PUT,DELETE on api/v1/users/{id}
func (*User) Routes() []rs.Route {
	return []rs.Route{
		{Handler: "CreateUser", Path: ".", Methods: []string{http.MethodPost}},
		{Handler: "ReadUser", Path: "{id}", Methods: []string{http.MethodGet}},
		{Handler: "UpdateUser", Path: "{id}", Methods: []string{http.MethodPut}},
		{Handler: "DeleteUser", Path: "{id}", Methods: []string{http.MethodDelete}},
	}
}

func (u *User) CreateUser() {
	log.Println("CreateUser")
}

func (u *User) ReadUser(ctx context.Context) {
	log.Println("ReadUser", rs.Vars(ctx)["id"])
}

func (u *User) UpdateUser(ctx context.Context) {
	log.Println("UpdateUser", rs.Vars(ctx)["id"])
}

func (u *User) DeleteUser(ctx context.Context) {
	log.Println("DeleteUser", rs.Vars(ctx)["id"])
}

func (v *V1) notFound(r *http.Request) error {
	log.Println("Not Found", r.URL.Path)
	return rs.Error{Status: http.StatusNotFound}
}

// all initialization can happen within the strcut method
func (v *V1) Init(h *rs.Handler) {
	v1.docs = h.Routes()
	// still defaultreader but used our bind to add validation errors
	h.Reader = &rs.DefaultReader{Bind: v1.bind}
	// still defaultwriter but with options to map custom errors
	h.Writer = writer
	// add middleware
	h.Use(limitsMiddleware)
	h.NotFound(v1.notFound)

	var buf bytes.Buffer
	buf.WriteString("Endpoints:")
	for _, r := range h.Routes() {
		buf.WriteString("\n> " + r)
	}
	log.Println(buf.String())
}

func main() {
	rs.Handle("/api/v1/", v1)
	http.Handle("/", catchAllHandler())
	port := "8090"
	log.Println("Listening", port)
	http.ListenAndServe(":"+port, nil)
}
