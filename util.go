package restruct

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/altlimit/restruct/structtag"
)

// Params returns map of params from url path like /{param1} will be map[param1] = value
func Params(r *http.Request) map[string]string {
	return Vars(r.Context())
}

// Vars returns map of params from url from request context
func Vars(ctx context.Context) map[string]string {
	if params, ok := ctx.Value(keyParams).(map[string]string); ok {
		return params
	}
	return map[string]string{}
}

// Query returns a query string value
func Query(r *http.Request, name string) string {
	return r.URL.Query().Get(name)
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
	if len(r.URL.Query()) > 0 {
		if err := BindQuery(r, out); err != nil {
			return err
		}
	}
	if r.Method == http.MethodGet {
		return nil
	}
	cType := r.Header.Get("Content-Type")
	if idx := strings.Index(cType, ";"); idx != -1 {
		cType = cType[0:idx]
	}
	switch cType {
	case "application/json":
		return BindJson(r, out)
	case "application/x-www-form-urlencoded", "multipart/form-data":
		return BindForm(r, out)
	}
	return Error{Status: http.StatusUnsupportedMediaType}
}

// BindJson puts all json tagged values into struct fields
func BindJson(r *http.Request, out interface{}) error {
	body, err := ioutil.ReadAll(r.Body)
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

// BindQuery puts all query string values into struct fields with tag:"query"
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

// BindForm puts all struct fields with tag:"form" from a form request
func BindForm(r *http.Request, out interface{}) error {
	t := reflect.TypeOf(out)
	v := reflect.ValueOf(out)
	if t.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	cType := r.Header.Get("Content-Type")
	formValues := make(map[string]interface{})
	if strings.HasPrefix(cType, "application/x-www-form-urlencoded") {
		r.ParseForm()
		for k := range r.PostForm {
			formValues[k] = r.PostFormValue(k)
		}
	} else if strings.Contains(cType, "multipart/form-data") {
		r.ParseMultipartForm(32 << 20)
		for k := range r.PostForm {
			formValues[k] = r.FormValue(k)
		}
		for k, v := range r.MultipartForm.File {
			if strings.HasSuffix(k, "[]") {
				formValues[k[:len(k)-2]] = v
			} else {
				formValues[k] = v[0]
			}
		}
	}
	if len(formValues) == 0 {
		return nil
	}
	for tag, field := range structtag.GetFieldsByTag(out, "form") {
		if formVal, ok := formValues[tag]; ok {
			vv := v.Field(field.Index)
			vk := vv.Kind()
			var val interface{}
			if vk == reflect.String {
				if v, ok := formVal.(string); ok {
					val = v
				}
			} else {
				switch vk {
				case reflect.Int:
					v := formVal.(string)
					val, _ = strconv.Atoi(v)
				case reflect.Int64:
					v := formVal.(string)
					val, _ = strconv.ParseInt(v, 10, 64)
				case reflect.Float64:
					v := formVal.(string)
					val, _ = strconv.ParseFloat(v, 64)
				case reflect.Ptr:
					if vv.Type() == typeMultipartFileHeader {
						if fh, ok := formVal.(*multipart.FileHeader); ok {
							val = fh
						}
					}
				case reflect.Slice:
					if vv.Type() == typeMultipartFileHeaderSlice {
						val = formVal
					}
				}
			}
			if val != nil {
				vv.Set(reflect.ValueOf(val))
			}
		}
	}
	return nil
}

// GetValues returns a map of all values from context
func GetValues(r *http.Request) map[string]interface{} {
	vars, ok := r.Context().Value(keyVals).(map[string]interface{})
	if ok {
		return vars
	}
	return make(map[string]interface{})
}

// SetValue stores a key value pair in context
func SetValue(r *http.Request, key string, val interface{}) *http.Request {
	vals := GetValues(r)
	vals[key] = val
	ctx := r.Context()
	return r.WithContext(context.WithValue(ctx, keyVals, vals))
}

// GetValue returns the stored value from context
func GetValue(r *http.Request, key string) interface{} {
	val, ok := GetValues(r)[key]
	if ok {
		return val
	}
	return nil
}

func refTypes(types ...reflect.Type) []reflect.Type {
	return types
}

func refVals(vals ...interface{}) (values []reflect.Value) {
	for _, v := range vals {
		values = append(values, reflect.ValueOf(v))
	}
	return
}
