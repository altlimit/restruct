package restruct

import (
	"log"
	"net/http"
	"reflect"
)

type (
	Handler struct {
		prefix      string
		prefixLen   int
		services    map[string]interface{}
		methodCache map[string]*method
		writers     map[string]ResponseWriter
	}
)

func (h *Handler) updateCache() {
	h.methodCache = make(map[string]*method)
	for k, v := range h.services {
		tv := reflect.TypeOf(v)
		vv := reflect.ValueOf(v)
		tvt := vv.NumMethod()
		for i := 0; i < tvt; i++ {
			m := tv.Method(i)
			mm := &method{
				name:   m.Name,
				source: vv.Method(i),
			}
			mm.mustParse()
			h.methodCache[k+mm.path] = mm
			log.Println(h.prefix + k + mm.path)
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
		if len(match) > 0 {
			log.Println("Found", v.name, match, path)
		} else {
			log.Println("NotFound", v.name, match, path)
		}
	}
	log.Println("Path", path)
}
