package restruct

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type (
	sampleService struct{}

	addRequest struct {
		A int64 `json:"a" form:"b"`
		B int64 `json:"b" form:"b"`
	}
)

func (ss *sampleService) Add(r *addRequest, x int64) int64 {
	return r.A + r.B
}

func TestDefaultReaderRead(t *testing.T) {
	h := NewHandler(&sampleService{})

	bod := `{"a":4,"b":3}`
	bod = `[{}, 500]`
	req := httptest.NewRequest(http.MethodPost, "/add", strings.NewReader(bod))
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
