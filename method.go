package restruct

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

const (
	paramRequest  = "request"
	paramResponse = "response"
)

var (
	pathToRe = regexp.MustCompile(`({\w+})`)
)

type (
	method struct {
		name    string
		prefix  string
		source  reflect.Value
		path    string
		pathRe  *regexp.Regexp
		params  []string
		returns []reflect.Kind
	}
)

// returns methods from structs and nested structs
func serviceToMethods(prefix string, svc interface{}) (methods []*method) {
	tv := reflect.TypeOf(svc)
	vv := reflect.ValueOf(svc)

	// get methods first
	tvt := vv.NumMethod()
	for i := 0; i < tvt; i++ {
		m := tv.Method(i)
		mm := &method{
			name:   m.Name,
			prefix: prefix,
			source: vv.Method(i),
		}
		mm.mustParse()
		methods = append(methods, mm)
	}

	if tv.Kind() == reflect.Ptr {
		tv = tv.Elem()
		vv = vv.Elem()
	}
	// check fields
	tvt = vv.NumField()
	for i := 0; i < tvt; i++ {
		f := tv.Field(i)
		if f.IsExported() {
			route := f.Tag.Get("route")
			if route != "-" {
				fk := f.Type.Kind()
				fv := vv.Field(i)
				if fk == reflect.Ptr {
					fk = f.Type.Elem().Kind()
					fv = fv.Elem()
				}
				if fk == reflect.Struct && fv.IsValid() {
					if route == "" {
						route = nameToPath(f.Name)
					}
					route = strings.Trim(route, "/") + "/"
					sv := fv.Addr().Interface()
					methods = append(methods, serviceToMethods(prefix+route, sv)...)
				}
			}
		}
	}
	return
}

// Converts a Name into a path route like:
// HelloWorld -> hello-world
// Hello_World -> hello_world
// Hello_0 -> hello/{0}
// Hello_0_World -> hello/{0}/world
func nameToPath(name string) string {
	var buf bytes.Buffer
	nt := len(name)
	skipDash := false
	for i := 0; i < nt; i++ {
		c := rune(name[i])
		if unicode.IsUpper(c) {
			if i > 0 && !skipDash {
				buf.WriteRune('-')
			}
			c = unicode.ToLower(c)
			buf.WriteRune(c)
			skipDash = false
		} else if c == '_' {
			buf.WriteRune('/')
			skipDash = true
		} else {
			if skipDash && unicode.IsNumber(c) {
				buf.WriteString(fmt.Sprintf("{%c}", c))
			} else {
				buf.WriteRune(c)
			}
			skipDash = false
		}
	}
	return buf.String()
}

// Populates method fields, if there's no params it will leave pathRe nil and
// directly compare path with equality.
func (m *method) mustParse() {
	m.path = m.prefix + nameToPath(m.name)
	rePath := m.path
	params := pathToRe.FindAllString(m.path, -1)
	if len(params) > 0 {
		for _, m := range params {
			rePath = strings.ReplaceAll(rePath, m, fmt.Sprintf(`(?P<%s>\w+)`, m[1:len(m)-1]))
		}
		rePath = "^" + rePath + "$"
		m.pathRe = regexp.MustCompile(rePath)
	}

	if m.source.IsValid() {
		mt := m.source.Type()
		for i := 0; i < mt.NumOut(); i++ {
			m.returns = append(m.returns, mt.Out(i).Kind())
		}
		for i := 0; i < mt.NumIn(); i++ {
			in := mt.In(i)
			if in == reflect.TypeOf(&http.Request{}) {
				m.params = append(m.params, paramRequest)
				continue
			} else {
				rwType := reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()
				if in.Implements(rwType) {
					m.params = append(m.params, paramResponse)
					continue
				}
			}
			panic("parameter " + in.Name() + " not supported in method " + m.name)
		}
	}

}

// Checks path against method if it's valid, this accepts a stripped path and not a full path
func (m *method) match(path string) (params map[string]string, ok bool) {
	params = make(map[string]string)
	if m.pathRe != nil {
		match := m.pathRe.FindStringSubmatch(path)
		if len(match) > 0 {
			for i, name := range m.pathRe.SubexpNames() {
				if i != 0 && name != "" {
					params[name] = match[i]
				}
			}
			ok = true
		}
	} else if m.path == path {
		ok = true
	}
	return
}
