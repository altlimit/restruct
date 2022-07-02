package restruct

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

var (
	pathToRe = regexp.MustCompile(`{[^}]+}`)

	typeHttpRequest = reflect.TypeOf(&http.Request{})
	typeHttpWriter  = reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()
	typeContext     = reflect.TypeOf((*context.Context)(nil)).Elem()
	typeError       = reflect.TypeOf((*error)(nil)).Elem()
)

type (
	method struct {
		location    string
		source      reflect.Value
		path        string
		pathRe      *regexp.Regexp
		pathParts   []string
		params      []reflect.Type
		returns     []reflect.Type
		methods     map[string]bool
		middlewares []middleware
	}
)

// returns methods from structs and nested structs
func serviceToMethods(prefix string, svc interface{}) (methods []*method) {
	tv := reflect.TypeOf(svc)
	vv := reflect.ValueOf(svc)

	// get methods first
	var routes []Route
	hasRoutes := false
	if router, ok := svc.(Router); ok {
		routes = router.Routes()
		hasRoutes = true
	}
	tvt := vv.NumMethod()
	tvEl := tv
	if tv.Kind() == reflect.Ptr {
		tvEl = tv.Elem()
	}
	location := tvEl.PkgPath() + "." + tvEl.Name()
	for i := 0; i < tvt; i++ {
		m := tv.Method(i)
		// Skip Routes method if it implements Router interface{}
		if hasRoutes && m.Name == "Routes" {
			continue
		}
		mm := &method{
			location: location + "." + m.Name,
			source:   vv.Method(i),
		}
		if len(routes) > 0 {
			foundRoute := false
			for _, route := range routes {
				if route.Handler != m.Name {
					continue
				}
				mr := &method{
					location: mm.location,
					source:   mm.source,
				}
				foundRoute = true
				mr.middlewares = route.Middlewares
				if route.Path != "" {
					mr.path = prefix + strings.TrimLeft(route.Path, "/")
				} else {
					mr.path = prefix + nameToPath(m.Name)
				}
				if len(route.Methods) > 0 {
					mr.methods = make(map[string]bool)
					for _, method := range route.Methods {
						mr.methods[method] = true
					}
				}
				mr.mustParse()
				methods = append(methods, mr)
			}
			if foundRoute {
				continue
			}
		} else {
			mm.path = prefix + nameToPath(m.Name)
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
		if f.PkgPath != "" {
			continue
		}
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
	if m.path == "" {
		panic("path not provided")
	}
	rePath := m.path
	params := pathToRe.FindAllString(m.path, -1)
	if len(params) > 0 {
		withRe := false
		for _, m := range params {
			ex := fmt.Sprintf(`(?P<%s>\w+)`, m[1:len(m)-1])
			if idx := strings.Index(m, ":"); idx != -1 {
				ex = fmt.Sprintf(`(?P<%s>%s)`, m[1:idx], m[idx+1:len(m)-1])
				withRe = true
			}
			rePath = strings.ReplaceAll(rePath, m, ex)
		}
		if withRe {
			rePath = "^" + rePath + "$"
			m.pathRe = regexp.MustCompile(rePath)
		} else {
			m.pathParts = strings.Split(m.path, "/")
		}
	}

	if m.source.IsValid() {
		mt := m.source.Type()
		for i := 0; i < mt.NumOut(); i++ {
			m.returns = append(m.returns, mt.Out(i))
		}
		for i := 0; i < mt.NumIn(); i++ {
			m.params = append(m.params, mt.In(i))
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
	} else if m.pathParts != nil {
		// match by parts
		idx := -1
		pt := len(m.pathParts)
		for {
			idx++
			if idx+1 > pt {
				return
			}
			i := strings.Index(path, "/")
			var part string
			if i == -1 {
				part = path[i+1:]
				if part == "" {
					return
				}
			} else {
				part = path[:i]
			}
			mPart := m.pathParts[idx]
			if mPart[0] == '{' {
				params[mPart[1:len(mPart)-1]] = part
			} else if mPart != part {
				return
			}
			if i == -1 {
				break
			}
			path = path[i+1:]
		}
		ok = idx+1 == pt
	}
	return
}
