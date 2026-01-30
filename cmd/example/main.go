package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	rs "github.com/altlimit/restruct"
	"github.com/go-playground/validator/v10"
)

//go:embed public
var publicFS embed.FS

type (
	V1 struct {
		validate *validator.Validate

		User  User `route:"users"`
		Blobs Blob
	}

	User struct {
	}

	Blob struct {
	}

	App struct {
	}

	Server struct {
		Api  V1
		App  App
		DB   struct{} `route:"-"`
		docs []string
	}
)

var (
	errBadRequest = errors.New("bad request")
	errAuth       = rs.Error{Status: http.StatusUnauthorized, Message: "not logged in"}
)

// Helper to get user from context
func getUser(r *http.Request) (int64, error) {
	if userID, ok := rs.GetValue(r, "userID").(int64); ok {
		// you could be doing DB to get current user here
		return userID, nil
	}
	return 0, errAuth
}

// Register custom tags for validator
func registerValidatorTags(v *validator.Validate) {
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
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

func (s *Server) Docs() *rs.Response {
	b, _ := json.Marshal(s.docs)
	return &rs.Response{
		Status:      http.StatusOK,
		ContentType: "application/json",
		Content:     b,
	}
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
			(&rs.DefaultWriter{}).WriteJSON(w, errAuth)
			return
		}
		// use SetValue/GetValue to easily sets and get values from context
		r = rs.SetValue(r, "userID", int64(1))
		next.ServeHTTP(w, r)
	})
}

func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("Request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// Add middleware to this service without changing their paths
func (b *Blob) Routes() []rs.Route {
	auth := []rs.Middleware{authMiddleware}
	return []rs.Route{
		{Handler: "Download", Path: "download/{blobID}", Methods: []string{http.MethodGet}, Middlewares: auth},
		{Handler: "Upload", Middlewares: auth},
	}
}

func (s *Server) About(ctx context.Context) any {
	return "You've hit the about page"
}

func (b *Blob) Any(ctx context.Context) any {
	slog.Info("Blob catch all", "vars", rs.Vars(ctx))
	return &rs.Response{
		Status:      http.StatusOK,
		ContentType: "application/json",
		Content:     []byte(`{"message": "Hello World"}`),
	}
}

// Add middleware to the whole struct
func (b *Blob) Middlewares() []rs.Middleware {
	return []rs.Middleware{loggerMiddleware}
}

func (b *Blob) Link_Any(ctx context.Context) string {
	slog.Info("Link Any", "vars", rs.Vars(ctx))
	return rs.Vars(ctx)["any"]
}

// Standard handler, you must handle your own response
func (b *Blob) Download(w http.ResponseWriter, r *http.Request) {
	user, err := getUser(r)
	if err != nil {
		(&rs.DefaultWriter{}).WriteJSON(w, err)
		return
	}
	blobID := rs.Params(r)["blobID"]
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
	slog.Info("CreateUser")
}

func (u *User) ReadUser(ctx context.Context) {
	slog.Info("ReadUser", "id", rs.Vars(ctx)["id"])
}

func (u *User) UpdateUser(ctx context.Context) {
	slog.Info("UpdateUser", "id", rs.Vars(ctx)["id"])
}

func (u *User) DeleteUser(ctx context.Context) {
	slog.Info("DeleteUser", "id", rs.Vars(ctx)["id"])
}

func (v *V1) Index(ctx context.Context) any {
	return map[string]any{
		"DataFromMethod": "ASD",
	}
}

func (v *V1) Any(ctx context.Context) error {
	return rs.Error{Status: http.StatusNotFound, Message: "API route not found"}
}

func (s *Server) Test_Any(ctx context.Context) any {
	path := rs.Vars(ctx)["any"]
	if path == "abc" {
		return errors.New("sample error")
	}
	return "OK"
}

// Any is a catch-all handler
func (s *Server) Any(ctx context.Context) any {
	slog.Info("Any Catch-All", "vars", rs.Vars(ctx))
	return map[string]interface{}{
		"Title":    "Error Page",
		"NotFound": "Catch-all" + rs.Vars(ctx)["any"],
	}
}

func (a *App) Any(ctx context.Context) any {
	slog.Info("App Catch All", "any", rs.Vars(ctx)["any"])
	return "Hello App"
}

func (v *V1) Backup_Any(ctx context.Context) *rs.Response {
	return &rs.Response{
		Status:      http.StatusOK,
		ContentType: "text/plain",
		Content:     []byte("OK" + rs.Vars(ctx)["any"]),
	}
}

func (s *Server) View() *rs.View {
	f, _ := fs.Sub(publicFS, "public")
	return &rs.View{
		FS:      f,
		Skips:   regexp.MustCompile("^layout"),
		Layouts: []string{"layout/*.html"},
		Error:   "error.html",
	}
}

func (s *Server) Profile(ctx context.Context) any {
	return map[string]interface{}{
		"Title":   "Profile",
		"0":       rs.Vars(ctx)["0"],
		"Profile": rs.Vars(ctx)["profile"],
	}
}

func (s *Server) Work(ctx context.Context) any {
	return map[string]interface{}{
		"Title": "Work",
		"0":     rs.Vars(ctx)["0"],
		"1":     rs.Vars(ctx)["1"],
	}
}

func (*Server) Routes() []rs.Route {
	return []rs.Route{
		{Handler: "Profile", Path: "{0}"},
		{Handler: "Work", Path: "{0}/{1}"},
	}
}

func (s *Server) Init(h *rs.Handler) {
	s.docs = h.Routes()
	// still defaultreader but used our bind to add validation errors
	h.Reader = &rs.DefaultReader{Bind: s.Api.bind}

	// add middleware
	h.Use(limitsMiddleware)

	var buf bytes.Buffer
	buf.WriteString("Endpoints:")
	for _, r := range h.Routes() {
		buf.WriteString("\n> " + r)
	}
	slog.Info(buf.String())
}

func (s *Server) Some_0(ctx context.Context) (*rs.Render, error) {
	if rs.Vars(ctx)["0"] == "error" {
		return nil, errors.New("sample error")
	}
	return &rs.Render{
		Path: "some/path.html",
		Data: map[string]interface{}{
			"Title": "Hello World",
			"Items": []string{"a", "b", "c"},
		},
	}, nil
}

func main() {
	val := validator.New()
	registerValidatorTags(val)

	rs.Handle("/", &Server{Api: V1{validate: val}})
	port := "8090"
	slog.Info("Listening", "port", port)
	http.ListenAndServe(":"+port, nil)
}
