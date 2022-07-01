package restruct

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

type (
	sampleService struct{}

	addRequest struct {
		A int64 `json:"a" form:"b"`
		B int64 `json:"b" form:"b"`
	}
)

func (ss *sampleService) Add(r *http.Request) int64 {
	return 5
}

func TestDefaultReaderRead(t *testing.T) {
	h := NewHandler(&sampleService{})

	req := httptest.NewRequest(http.MethodGet, "/add", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll error %v", err)
	}
	t.Errorf("%s", data)
}
