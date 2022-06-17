package restruct

import (
	"net/http"
	"strings"
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

	routes := map[string][]string{
		"s1/hello":                                  {paramRequest},
		"s1/my/{tag}/world":                         {paramResponse},
		"s1/my/{tag}/delta/hello-world":             {paramRequest, paramResponse},
		"s1/my/{tag}/delta/hello/world":             {paramResponse, paramRequest},
		"s1/charlie/world":                          {paramResponse},
		"s1/charlie/delta/hello-world":              {paramRequest, paramResponse},
		"s1/charlie/delta/hello/world":              {paramResponse, paramRequest},
		"s1/charlie/delta/.custom/{pid}/_download_": {},
	}
	methods := serviceToMethods("s1/", s1)
	for _, m := range methods {
		if strings.Join(m.params, ",") != strings.Join(routes[m.path], ",") {
			t.Errorf("route mismatch %s", m.path)
		}
	}
}
