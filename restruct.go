package restruct

import (
	"net/http"
)

// Handle adds new service to a route.
func Handle(pattern string, handler *Handler) {
	handler.mustCompile(pattern)
	http.Handle(pattern, handler)
}
