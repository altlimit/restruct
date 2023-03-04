package restruct

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

type (
	ctxKey string
)

var (
	ErrReaderReturnLen = errors.New("reader args len does not match")
)

const (
	keyParams ctxKey = "params"
	keyVals   ctxKey = "vals"
)

type (
	Middleware func(http.Handler) http.Handler

	Handler struct {
		// Writer controls the output of your service, defaults to DefaultWriter
		Writer ResponseWriter
		// Reader controls the input of your service, defaults to DefaultReader
		Reader RequestReader

		prefix      string
		prefixLen   int
		services    map[string]interface{}
		cache       *methodCache
		middlewares []Middleware
		notFound    *method
	}

	methodCache struct {
		byParams []paramCache
		byPath   map[string][]*method
	}

	paramCache struct {
		path      string
		pathParts []string
		pathRe    *regexp.Regexp
		methods   []*method
	}

	wrappedHandler struct {
		handler http.Handler
	}
)

func (mc *methodCache) methods() (methods []*method) {
	for _, param := range mc.byParams {
		methods = append(methods, param.methods...)
	}
	for _, m := range mc.byPath {
		methods = append(methods, m...)
	}
	return
}

func (wh *wrappedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wh.handler.ServeHTTP(w, r)
}

// NewHandler creates a handler for a given struct.
func NewHandler(svc interface{}) *Handler {
	h := &Handler{
		services: map[string]interface{}{"": svc},
	}
	h.mustCompile("")
	return h
}

// WithPrefix prefixes your service with given path. You can't use parameters here.
// This is useful if you want to register this handler with another third party router.
func (h *Handler) WithPrefix(prefix string) *Handler {
	h.mustCompile(prefix)
	return h
}

// Routes returns a list of routes registered and it's definition
func (h *Handler) Routes() (routes []string) {
	h.updateCache()
	for _, m := range h.cache.methods() {
		var methods []string
		for k := range m.methods {
			methods = append(methods, k)
		}
		if len(methods) == 0 {
			methods = append(methods, "*")
		}
		r := h.prefix + m.path + " [" + strings.Join(methods, ",") + "] -> " + m.location
		var params []string
		for _, v := range m.params {
			params = append(params, v.String())
		}
		var returns []string
		for _, v := range m.returns {
			returns = append(returns, v.String())
		}
		r += "(" + strings.Join(params, ", ") + ")"
		if len(returns) > 0 {
			r += " (" + strings.Join(returns, ", ") + ")"
		}
		routes = append(routes, r)
	}
	sort.Strings(routes)
	return
}

// AddService adds a new service to specified route.
// You can put {param} in this route.
func (h *Handler) AddService(path string, svc interface{}) {
	path = strings.TrimPrefix(path, "/")
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	if _, ok := h.services[path]; ok {
		panic("service " + path + " already exists")
	}
	h.services[path] = svc
	h.cache = nil
}

// Use adds a middleware to your services.
func (h *Handler) Use(fns ...Middleware) {
	h.middlewares = append(h.middlewares, fns...)
}

// NotFound sets the notFound handler and calls it
// if no route matches
func (h *Handler) NotFound(handler interface{}) {
	h.notFound = &method{source: reflect.ValueOf(handler)}
	h.notFound.mustParse()
}

// ServeHTTP calls the method with the matched route.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[h.prefixLen:]
	if h.Writer == nil {
		h.Writer = &DefaultWriter{}
	}
	if h.Reader == nil {
		h.Reader = &DefaultReader{Bind: Bind}
	}
	// if there are middleware we wrap it in reverse so it's called
	// in the order they were added
	chain := func(m *method) *wrappedHandler {
		handler := &wrappedHandler{handler: h.createHandler(m)}
		middlewares := append(h.middlewares, m.middlewares...)
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = &wrappedHandler{handler: middlewares[i](handler)}
		}
		return handler
	}
	runMethod := func(m *method) {
		chain(m).ServeHTTP(w, r)
	}
	// we check path look up first then see if proper method
	if vals, ok := h.cache.byPath[path]; ok {
		for _, m := range vals {
			ok := m.methods == nil
			if !ok {
				_, ok = m.methods[r.Method]
			}
			if ok {
				runMethod(m)
				return
			}
		}
		h.Writer.Write(w, r, refTypes(typeError), refVals(Error{Status: http.StatusMethodNotAllowed}))
		return
	}
	// we do heavier look up such as path parts or regex then if any match
	// we set path found but still need to match method for proper error return
	status := http.StatusNotFound
	for _, bp := range h.cache.byParams {
		params, ok := matchPath(bp, path)
		if ok {
			for _, v := range bp.methods {
				ok := v.methods == nil
				if !ok {
					_, ok = v.methods[r.Method]
				}
				if ok {
					if len(params) > 0 {
						ctx := r.Context()
						ctx = context.WithValue(ctx, keyParams, params)
						r = r.WithContext(ctx)
					}
					runMethod(v)
					return
				}
			}
			status = http.StatusMethodNotAllowed
		}
	}
	if status == http.StatusNotFound && h.notFound != nil {
		runMethod(h.notFound)
		return
	}
	h.Writer.Write(w, r, refTypes(typeError), refVals(Error{Status: status}))
}

