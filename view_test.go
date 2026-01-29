package restruct

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"testing/fstest"
	"time"
)

func TestView_Write_DataMerging(t *testing.T) {
	// Mock FS
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte(`Data: {{ .Data.DataFromMethod }}
Request: {{ .Request.URL.Path }}
`),
		},
	}

	// Mock Handler returning struct
	handlerData := struct {
		DataFromMethod string
	}{
		DataFromMethod: "StructValue",
	}

	v := &View{
		FS: fsys,
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Reflect types and values as if coming from Handler
	types := []reflect.Type{reflect.TypeOf(handlerData)}
	vals := []reflect.Value{reflect.ValueOf(handlerData)}

	v.Write(w, req, types, vals)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()

	// Check if data from struct is present via .Data wrapper
	if !contains(body, "Data: StructValue") {
		t.Errorf("Expected 'Data: StructValue' in body, got: %s", body)
	}

	// Check if request is present via .Request wrapper
	if !contains(body, "Request: /") {
		t.Errorf("Expected 'Request: /' in body, got: %s", body)
	}
}

func TestView_Cache_Invalidation(t *testing.T) {
	fsys := fstest.MapFS{
		"layout/main.html": &fstest.MapFile{
			Data:    []byte(`{{define "layout"}}{{.}}{{end}}`),
			ModTime: time.Now().Add(-1 * time.Hour),
		},
		"index.html": &fstest.MapFile{
			Data:    []byte(`{{template "layout" "Hello"}}`),
			ModTime: time.Now().Add(-1 * time.Hour),
		},
	}

	v := &View{
		FS:      fsys,
		Layouts: []string{"layout/*.html"},
		Funcs:   template.FuncMap{},
	}

	t0 := time.Now().Add(-1 * time.Hour)
	// First load
	tmpl1, err := v.getTemplate("index.html", t0)
	if err != nil {
		t.Fatalf("First getTemplate failed: %v", err)
	}

	// Second load (cache hit)
	tmpl2, err := v.getTemplate("index.html", t0)
	if err != nil {
		t.Fatalf("Second getTemplate failed: %v", err)
	}

	if tmpl1 != tmpl2 {
		t.Error("Expected same template instance from cache")
	}

	// Update Layout
	newTime := time.Now()
	fsys["layout/main.html"] = &fstest.MapFile{
		Data:    []byte(`{{define "layout"}}UPDATED {{.}}{{end}}`),
		ModTime: newTime,
	}

	// Third load (invalidation)
	tmpl3, err := v.getTemplate("index.html", newTime)
	if err != nil {
		t.Fatalf("Third getTemplate failed: %v", err)
	}

	if tmpl3 == tmpl1 {
		t.Error("Expected new template instance after layout update")
	}
}

func TestView_Cache_Concurrency(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data:    []byte(`Hello`),
			ModTime: time.Now(),
		},
	}
	v := &View{
		FS:    fsys,
		Funcs: template.FuncMap{},
	}

	done := make(chan bool)
	go func() {
		v.getTemplate("index.html", time.Now())
		done <- true
	}()
	go func() {
		v.getTemplate("index.html", time.Now())
		done <- true
	}()

	<-done
	<-done
}

func TestView_Routes(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":     &fstest.MapFile{},
		"about.html":     &fstest.MapFile{},
		"posts/new.html": &fstest.MapFile{},
	}
	v := &View{FS: fsys}
	routes := v.Routes()
	if !sliceContains(routes, "/") { // index.html -> /
		t.Error("Expected / route")
	}
	if !sliceContains(routes, "/about") {
		t.Error("Expected /about route")
	}
	if !sliceContains(routes, "/posts/new") {
		t.Error("Expected /posts/new route")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr || (len(s) > len(substr) && contains(s[1:], substr))
}

func sliceContains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}
