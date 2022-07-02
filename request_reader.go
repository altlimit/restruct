package restruct

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
)

type (
	// RequestReader is called for input for your method if your parameter contains
	// a things other than *http.Request, http.ResponseWriter, context.Context
	// you'll get a slice of types and you must return values corresponding to those types
	RequestReader interface {
		Read(*http.Request, []reflect.Type) ([]reflect.Value, error)
	}

	// DefaultReader processes request with json.Encoder, urlencoded form and multipart for structs
	// if it's just basic types it will be read from body as array such as [1, "hello", false]
	// you can overwrite bind to apply validation library, etc
	DefaultReader struct {
		Bind func(*http.Request, interface{}, ...string) error
	}
)

func (dr *DefaultReader) Read(r *http.Request, types []reflect.Type) (vals []reflect.Value, err error) {
	typeLen := len(types)
	vals = make([]reflect.Value, typeLen)

	if typeLen == 0 {
		return
	}

	// if types is just 1 and a struct, we simply Bind and return
	if typeLen == 1 && (types[0].Kind() == reflect.Struct ||
		types[0].Kind() == reflect.Ptr && types[0].Elem().Kind() == reflect.Struct) {
		var ptr bool
		arg := types[0]
		if arg.Kind() == reflect.Ptr {
			arg = arg.Elem()
			ptr = true
		}
		val := reflect.New(arg)
		err = dr.Bind(r, val.Interface())
		if err != nil {
			return
		}
		if !ptr {
			val = val.Elem()
		}
		vals[0] = val
		return
	}
	// otherwise we get request body as json array
	badRequest := func(s string, f ...interface{}) {
		err = Error{
			Status: http.StatusBadRequest,
			Err:    fmt.Errorf(s, f...),
		}
	}
	var params []json.RawMessage
	var body []byte
	body, err = ioutil.ReadAll(r.Body)
	if err != nil {
		err = fmt.Errorf("DefaultReader.Read: ioutil.ReadAll error %v", err)
		return
	}
	err = r.Body.Close()
	if err != nil {
		err = fmt.Errorf("DefaultReader.Read: r.Body.Close error %v", err)
		return
	}
	err = json.Unmarshal(body, &params)
	if err != nil {
		badRequest("DefaultReader.Read: json.Unmarshal error %v", err)
		return
	}
	if len(params) < typeLen {
		badRequest("DefaultReader.Read: missing params")
		return
	}
	for i := 0; i < typeLen; i++ {
		t := types[i]
		val := reflect.New(t)
		err = json.Unmarshal(params[i], val.Interface())
		if err != nil {
			badRequest("DefaultReader.Read: param %d must be %s (%v)", i, t, err)
			return
		}
		vals[i] = val.Elem()
	}
	return
}
