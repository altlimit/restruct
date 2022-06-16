package restruct

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"

	"github.com/altlimit/restruct/structtag"
)

func Params(r *http.Request) map[string]string {
	if params, ok := r.Context().Value(keyParams).(map[string]string); ok {
		return params
	}
	return map[string]string{}
}

func Query(r *http.Request, name string) string {
	if v, ok := r.URL.Query()[name]; ok && len(v) > 0 {
		return v[0]
	}
	return ""
}

// Bind checks for valid methods and tries to bind query strings and body into struct
func Bind(r *http.Request, out interface{}, methods ...string) error {
	if len(methods) > 0 {
		found := false
		for _, m := range methods {
			if r.Method == m {
				found = true
				break
			}
		}
		if !found {
			return Error{Status: http.StatusMethodNotAllowed}
		}
	}
	if out == nil {
		return nil
	}
	if err := BindQuery(r, out); err != nil {
		return err
	}
	if r.Method == http.MethodGet {
		return nil
	}
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		return fmt.Errorf("Bind: ioutil.ReadAll error %v", err)
	}
	if err := r.Body.Close(); err != nil {
		return fmt.Errorf("Bind: r.Body.Close error %v", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return Error{
			Status: http.StatusBadRequest,
			Err:    fmt.Errorf("Bind: json.Unmarshal error %v", err),
		}
	}
	return nil
}

func BindQuery(r *http.Request, out interface{}) error {
	t := reflect.TypeOf(out)
	v := reflect.ValueOf(out)
	if t.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	intSlice := reflect.TypeOf([]int{})
	int64Slice := reflect.TypeOf([]int64{})
	stringSlice := reflect.TypeOf([]string{})
	toTypeSlice := func(vals reflect.Value, sliceType reflect.Type) interface{} {
		if sliceType == stringSlice {
			return vals.Interface()
		}
		newVals := reflect.New(sliceType).Elem()
		for i := 0; i < vals.Len(); i++ {
			val := vals.Index(i).String()
			var v interface{}
			switch sliceType {
			case intSlice:
				v, _ = strconv.Atoi(val)
			case int64Slice:
				v, _ = strconv.ParseInt(val, 10, 64)
			default:
				v = nil
			}
			if v != nil {
				newVals = reflect.Append(newVals, reflect.ValueOf(v))
			}
		}
		return newVals.Interface()
	}
	for tag, field := range structtag.GetFieldsByTag(out, "query") {
		if q := Query(r, tag); q != "" {
			vv := v.Field(field.Index)
			vk := vv.Kind()
			if vk != reflect.String {
				var val interface{}
				switch vk {
				case reflect.Int:
					val, _ = strconv.Atoi(q)
				case reflect.Int64:
					val, _ = strconv.ParseInt(q, 10, 64)
				case reflect.Slice:
					val = toTypeSlice(reflect.ValueOf(r.URL.Query()[tag]), vv.Type())
				}
				vv.Set(reflect.ValueOf(val))
			} else {
				vv.Set(reflect.ValueOf(q))
			}
		}
	}
	return nil
}
