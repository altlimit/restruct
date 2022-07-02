package restruct

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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

func TestDefaultReaderRead(t *testing.T) {
	h := NewHandler(&sampleService{})

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
