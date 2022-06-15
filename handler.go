package restruct

import (
	"context"
	"log"
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
	Handler struct {
		prefix      string
		prefixLen   int
		services    map[string]interface{}
		methodCache []*method
		writers     map[string]ResponseWriter
	}
)

func (h *Handler) updateCache() {
	h.methodCache = make([]*method, 0)
	for k, v := range h.services {
		tv := reflect.TypeOf(v)
		vv := reflect.ValueOf(v)
		tvt := vv.NumMethod()
		for i := 0; i < tvt; i++ {
			m := tv.Method(i)
			mm := &method{
				name:   m.Name,
				prefix: k,
				source: vv.Method(i),
			}
			mm.mustParse()
			h.methodCache = append(h.methodCache, mm)
			log.Println(h.prefix + mm.path)
		}
	}
}

func NewHandler(rootService interface{}) *Handler {
	h := &Handler{
		services: map[string]interface{}{"": rootService},
	}
	return h
}

func (h *Handler) AddService(path string, svc interface{}) {
	path = strings.TrimPrefix(path, "/")
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	if _, ok := h.services[path]; ok {
		panic("service " + path + " already exists")
	}
	h.services[path] = svc
}

func (h *Handler) AddWriter(contentType string, w ResponseWriter) {
	if h.writers == nil {
		h.writers = make(map[string]ResponseWriter)
	}
	h.writers[contentType] = w
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[h.prefixLen:]
	for _, v := range h.methodCache {
		match := v.pathRe.FindStringSubmatch(path)
		tm := len(match)
		if tm > 0 {
			if tm > 1 {
				params := make(map[string]string)
				for i, name := range v.pathRe.SubexpNames() {
					if i != 0 && name != "" {
						params[name] = match[i]
					}
				}
				ctx := r.Context()
				ctx = context.WithValue(ctx, keyParams, params)
				r = r.WithContext(ctx)
			}
			log.Println("Found", v.name, match, path)
		}
	}
	log.Println("Path", path)
}