// wrapped handler that calls the actual method and processes the returns
// the parameter allowed here are *http.Request and http.ResponseWriter
// the returns can be anything or an error which will be sent to the ResponseWriter
// a multiple return is passed as slice of interface{}
func (h *Handler) createHandler(m *method) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			argTypes   []reflect.Type
			argIndexes []int
		)
		args := make([]reflect.Value, len(m.params))
		for k, v := range m.params {
			switch v {
			case typeHttpRequest:
				args[k] = reflect.ValueOf(r)
			case typeHttpWriter:
				args[k] = reflect.ValueOf(w)
			case typeContext:
				args[k] = reflect.ValueOf(r.Context())
			default:
				argTypes = append(argTypes, v)
				argIndexes = append(argIndexes, k)
			}
		}
		// has unknown types in parameters, use RequestReader
		if len(argIndexes) > 0 {
			typeArgs, err := h.Reader.Read(r, argTypes)
			if err != nil {
				h.Writer.Write(w, r, refTypes(typeError), refVals(err))
				return
			}
			if len(typeArgs) != len(argIndexes) {
				h.Writer.Write(w, r, refTypes(typeError), refVals(Error{Err: ErrReaderReturnLen}))
				return
			}
			for k, i := range argIndexes {
				args[i] = typeArgs[k]
			}
		}
		out := m.source.Call(args)
		ot := len(out)
		if ot == 0 {
			return
		}
		h.Writer.Write(w, r, m.returns, out)
	})
}

// Called every time you add a handler to create a cached info about
// your routes and which methods it points to. This will also look up
// exported structs to add as a service. You can avoid this by adding
// route:"-" or to specify specific route add route:"path/{hello}"
func (h *Handler) updateCache() {
	if h.cache != nil {
		return
	}
	if h.prefix == "" {
		h.mustCompile("")
	}
	h.cache = &methodCache{
		byPath: make(map[string][]*method),
	}
	// cache all same paths so we only compare it once
	pathCache := make(map[string][]*method)
	// we store ordered paths so it's still looked up in order you enter it
	var orderedPaths []string
	for k, svc := range h.services {
		for _, v := range serviceToMethods(k, svc) {
			if v.pathRe != nil || v.pathParts != nil {
				_, ok := pathCache[v.path]
				if !ok {
					orderedPaths = append(orderedPaths, v.path)
				}
				pathCache[v.path] = append(pathCache[v.path], v)
			} else {
				h.cache.byPath[v.path] = append(h.cache.byPath[v.path], v)
			}
		}
	}
	for _, path := range orderedPaths {
		// all of methods here have the same path so we use first one
		m := pathCache[path][0]
		h.cache.byParams = append(h.cache.byParams, paramCache{
			path:      path,
			pathParts: m.pathParts,
			pathRe:    m.pathRe,
			methods:   pathCache[path],
		})
	}
}

func (h *Handler) mustCompile(prefix string) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	h.prefix = prefix
	h.prefixLen = len(h.prefix)
	h.updateCache()
}

// Checks path against request path if it's valid, this accepts a stripped path and not a full path
func matchPath(pc paramCache, path string) (params map[string]string, ok bool) {
	params = make(map[string]string)
	if pc.pathRe != nil {
		match := pc.pathRe.FindStringSubmatch(path)
		if len(match) > 0 {
			for i, name := range pc.pathRe.SubexpNames() {
				if i != 0 && name != "" {
					params[name] = match[i]
				}
			}
			ok = true
		}
	} else if pc.pathParts != nil {
		// match by parts
		idx := -1
		pt := len(pc.pathParts)
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
			mPart := pc.pathParts[idx]
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
