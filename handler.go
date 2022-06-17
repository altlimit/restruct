package restruct

import (
	"context"
	"net/http"
	"reflect"
	"strings"
)

type (
	ctxKey string
)

const (
	keyParams ctxKey = "params"
)

type (
	middleware func(http.Handler) http.Handler

	Handler struct {
		prefix            string
		prefixLen         int
		services          map[string]interface{}
		writers           map[string]ResponseWriter
		methodCache       []*method
		methodCacheByPath map[string]*method
		middlewares       []middleware
	}

	wrappedHandler struct {
		handler http.Handler
	}
)

func (wh *wrappedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wh.handler.ServeHTTP(w, r)
}

// NewHandler creates a handler with a root service.
func NewHandler(rootService interface{}) *Handler {
	h := &Handler{
		services: map[string]interface{}{"": rootService},
	}
	return h
}

// Routes returns a list of routes registered
func (h *Handler) Routes() (routes []string) {
	h.updateCache()
	for _, m := range h.methodCache {
		routes = append(routes, h.prefix+m.path)
	}
	for p := range h.methodCacheByPath {
		routes = append(routes, h.prefix+p)
	}
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
	h.methodCache = nil
}

// AddWriter adds new writer by content type.
// The content type is not enforced and will use the first it finds when it doesn't
// match an Accept/Content-Type header.
func (h *Handler) AddWriter(contentType string, w ResponseWriter) {
	if h.writers == nil {
		h.writers = make(map[string]ResponseWriter)
	}
	h.writers[contentType] = w
}

// Use adds a middleware to your services.
func (h *Handler) Use(fns ...middleware) {
	h.middlewares = append(h.middlewares, fns...)
}

// ServeHTTP calls the method with the matched route.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[h.prefixLen:]
	var writer ResponseWriter
	if len(h.writers) == 0 {
		writer = &DefaultWriter{}
	} else {
		contentType := strings.Split(r.Header.Get("Accept"), ";")[0]
		if contentType == "" {
			contentType = strings.Split(r.Header.Get("Content-Type"), ";")[0]
		}
		wrtr, ok := h.writers[contentType]
		if !ok {
			for _, ww := range h.writers {
				wrtr = ww
				break
			}
		}
		writer = wrtr
	}
	// if there are middleware we wrap it in reverse so it's called
	// in the order they were added
	chain := func(m *method) *wrappedHandler {
		handler := &wrappedHandler{handler: h.createHandler(writer, m)}
		for i := len(h.middlewares) - 1; i >= 0; i-- {
			handler = &wrappedHandler{handler: h.middlewares[i](handler)}
		}
		return handler
	}
	if v, ok := h.methodCacheByPath[path]; ok {
		chain(v).ServeHTTP(w, r)
		return
	}
	for _, v := range h.methodCache {
		params, ok := v.match(path)
		if ok {
			if len(params) > 0 {
				ctx := r.Context()
				ctx = context.WithValue(ctx, keyParams, params)
				r = r.WithContext(ctx)
			}
			chain(v).ServeHTTP(w, r)
			return
		}
	}

	writer.Write(w, Error{Status: http.StatusNotFound})
}

// wrapped handler that calls the actual method and processes the returns
// the parameter allowed here are *http.Request and http.ResponseWriter
// the returns can be anything or an error which will be sent to the ResponseWriter
// a multiple return is passed as slice of interface{}
func (h *Handler) createHandler(writer ResponseWriter, m *method) http.Handler {
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
			writer.Write(w, out[0].Interface())
		} else {
			var outs []interface{}
			for _, o := range out {
				outs = append(outs, o.Interface())
			}
			writer.Write(w, outs)
		}
	})
}

// Called every time you add a handler to create a cached info about
// your routes and which methods it points to. This will also look up
// exported structs to add as a service. You can avoid this by adding
// route:"-" or to specify specific route add route:"path/{hello}"
func (h *Handler) updateCache() {
	if h.methodCache != nil {
		return
	}
	var cache []*method
	for k, v := range h.services {
		cache = append(cache, serviceToMethods(k, v)...)
	}
	h.methodCacheByPath = make(map[string]*method)
	for _, v := range cache {
		if v.pathRe == nil {
			_, ok := h.methodCacheByPath[v.path]
			if ok {
				panic("duplicate " + v.path + " registered")
			}
			h.methodCacheByPath[v.path] = v
		} else {
			h.methodCache = append(h.methodCache, v)
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
