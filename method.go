package restruct

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"unicode"
)

type (
	method struct {
		name    string
		source  reflect.Value
		path    string
		pathRe  *regexp.Regexp
		params  []string
		returns []reflect.Kind
	}
)

func (m *method) mustParse() {
	var buf bytes.Buffer
	nt := len(m.name)
	skipDash := false
	for i := 0; i < nt; i++ {
		c := rune(m.name[i])
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
				buf.WriteString(fmt.Sprintf(`(?P<%c>\w+)`, c))
			} else {
				buf.WriteRune(c)
			}
			skipDash = false
		}
	}
	m.path = buf.String()
	m.pathRe = regexp.MustCompile(m.path)

	if m.source.IsValid() {
		mt := m.source.Type()
		for i := 0; i < mt.NumOut(); i++ {
			m.returns = append(m.returns, mt.Out(i).Kind())
		}
		for i := 0; i < mt.NumIn(); i++ {
			in := mt.In(i)
			if in == reflect.TypeOf(&http.Request{}) {
				m.params = append(m.params, "request")
				continue
			} else {
				rwType := reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()
				if in.Implements(rwType) {
					m.params = append(m.params, "response")
					continue
				}
			}
			panic("parameter " + in.Name() + " not supported in method " + m.name)
		}
	}

}
