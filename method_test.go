package restruct

import (
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
}

type serviceB struct {
	Delta serviceC
}

type serviceC struct{}

func (s *serviceA) Hello(r *http.Request)                             {}
func (s *serviceB) World(w http.ResponseWriter)                       {}
func (s *serviceC) HelloWorld(r *http.Request, w http.ResponseWriter) {}
func (s serviceC) Hello_World(w http.ResponseWriter, r *http.Request) {}
func (s serviceC) Overwrite()                                         {}
func (s *serviceC) Routes() map[string]string {
	return map[string]string{
		"Overwrite": ".custom/{pid}/_download_",
	}
}

func TestServiceToMethods(t *testing.T) {
	s1 := &serviceA{Charlie: &serviceB{}}

	routes := map[string][]reflect.Type{
		"s1/hello":                                  {typeHttpRequest},
		"s1/my/{tag}/world":                         {typeHttpWriter},
		"s1/my/{tag}/delta/hello-world":             {typeHttpRequest, typeHttpWriter},
		"s1/my/{tag}/delta/hello/world":             {typeHttpWriter, typeHttpRequest},
		"s1/charlie/world":                          {typeHttpWriter},
		"s1/charlie/delta/hello-world":              {typeHttpRequest, typeHttpWriter},
		"s1/charlie/delta/hello/world":              {typeHttpWriter, typeHttpRequest},
		"s1/charlie/delta/.custom/{pid}/_download_": {},
	}
	methods := serviceToMethods("s1/", s1)
	for _, m := range methods {
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
		{"with/regex/{tag:\\d+}/{hello}", "with/regex/123/world", true, map[string]string{"tag": "123", "hello": "world"}},
		{"path/{tag}/hello/{0}", "path/Anything/hello/129", true, map[string]string{"tag": "Anything", "0": "129"}},
		{"catch/{all:.+}", "catch/hello/world/caught/all", true, map[string]string{"all": "hello/world/caught/all"}},
		{"test/{a}", "test/", false, map[string]string{}},
	}

	for _, v := range table {
		m := &method{path: v.path}
		m.mustParse()
		params, ok := m.match(v.test)
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
