package restruct

import (
	"encoding/json"
	"log"
	"net/http"
	"reflect"
)

type (
	// ResponseWriter is called on outputs of your methods.
	// slice of reflect.Type & Value is the types and returned values
	ResponseWriter interface {
		Write(http.ResponseWriter, *http.Request, []reflect.Type, []reflect.Value)
	}

	// DefaultWriter uses json.Encoder for output
	// and manages error handling. Adding Errors mapping can
	// help with your existing error to a proper Error{}
	DefaultWriter struct {
		Errors map[error]Error
	}

	// Response is used by DefaultWriter for custom response
	Response struct {
		Status      int
		Headers     map[string]string
		ContentType string
		Content     interface{}
	}
)

// Write implements the DefaultWriter ResponseWriter
// this handles if your return is (int, error) or any last error it would write the error if it's not nil
// otherwise return the other returns as output, if its more than 2 it will be slice of interface{} exclduing error
func (dw *DefaultWriter) Write(w http.ResponseWriter, r *http.Request, types []reflect.Type, vals []reflect.Value) {
	// no returns are not sent here so we just check if 1 or more
	lt := len(types)
	if lt == 1 {
		dw.WriteJSON(w, r, vals[0].Interface())
		return
	}
	var out interface{}
	defer func() {
		dw.WriteJSON(w, r, out)
	}()
	if types[lt-1] == typeError {
		errVal := vals[lt-1]
		if !errVal.IsNil() {
			out = errVal.Interface()
			return
		}
		vals = vals[:lt-1]
	}
	if len(vals) == 1 {
		dw.WriteJSON(w, r, vals[0].Interface())
		return
	}
	var args []interface{}
	for _, v := range vals {
		args = append(args, v.Interface())
	}
	out = args
}

// This writes application/json content type uses status codes 200
// on valid ones and 500 on uncaught, 400 on malformed json, etc.
func (dw *DefaultWriter) WriteJSON(w http.ResponseWriter, r *http.Request, out interface{}) {
	if w == nil {
		return
	}
	if out == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	cType := "application/json; charset=UTF-8"
	status := http.StatusOK
	var headers map[string]string
	if resp, ok := out.(Response); ok {
		if resp.Status != 0 {
			status = resp.Status
		}
		headers = resp.Headers
		if resp.ContentType != "" {
			cType = resp.ContentType
		}
		if resp.Content != nil {
			out = resp.Content
		} else {
			out = nil
		}
	}

	if err, ok := out.(error); ok {
		status = http.StatusInternalServerError
		var (
			msg     string
			errData interface{}
		)
		e, ok := err.(Error)
		if dw.Errors != nil && !ok {
			if ee, k := dw.Errors[err]; k {
				ok = true
				e = ee
			}
		}
		if ok {
			if e.Status != 0 {
				status = e.Status
			}
			if e.Message != "" {
				msg = e.Message
			}
			if e.Data != nil {
				errData = e.Data
			}
			if e.Err != nil {
				log.Println("Error:", e.Err)
			}
		} else {
			log.Println("Error:", err)
		}
		if msg == "" {
			msg = http.StatusText(status)
		}
		errResp := map[string]interface{}{
			"error": msg,
		}
		if errData != nil {
			errResp["data"] = errData
		}
		out = errResp
	}

	w.WriteHeader(status)
	h := w.Header()
	foundContentType := false
	for k, v := range headers {
		if k == "Content-Type" {
			foundContentType = true
		}
		h.Add(k, v)
	}
	if !foundContentType {
		h.Set("Content-Type", cType)
	}
	if b, ok := out.([]byte); ok {
		_, err := w.Write(b)
		if err != nil {
			log.Println("WriteError", err)
		}
	} else {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(out); err != nil {
			log.Println("WriteJsonError", err)
		}
	}
}
