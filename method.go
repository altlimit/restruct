package restruct

import (
	"context"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"
	"unicode"
)

var (
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
		Name          string
		location      string
		source        reflect.Value
		path          string
		pathParts     []string
		params        []reflect.Type
		returns       []reflect.Type
		methods       map[string]bool
		middlewares   []Middleware
		writer        ResponseWriter
		readerTypes   []reflect.Type // Pre-computed types for RequestReader
		readerIndexes []int          // Pre-computed indexes for RequestReader args
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
	// Check for Viewer
	var writer ResponseWriter
	if v, ok := svc.(Viewer); ok {
		writer = v.View()
		skipMethods["View"] = true
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
			Name:        m.Name,
			location:    location + "." + m.Name,
			source:      vv.Method(i),
			middlewares: middlewares,
			writer:      writer,
		}
		if len(routes) > 0 {
			rts, ok := routes[m.Name]
			if ok {
				for _, route := range rts {
					mr := &method{
						Name:        mm.Name,
						location:    mm.location,
						source:      mm.source,
						middlewares: mm.middlewares,
						writer:      mm.writer,
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
	var buf strings.Builder
	nt := len(name)
	if name == "Index" {
		return ""
	}
	if name == "Any" {
		return "{any*}"
	}
	if strings.HasSuffix(name, "_Any") {
		return nameToPath(strings.TrimSuffix(name, "_Any")) + "/{any*}"
	}
	skipDash := false
	startParam := false
	for i := 0; i < nt; i++ {
		c := rune(name[i])
		if !startParam && unicode.IsUpper(c) {
			if i > 0 && !skipDash {
				buf.WriteByte('-')
			}
			c = unicode.ToLower(c)
			buf.WriteRune(c)
			skipDash = false
		} else if c == '_' {
			if startParam {
				buf.WriteByte('}')
				startParam = false
			}
			buf.WriteByte('/')
			skipDash = true
		} else {
			if !startParam && skipDash && unicode.IsNumber(c) {
				startParam = true
				buf.WriteString("{")
				buf.WriteRune(c)
			} else {
				buf.WriteRune(c)
			}
			if !startParam {
				skipDash = false
			}
		}
	}
	if startParam {
		buf.WriteByte('}')
	}
	return buf.String()
}

// Populates method fields, if there's no params it will leave pathRe nil and
// directly compare path with equality.
func (m *method) mustParse() {
	if strings.Contains(m.path, "{") && strings.Contains(m.path, "}") {
		for _, p := range strings.Split(m.path, "/") {
			if p != "" {
				m.pathParts = append(m.pathParts, p)
			}
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
			t := mt.In(i)
			m.params = append(m.params, t)
			// Pre-compute which params need RequestReader
			switch t {
			case typeHttpRequest, typeHttpWriter, typeContext:
				// These are handled directly, not via RequestReader
			default:
				m.readerTypes = append(m.readerTypes, t)
				m.readerIndexes = append(m.readerIndexes, i)
			}
		}
	}
}
