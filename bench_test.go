package restruct

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type testService struct{}

func (ts *testService) Hello(r *http.Request) {}

type testService2 struct{}

func (ts *testService2) Hello_0(r *http.Request) {}

type testService3 struct{}

func (ts *testService3) Routes() map[string]string {
	return map[string]string{"Hello": "{v1}/{v2}/{v3}/{v4}/{v5}"}
}
func (ts *testService2) Hello(r *http.Request) {}

func BenchmarkHandler(b *testing.B) {
	h := NewHandler(&testService{})
	h.mustCompile("/api/v1")

	request, _ := http.NewRequest("GET", "/api/v1/hello", nil)
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(nil, request)
	}
}

func BenchmarkHandlerWithParam(b *testing.B) {
	h := NewHandler(&testService2{})
	h.mustCompile("/api/v1")

	requestA, _ := http.NewRequest("GET", "/api/v1/hello/1", nil)
	requestB, _ := http.NewRequest("GET", "/api/v1/hello/2", nil)
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(nil, requestA)
		h.ServeHTTP(nil, requestB)
	}
}

func BenchmarkWithManyParams(b *testing.B) {
	h := NewHandler(&testService3{})
	h.mustCompile("/api/v1")

	matchingRequest, _ := http.NewRequest("GET", "/api/v1/1/2/3/4/5", nil)
	notMatchingRequest, _ := http.NewRequest("GET", "/api/v1/1/2/3/4", nil)
	recorder := httptest.NewRecorder()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(nil, matchingRequest)
		h.ServeHTTP(recorder, notMatchingRequest)
	}
}
