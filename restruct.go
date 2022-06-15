package restruct

import (
	"net/http"
	"strings"
)

func Handle(pattern string, handler *Handler) {
	if !strings.HasSuffix(pattern, "/") {
		pattern += "/"
	}
	handler.prefix = pattern
	handler.updateCache()
	http.Handle(pattern, handler)
}
