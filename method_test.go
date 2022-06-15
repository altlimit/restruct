package restruct

import "testing"

func TestMethodPath(t *testing.T) {
	table := []struct {
		method string
		path   string
	}{
		{"Add", "add"},
		{"UserAuth", "user-auth"},
		{"Hello_World", "hello/world"},
		{"UserAuth_Bad", "user-auth/bad"},
		{"Products_0", `products/(?P<0>\w+)`},
		{"Products_0_1", `products/(?P<0>\w+)/(?P<1>\w+)`},
		{"Products_0_UserX_1", `products/(?P<0>\w+)/user-x/(?P<1>\w+)`},
	}

	for _, v := range table {
		m := &method{name: v.method}
		m.mustParse()
		p := m.path
		if v.path != p {
			t.Errorf("got path %s want %s", p, v.path)
		}
	}
}
