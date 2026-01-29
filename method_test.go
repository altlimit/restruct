package restruct

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

func TestNameToPath(t *testing.T) {
	table := []struct {
		name string
		path string
	}{
		{"Add", "add"},
		{"UserAuth", "user-auth"},
		{"Hello_World", "hello/world"},
		{"UserAuth_Bad", "user-auth/bad"},
		{"Products_0", `products/{0}`},
		{"Products_0_1", `products/{0}/{1}`},
		{"Products_0_UserX_1", `products/{0}/user-x/{1}`},
		{"Products15", `products15`},
	}

	for _, v := range table {
		p := nameToPath(v.name)
		if v.path != p {
			t.Errorf("got path %s want %s", p, v.path)
		}
	}
}

type serviceA struct {
	Alpha   serviceB `route:"-"`
	Bravo   serviceB `route:"my/{tag}"`
	Charlie *serviceB
	Delta   *serviceB
	Echo    *serviceD
}

type serviceB struct {
	Delta serviceC
}

type serviceC struct{}

type serviceD struct{}

func (s *serviceA) Hello(r *http.Request)                             {}
func (s *serviceB) World(w http.ResponseWriter)                       {}
func (s *serviceC) HelloWorld(r *http.Request, w http.ResponseWriter) {}
func (s serviceC) Hello_World(w http.ResponseWriter, r *http.Request) {}

func (s serviceD) Overwrite()             {}
func (s serviceD) Root(c context.Context) {}

func (s *serviceA) Path_0()        {}
func (s *serviceA) Path_0_1()      {}
func (s *serviceA) Path_0_Sub_1()  {}
func (s *serviceA) Link_0FP()      {}
func (s *serviceA) Link_0FP_0123() {}

func (s *serviceD) Routes() []Route {
	return []Route{
		{Handler: "Overwrite", Path: ".custom/{pid}/_download_"},
		{Handler: "Root", Path: "."},
	}
}

func TestServiceToMethods(t *testing.T) {
	s1 := &serviceA{Charlie: &serviceB{}, Echo: &serviceD{}}

	routes := map[string][]reflect.Type{
		"s1/hello":                         {typeHttpRequest},
		"s1/my/{tag}/world":                {typeHttpWriter},
		"s1/my/{tag}/delta/hello-world":    {typeHttpRequest, typeHttpWriter},
		"s1/my/{tag}/delta/hello/world":    {typeHttpWriter, typeHttpRequest},
		"s1/charlie/world":                 {typeHttpWriter},
		"s1/charlie/delta/hello-world":     {typeHttpRequest, typeHttpWriter},
		"s1/charlie/delta/hello/world":     {typeHttpWriter, typeHttpRequest},
		"s1/echo/.custom/{pid}/_download_": {},
		"s1/echo":                          {typeContext},
		"s1/path/{0}":                      {},
		"s1/path/{0}/{1}":                  {},
		"s1/path/{0}/sub/{1}":              {},
		"s1/link/{0FP}":                    {},
		"s1/link/{0FP}/{0123}":             {},
	}
	methods := serviceToMethods("s1/", s1)
	if len(methods) != len(routes) {
		t.Fatalf("expected %d methods got %d", len(routes), len(methods))
	}
	for _, m := range methods {
		if _, ok := routes[m.path]; !ok {
			t.Fatalf("route %s not found", m.path)
		}
		if len(m.params) != len(routes[m.path]) {
			t.Errorf("%s param mismatch expected %d got %d", m.path, len(routes[m.path]), len(m.params))
		}
		for i, v := range m.params {
			if v != routes[m.path][i] {
				t.Errorf("route mismatch %s", m.path)
			}
		}
	}
}

func TestMethodMustParseMatch(t *testing.T) {
	table := []struct {
		path   string
		test   string
		match  bool
		params map[string]string
	}{
		{"with/wildcard/{tag}/{hello*}", "with/wildcard/123/world", true, map[string]string{"tag": "123", "hello": "world"}},
		{"path/{tag}/hello/{0}", "path/Anything/hello/129", true, map[string]string{"tag": "Anything", "0": "129"}},
		{"catch/{all*}", "catch/hello/world/caught/all", true, map[string]string{"all": "hello/world/caught/all"}},
		{"test/{a}", "test/", false, map[string]string{}},
	}

	for _, v := range table {
		m := &method{path: v.path}
		m.mustParse()
		params, ok := matchPath(paramCache{path: m.path, pathParts: m.pathParts}, v.test)
		if ok != v.match {
			t.Errorf("path %s not match %s", v.path, v.test)
		}
		for k, p := range v.params {
			if p != params[k] {
				t.Errorf("got param %s want %s for %s in %s", params[k], p, k, v.path)
			}
		}
	}
}
