package restruct

import (
	"encoding/json"
	"net/http"
)

type (
	ResponseWriter interface {
		Write(http.ResponseWriter, interface{})
	}

	DefaultWriter struct {
	}
)

func (dw *DefaultWriter) Write(w http.ResponseWriter, out interface{}) {
	if out == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(out); err != nil {
		panic(err)
	}
}
