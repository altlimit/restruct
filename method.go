package restruct

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

var (
	pathToRe = regexp.MustCompile(`{[^}]+}`)

	typeHttpRequest              = reflect.TypeOf(&http.Request{})
	typeHttpWriter               = reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()
	typeContext                  = reflect.TypeOf((*context.Context)(nil)).Elem()
	typeError                    = reflect.TypeOf((*error)(nil)).Elem()
	typeInt                      = reflect.TypeOf((*int)(nil)).Elem()
	typeMultipartFileHeader      = reflect.TypeOf(&multipart.FileHeader{})
	typeMultipartFileHeaderSlice = reflect.TypeOf([]*multipart.FileHeader{})
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
		middlewares []Middleware
	}
)

// returns methods from structs and nested structs
func serviceToMethods(prefix string, svc interface{}) (methods []*method) {
	tv := reflect.TypeOf(svc)
	vv := reflect.ValueOf(svc)

	// get methods first
	routes := make(map[string][]Route)
	skipMethods := map[string]bool{}
	if router, ok := svc.(Router); ok {
		for _, route := range router.Routes() {
			routes[route.Handler] = append(routes[route.Handler], route)
		}
		skipMethods["Routes"] = true
	}
	if _, ok := svc.(Init); ok {
		skipMethods["Init"] = true
	}
	var middlewares []Middleware
	if mws, ok := svc.(Middlewares); ok {
		middlewares = mws.Middlewares()
		skipMethods["Middlewares"] = true
	}
	tvt := vv.NumMethod()
	tvEl := tv
	if tv.Kind() == reflect.Ptr {
		tvEl = tv.Elem()
	}
	location := tvEl.PkgPath() + "." + tvEl.Name()
	for i := 0; i < tvt; i++ {
		m := tv.Method(i)
		// Skip interface methods
		if _, ok := skipMethods[m.Name]; ok {
			continue
		}
		mm := &method{
			location:    location + "." + m.Name,
			source:      vv.Method(i),
			middlewares: middlewares,
		}
		if len(routes) > 0 {
			rts, ok := routes[m.Name]
			if ok {
				for _, route := range rts {
					mr := &method{
						location:    mm.location,
						source:      mm.source,
						middlewares: mm.middlewares,
					}
					mr.middlewares = append(mr.middlewares, route.Middlewares...)
					if route.Path != "" {
						if route.Path == "." {
							mr.path = strings.TrimRight(prefix, "/")
						} else {
							mr.path = prefix + strings.TrimLeft(route.Path, "/")
						}
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
				continue
			}
		}
		mm.path = prefix + nameToPath(m.Name)
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
	startParam := false
	for i := 0; i < nt; i++ {
		c := rune(name[i])
		if !startParam && unicode.IsUpper(c) {
			if i > 0 && !skipDash {
				buf.WriteRune('-')
			}
			c = unicode.ToLower(c)
			buf.WriteRune(c)
			skipDash = false
		} else if c == '_' {
			if startParam {
				buf.WriteRune('}')
				startParam = false
			}
			buf.WriteRune('/')
			skipDash = true
		} else {
			if !startParam && skipDash && unicode.IsNumber(c) {
				startParam = true
				buf.WriteString(fmt.Sprintf("{%c", c))
			} else {
				buf.WriteRune(c)
			}
			if !startParam {
				skipDash = false
			}
		}
	}
	if startParam {
		buf.WriteRune('}')
	}
	return buf.String()
}

// Populates method fields, if there's no params it will leave pathRe nil and
// directly compare path with equality.
func (m *method) mustParse() {
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
		if mt.Kind() != reflect.Func {
			panic("method must be of type func")
		}
		for i := 0; i < mt.NumOut(); i++ {
			m.returns = append(m.returns, mt.Out(i))
		}
		for i := 0; i < mt.NumIn(); i++ {
			m.params = append(m.params, mt.In(i))
		}
	}
}
