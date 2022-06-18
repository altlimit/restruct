package restruct

import (
	"net/http"
)

// Handle registers a struct  or a *Handler for the given pattern in the http.DefaultServeMux.
func Handle(pattern string, svc interface{}) {
	h, ok := svc.(*Handler)
	if ok {
		h.mustCompile(pattern)
		http.Handle(pattern, h)
		return
	}
	h = NewHandler(svc)
	h.mustCompile(pattern)
	http.Handle(pattern, h)
}
