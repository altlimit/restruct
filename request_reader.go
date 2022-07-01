package restruct

import (
	"net/http"
	"reflect"
)

type (
	// RequestReader is called for input for your method if your parameter contains
	// a things other than *http.Request, http.ResponseWriter, context.Context
	// you'll get a slice of types and you must return values corresponding to those types
	RequestReader interface {
		Read(*http.Request, []reflect.Type) ([]interface{}, error)
	}

	// DefaultReader processes request with json.Encoder, urlencoded form and multipart
	DefaultReader struct {
	}
)

func (dr *DefaultReader) Read(r *http.Request, args []reflect.Type) (vals []interface{}, err error) {

	return
}
