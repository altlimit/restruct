package restruct

import (
	"net/http"
	"testing"
)

type testService struct{}

func (ts *testService) Hello(r *http.Request) {}

type testService2 struct{}

func (ts *testService2) Hello_0(r *http.Request) {}

type testService3 struct{}

func (ts *testService3) Hello(r *http.Request) {}

func (ts *testService3) Routes() map[string]Route {
	return map[string]Route{"Hello": {Path: "{v1}/{v2}/{v3}/{v4}/{v5}"}}
}

type testService4 struct{}

func (ts *testService4) Hello(r *http.Request)   {}
func (ts *testService4) Hello_0(r *http.Request) {}
func (ts *testService4) World(r *http.Request)   {}

func (ts *testService4) Routes() map[string]Route {
	return map[string]Route{"Hello": {Path: "tags/{tag:.+}"}}
}

// goos: linux
// goarch: amd64
// pkg: github.com/altlimit/restruct
// cpu: Intel(R) Core(TM) i7-3770K CPU @ 3.50GHz
// BenchmarkHandlerStatic-8   	13060087	        95.04 ns/op	      64 B/op	       1 allocs/op
// PASS
// ok  	github.com/altlimit/restruct	1.342s
func BenchmarkHandlerStatic(b *testing.B) {
	h := NewHandler(&testService{})
	h.mustCompile("/api/v1")
	request, _ := http.NewRequest("GET", "/api/v1/hello", nil)
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(nil, request)
	}
}

// goos: linux
// goarch: amd64
// pkg: github.com/altlimit/restruct
// cpu: Intel(R) Core(TM) i7-3770K CPU @ 3.50GHz
// BenchmarkHandlerWithParam-8   	 2023248	       574.1 ns/op	     848 B/op	       6 allocs/op
// PASS
// ok  	github.com/altlimit/restruct	1.770s
func BenchmarkHandlerWithParam(b *testing.B) {
	h := NewHandler(&testService2{})
	h.mustCompile("/api/v1")

	requestA, _ := http.NewRequest("GET", "/api/v1/hello/1", nil)
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(nil, requestA)
	}
}

// goos: linux
// goarch: amd64
// pkg: github.com/altlimit/restruct
// cpu: Intel(R) Core(TM) i7-3770K CPU @ 3.50GHz
// BenchmarkWithManyParams-8   	 1621026	       686.5 ns/op	     848 B/op	       6 allocs/op
// PASS
// ok  	github.com/altlimit/restruct	1.871s
func BenchmarkWithManyParams(b *testing.B) {
	h := NewHandler(&testService3{})
	h.mustCompile("/api/v1")

	matchingRequest, _ := http.NewRequest("GET", "/api/v1/1/2/3/4/5", nil)
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(nil, matchingRequest)
	}
}

// goos: linux
// goarch: amd64
// pkg: github.com/altlimit/restruct
// cpu: Intel(R) Core(TM) i7-3770K CPU @ 3.50GHz
// BenchmarkMixedHandler-8   	  609625	      1913 ns/op	    1952 B/op	      17 allocs/op
// PASS
// ok  	github.com/altlimit/restruct	1.193s
func BenchmarkMixedHandler(b *testing.B) {
	h := NewHandler(&testService4{})
	h.mustCompile("/api/v1")

	matchingRequest, _ := http.NewRequest("GET", "/api/v1/tags/abc/123", nil)
	matchingRequest2, _ := http.NewRequest("GET", "/api/v1/hello/123", nil)
	matchingRequest3, _ := http.NewRequest("GET", "/api/v1/world", nil)
	notMatchingRequest, _ := http.NewRequest("GET", "/api/v1/world", nil)
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(nil, matchingRequest)
		h.ServeHTTP(nil, matchingRequest2)
		h.ServeHTTP(nil, matchingRequest3)
		h.ServeHTTP(nil, notMatchingRequest)
	}
}

// goos: linux
// goarch: amd64
// pkg: github.com/altlimit/restruct
// cpu: Intel(R) Core(TM) i7-3770K CPU @ 3.50GHz
// BenchmarkMatch-8   	 5562338	       212.0 ns/op	     336 B/op	       2 allocs/op
// PASS
// ok  	github.com/altlimit/restruct	1.407s
func BenchmarkMatch(b *testing.B) {
	m := &method{path: "catch/{all}"}
	m.mustParse()
	for i := 0; i < b.N; i++ {
		m.match("catch/hello")
	}
}
