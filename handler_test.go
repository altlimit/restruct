package restruct_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rs "github.com/altlimit/restruct"
)

// Integration tests

type (
	DB struct{}
	V1 struct {
		DB DB `route:"-"`

		User  User `route:"users"`
		Blobs Blob
	}

	User struct {
	}

	Blob struct {
	}

	Calculator struct {
	}
)

var (
	errBadRequest = errors.New("bad request")
	errAuth       = fmt.Errorf("not logged in")

	executions int
)

func (v *V1) bind(r *http.Request, src interface{}, methods ...string) error {
	if err := rs.Bind(r, src, methods...); err != nil {
		return err
	}
	if src == nil {
		return nil
	}
	return nil
}

func execMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executions++
		r = rs.SetValue(r, "execs", executions)
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.Handler) http.Handler {
	wr := rs.DefaultWriter{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "admin" {
			wr.WriteJSON(w, rs.Error{Status: http.StatusUnauthorized})
			return
		}
		r = rs.SetValue(r, "userID", int64(1))
		next.ServeHTTP(w, r)
	})
}

func (db *DB) Query() error {
	return nil
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

func (c *Calculator) Subtract(a, b int64) int64 {
	return a - b
}

func (c *Calculator) Divide(a, b int64) (int64, error) {
	if b == 0 {
		return 0, errors.New("divide by 0")
	}
	return a / b, nil
}

func (c *Calculator) Multiply(r struct {
	A int64 `json:"a"`
	B int64 `json:"b"`
}) int64 {
	return r.A * r.B
}

func (b *Blob) Routes() []rs.Route {
	// todo maybe ability to somehow put middleware to a whole nested struct
	auth := []rs.Middleware{authMiddleware}
	return []rs.Route{
		{Handler: "Download_0", Methods: []string{http.MethodGet}, Middlewares: auth},
		{Handler: "Upload", Path: ".custom/{path*}", Middlewares: auth},
	}
}

// Standard handler, you must handle your own response
func (b *Blob) Download_0(w http.ResponseWriter, r *http.Request) {
	blobID := rs.Params(r)["0"]
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "text/plain")
	w.Write([]byte(string(blobID)))
}

func (b *Blob) Link_Any(ctx context.Context) string {
	return rs.Vars(ctx)["any"]
}

func (b *Blob) Upload(r *http.Request) interface{} {
	return rs.Params(r)["path"]
}

func (u *User) Login(ctx context.Context, login struct {
	Username string `json:"username" form:"username"`
	Password string `json:"password" form:"password"`
}) (bool, error) {
	if login.Username == "admin" && login.Password == "admin" {
		return true, nil
	}
	return false, rs.Error{Status: http.StatusForbidden, Message: "Invalid login"}
}

func (u *User) Execs(r *http.Request) int {
	execs := rs.GetValue(r, "execs").(int)
	return execs
}

func (v *V1) Index() string {
	return "Welcome to Root"
}

func (v *V1) Files_Any(ctx context.Context) string {
	return rs.Vars(ctx)["any"]
}

