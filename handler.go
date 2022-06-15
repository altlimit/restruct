package restruct

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"unicode"
)

type (
	Handler struct {
		prefix      string
		services    map[string]interface{}
		methodCache map[string]map[string]*method
	}

	method struct {
		name   string
		source reflect.Value
	}
)

func (m *method) path() string {
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
				buf.WriteString(fmt.Sprintf(`(?P<%c>\+)`, c))
			} else {
				buf.WriteRune(c)
			}
			skipDash = false
		}
	}
	return buf.String()
}

func (h *Handler) updateCache() {
	for k, v := range h.services {
		if h.methodCache == nil {
			h.methodCache = make(map[string]map[string]*method)
		}
		_, ok := h.methodCache[k]
		if !ok {
			h.methodCache[k] = make(map[string]*method)
			tv := reflect.TypeOf(v)
			vv := reflect.ValueOf(v)
			tvt := vv.NumMethod()
			for i := 0; i < tvt; i++ {
				m := tv.Method(i)
				mm := &method{
					name:   m.Name,
					source: vv.Method(i),
				}
				h.methodCache[k][mm.path()] = mm
				log.Println(h.prefix + k + mm.path())
			}
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

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println("Path", r.URL.Path)
}
