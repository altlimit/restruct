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
		{"Products_0", `products/{0}`},
		{"Products_0_1", `products/{0}/{1}`},
		{"Products_0_UserX_1", `products/{0}/user-x/{1}`},
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