func TestHandler(t *testing.T) {
	v1 := &V1{}
	h := rs.NewHandler(v1)
	h.AddService("calc", new(Calculator))
	h.Use(execMiddleware)
	h.Reader = &rs.DefaultReader{Bind: v1.bind}
	h.Writer = &rs.DefaultWriter{
		Errors: map[error]rs.Error{
			errAuth:       {Status: http.StatusUnauthorized},
			errBadRequest: {Status: http.StatusBadRequest},
		},
	}
	var buf bytes.Buffer
	for _, r := range h.Routes() {
		buf.WriteString(r + "\n")
	}
	routes := `/ [*] -> github.com/altlimit/restruct_test.V1.Index() (string)
/blobs/.custom/{path*} [*] -> github.com/altlimit/restruct_test.Blob.Upload(*http.Request) (interface {})
/blobs/download/{0} [GET] -> github.com/altlimit/restruct_test.Blob.Download_0(http.ResponseWriter, *http.Request)
/blobs/link/{any*} [*] -> github.com/altlimit/restruct_test.Blob.Link_Any(context.Context) (string)
/calc/add [*] -> github.com/altlimit/restruct_test.Calculator.Add(*http.Request) (interface {})
/calc/divide [*] -> github.com/altlimit/restruct_test.Calculator.Divide(int64, int64) (int64, error)
/calc/multiply [*] -> github.com/altlimit/restruct_test.Calculator.Multiply(struct { A int64 "json:\"a\""; B int64 "json:\"b\"" }) (int64)
/calc/subtract [*] -> github.com/altlimit/restruct_test.Calculator.Subtract(int64, int64) (int64)
/files/{any*} [*] -> github.com/altlimit/restruct_test.V1.Files_Any(context.Context) (string)
/users/execs [*] -> github.com/altlimit/restruct_test.User.Execs(*http.Request) (int)
/users/login [*] -> github.com/altlimit/restruct_test.User.Login(context.Context, struct { Username string "json:\"username\" form:\"username\""; Password string "json:\"password\" form:\"password\"" }) (bool, error)`
	found := strings.Trim(buf.String(), "\n")
	if routes != found {
		t.Errorf("wanted \n%s\n routes got \n%s\n", routes, found)
	}
	jh := map[string]string{"Content-Type": "application/json"}
	table := []struct {
		method   string
		path     string
		request  string
		headers  map[string]string
		response string
		status   int
	}{
		{http.MethodPost, "/users/login", `{"username": "admin", "password": "admin"}`, jh, `true`, 200},
		{http.MethodPost, "/users/login", `{}`, jh, `{"error":"Invalid login"}`, 403},
		{http.MethodPost, "/users/login", `{`, jh, `{"error":"Bad Request"}`, 400},
		{http.MethodPost, "/users/login", `{"username": "admin", "password": "admin"}`, map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			`{"error":"Invalid login"}`, 403},
		{http.MethodPost, "/users/login", `username=admin&password=admin`, map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			`true`, 200},
		{http.MethodPost, "/blobs/download/abc", `{}`, jh, `{"error":"Method Not Allowed"}`, 405},
		{http.MethodGet, "/blobs/download/abc", `{}`, nil, `{"error":"Unauthorized"}`, 401},
		{http.MethodGet, "/blobs/download/abc/", `{}`, nil, `{"error":"Not Found"}`, 404},
		{http.MethodGet, "/blobs/download/abc", ``, map[string]string{"Authorization": "admin"}, `abc`, 200},
		{http.MethodGet, "/blobs/.custom/abc/123", ``, nil, `{"error":"Unauthorized"}`, 401},
		{http.MethodGet, "/blobs/.custom/abc/123/", ``, map[string]string{"Authorization": "admin"}, `"abc/123/"`, 200},
		{http.MethodPost, "/calc/add", `{"a":10,"b":20}`, jh, `30`, 200},
		{http.MethodPost, "/calc/subtract", `[20,10]`, jh, `10`, 200},
		{http.MethodPost, "/calc/subtract", `["bad"]`, jh, `{"error":"Bad Request"}`, 400},
		{http.MethodPost, "/calc/divide", `[10,2]`, jh, `5`, 200},
		{http.MethodPost, "/calc/divide", `[10,0]`, jh, `{"error":"Internal Server Error"}`, 500},
		{http.MethodPost, "/calc/multiply", `{"a":10,"b":2}`, jh, `20`, 200},
		{http.MethodPost, "/calc/multiply", `{"a":10,"b":2}`, nil, `{"error":"Unsupported Media Type"}`, 415},
		{http.MethodGet, "/users/execs", ``, nil, `---EXECS---`, 200},
		{http.MethodGet, "/", ``, nil, `"Welcome to Root"`, 200},
		{http.MethodGet, "/files/test.txt", ``, nil, `"test.txt"`, 200},
		{http.MethodGet, "/files/a/b/c/d", ``, nil, `"a/b/c/d"`, 200},
	}

	runs := 0
	for _, v := range table {
		if v.response == "---EXECS---" {
			v.response = fmt.Sprintf("%d", runs+1)
		}
		req := httptest.NewRequest(v.method, v.path, strings.NewReader(v.request))
		w := httptest.NewRecorder()

		if v.headers != nil {
			for k, v := range v.headers {
				req.Header.Add(k, v)
			}
		}

		h.ServeHTTP(w, req)

		res := w.Result()
		defer res.Body.Close()
		data, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Errorf("ioutil.ReadAll error %v", err)
		}
		resp := strings.TrimRight(string(data), "\n")
		if !(resp == v.response && res.StatusCode == v.status) {
			t.Errorf("path %s wanted %d `%s` got %d `%s`", v.path, v.status, v.response, res.StatusCode, resp)
		}
		if res.StatusCode != 404 && res.StatusCode != 405 {
			runs++
		}
	}
}
