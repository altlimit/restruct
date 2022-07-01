package restruct

import (
	"context"
	"net/http"
	"reflect"
	"sort"
	"strings"
)

type (
	ctxKey string
)

const (
	keyParams ctxKey = "params"
	keyVals   ctxKey = "vals"
)

type (
	middleware func(http.Handler) http.Handler

	Handler struct {
		// Writer controls the output of your service, defaults to DefaultWriter
		Writer ResponseWriter

		prefix      string
		prefixLen   int
		services    map[string]interface{}
		cache       *methodCache
		middlewares []middleware
	}

	methodCache struct {
		byParams []*method
		byPath   map[string][]*method
	}

	wrappedHandler struct {
		handler http.Handler
	}
)

func (mc *methodCache) methods() (methods []*method) {
	methods = append(methods, mc.byParams...)
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

// Routes returns a list of routes registered
func (h *Handler) Routes() (routes []string) {
	h.updateCache()
	for _, m := range h.cache.methods() {
		var methods []string
		for k := range m.methods {
			methods = append(methods, k)
		}
		routes = append(routes, m.location+":"+strings.Join(methods, ",")+" "+h.prefix+m.path)
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
func (h *Handler) Use(fns ...middleware) {
	h.middlewares = append(h.middlewares, fns...)
}

// ServeHTTP calls the method with the matched route.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[h.prefixLen:]
	if h.Writer == nil {
		h.Writer = &DefaultWriter{}
	}
	// if there are middleware we wrap it in reverse so it's called
	// in the order they were added
	chain := func(m *method) *wrappedHandler {
		handler := &wrappedHandler{handler: h.createHandler(m)}
		middlewares := append(h.middlewares, m.middlewares...)
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = &wrappedHandler{handler: h.middlewares[i](handler)}
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
		h.Writer.Write(w, r, Error{Status: http.StatusMethodNotAllowed})
		return
	}
	// we do heavier look up such as path parts or regex then if any match
	// we set path found but still need to match method for proper error return
	status := http.StatusNotFound
	for _, v := range h.cache.byParams {
		params, ok := v.match(path)
		if ok {
			if len(params) > 0 {
				ctx := r.Context()
				ctx = context.WithValue(ctx, keyParams, params)
				r = r.WithContext(ctx)
			}
			ok := v.methods == nil
			if !ok {
				_, ok = v.methods[r.Method]
			}
			if ok {
				runMethod(v)
				return
			}
			status = http.StatusMethodNotAllowed
		}
	}
	h.Writer.Write(w, r, Error{Status: status})
}

// wrapped handler that calls the actual method and processes the returns
// the parameter allowed here are *http.Request and http.ResponseWriter
// the returns can be anything or an error which will be sent to the ResponseWriter
// a multiple return is passed as slice of interface{}
func (h *Handler) createHandler(m *method) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var args []reflect.Value
		for _, v := range m.params {
			if v == paramRequest {
				args = append(args, reflect.ValueOf(r))
			} else if v == paramResponse {
				args = append(args, reflect.ValueOf(w))
			}
		}
		out := m.source.Call(args)
		ot := len(out)
		if ot == 0 {
			return
		} else if ot == 1 {
			h.Writer.Write(w, r, out[0].Interface())
		} else {
			var outs []interface{}
			for _, o := range out {
				outs = append(outs, o.Interface())
			}
			h.Writer.Write(w, r, outs)
		}
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
	for k, svc := range h.services {
		for _, v := range serviceToMethods(k, svc) {
			if v.pathRe != nil || v.pathParts != nil {
				h.cache.byParams = append(h.cache.byParams, v)
			} else {
				h.cache.byPath[v.path] = append(h.cache.byPath[v.path], v)
			}
		}
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
