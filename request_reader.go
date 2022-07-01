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

func (dr *DefaultReader) Read(r *http.Request, args []reflect.Type) (vals []reflect.Value, err error) {
	argLen := len(args)
	vals = make([]reflect.Value, argLen)

	if argLen == 0 {
		return
	}

	// if args is just 1 and a struct, we simply Bind and return
	if argLen == 1 && (args[0].Kind() == reflect.Struct ||
		args[0].Kind() == reflect.Ptr && args[0].Elem().Kind() == reflect.Struct) {
		var ptr bool
		arg := args[0]
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
	var params []interface{}
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
	argVals := reflect.ValueOf(params)
	if argVals.Len() < argLen {
		badRequest("DefaultReader.Read: missing params")
		return
	}
	for i := 0; i < argLen; i++ {
		t := args[i]
		val := reflect.ValueOf(params[i])
		var ptr bool
		if t.Kind() == reflect.Ptr {
			ptr = true
			t = t.Elem()
		}
		tk := t.Kind()
		vk := val.Kind()
		// target type struct, slice, map will just be re-marshalled
		// otherwise convert numbers, since unmarshal uses float64
		if tk == reflect.Struct || tk == reflect.Slice || tk == reflect.Map {
			var b []byte
			b, err = json.Marshal(val.Interface())
			if err != nil {
				badRequest("DefaultReader.Read: json.Marshal error %v", err)
				return
			}
			val = reflect.New(t)
			err = json.Unmarshal(b, val.Interface())
			if err != nil {
				badRequest("DefaultReader.Read: json.Unmarshal error %v", err)
				return
			}
			if !ptr {
				val = val.Elem()
			}
		} else if (tk == reflect.Int64 || tk == reflect.Int) &&
			vk == reflect.Float64 {
			num := int64(val.Float())
			val = reflect.New(t).Elem()
			val.SetInt(num)
		} else {
			if val.Kind() != tk {
				badRequest("DefaultReader.Read: param %d must be %s", i+1, t)
				return
			}
		}
		vals[i] = val
	}
	return
}
