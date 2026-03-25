package restruct_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/altlimit/restruct"
)

type (
	sampleService struct{}

	addRequest struct {
		A int64 `json:"a" form:"a"`
		B int64 `json:"b" form:"b"`
	}
)

func (ss *sampleService) Add(ctx context.Context, r *addRequest, x map[string]int64, y float64, z int) int64 {
	var total int64
	for _, v := range x {
		total += v
	}
	return total + r.A + r.B + int64(y) + int64(z)
}

func (ss *sampleService) Add2(ctx context.Context, r *addRequest) (int64, error) {
	return r.A + r.B, nil
}

func (ss *sampleService) SaveMap(data map[string]any) map[string]any {
	return data
}

func (ss *sampleService) SaveSlice(data []int64) int64 {
	var total int64
	for _, v := range data {
		total += v
	}
	return total
}

type searchRequest struct {
	Q    string `json:"q" query:"q"`
	Page int    `json:"page" query:"page"`
}

func (ss *sampleService) Search(r searchRequest) map[string]any {
	return map[string]any{"q": r.Q, "page": r.Page}
}

func TestDefaultReaderRead(t *testing.T) {
	h := restruct.NewHandler(&sampleService{})

	bod := `[{"a":10,"b":20}, {"S":30,"d":50}, 100, 200]`
	req := httptest.NewRequest(http.MethodPost, "/add", strings.NewReader(bod))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll error %v", err)
	}
	if strings.TrimRight(string(data), "\n") != "410" {
		t.Errorf("wanted 410 got %s", data)
	}

	ubod := url.Values{"a": {"4"}, "b": {"3"}}
	req = httptest.NewRequest(http.MethodPost, "/add2", strings.NewReader(ubod.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()

	h.ServeHTTP(w, req)

	res = w.Result()
	defer res.Body.Close()
	data, err = ioutil.ReadAll(res.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll error %v", err)
	}
	if strings.TrimRight(string(data), "\n") != "7" {
		t.Errorf("wanted 7 got %s", data)
	}
}

func TestDefaultReaderMapSlice(t *testing.T) {
	h := restruct.NewHandler(&sampleService{})
	jh := map[string]string{"Content-Type": "application/json"}

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		headers  map[string]string
		wantBody string
		wantCode int
	}{
		// map[string]any with JSON body - should bind directly (no array wrapper needed)
		{
			name:     "json map body",
			method:   http.MethodPost,
			path:     "/save-map",
			body:     `{"name":"test","value":42}`,
			headers:  jh,
			wantBody: `{"name":"test","value":42}`,
			wantCode: 200,
		},
		// []int64 with JSON body - should bind directly
		{
			name:     "json slice body",
			method:   http.MethodPost,
			path:     "/save-slice",
			body:     `[10,20,30]`,
			headers:  jh,
			wantBody: `60`,
			wantCode: 200,
		},
		// struct with query params still works
		{
			name:     "struct with query params",
			method:   http.MethodGet,
			path:     "/search?q=hello&page=2",
			body:     ``,
			headers:  nil,
			wantBody: `{"page":2,"q":"hello"}`,
			wantCode: 200,
		},
		// struct with JSON body still works
		{
			name:     "struct with json body",
			method:   http.MethodPost,
			path:     "/add2",
			body:     `{"a":5,"b":3}`,
			headers:  jh,
			wantBody: `8`,
			wantCode: 200,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			res := w.Result()
			data, _ := ioutil.ReadAll(res.Body)
			res.Body.Close()
			body := strings.TrimRight(string(data), "\n")

			if res.StatusCode != tc.wantCode {
				t.Errorf("want status %d, got %d", tc.wantCode, res.StatusCode)
			}
			if body != tc.wantBody {
				t.Errorf("want body %q, got %q", tc.wantBody, body)
			}
		})
	}
}

