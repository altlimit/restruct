package restruct

import (
	"encoding/json"
	"log/slog"
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
		// Optional ErrorHandler, called whenever unhandled errors occurs, defaults to logging errors
		ErrorHandler   func(error)
		Errors         map[error]Error
		EscapeJsonHtml bool
	}

	// Response is used by DefaultWriter for custom response
	Response struct {
		Status      int
		Headers     map[string]string
		ContentType string
		Content     []byte
	}

	// Json response to specify a status code for default writer
	Json struct {
		Status  int
		Content interface{}
	}
)

var _ ResponseWriter = (*DefaultWriter)(nil)

// Write implements the DefaultWriter ResponseWriter
// returning (int, any, error) will write status int, any response if error is nil
// returning (any, error) will write any response if error is nil with status 200 or 400, 500 depdening on your error
// returning (int, any, any, error) will write status int slice of [any, any] response if error is nil
func (dw *DefaultWriter) Write(w http.ResponseWriter, r *http.Request, types []reflect.Type, vals []reflect.Value) {
	// no returns are not sent here so we just check if 1 or more
	lt := len(types)
	if lt == 1 {
		val := vals[0].Interface()
		if resp, ok := val.(*Response); ok {
			dw.WriteResponse(w, resp)
		} else {
			dw.WriteJSON(w, val)
		}
		return
	}
	var (
		out interface{}
		j   *Json
	)
	defer func() {
		if j != nil {
			j.Content = out
			out = j
		}
		dw.WriteJSON(w, out)
	}()
	// return with last type error
	if types[lt-1] == typeError {
		errVal := vals[lt-1]
		if !errVal.IsNil() {
			out = errVal.Interface()
			return
		}
		vals = vals[:lt-1]
	}
	// returning (int, something) means status code, response
	if len(vals) > 1 && types[0] == typeInt {
		j = &Json{Status: int(vals[0].Int())}
		vals = vals[1:]
	}
	if len(vals) == 1 {
		out = vals[0].Interface()
		return
	}
	var args []interface{}
	for _, v := range vals {
		args = append(args, v.Interface())
	}
	out = args
}

func (dw *DefaultWriter) log(err error) {
	if dw.ErrorHandler == nil {
		dw.ErrorHandler = func(err error) {
			slog.Error("InternalError", "error", err)
		}
	}
	dw.ErrorHandler(err)
}

// This writes application/json content type uses status code 200
// on valid ones and 500 on uncaught, 400 on malformed json, etc.
// use Json{Status, Content} to specify a code
func (dw *DefaultWriter) WriteJSON(w http.ResponseWriter, out interface{}) {
	if w == nil {
		return
	}
	status := http.StatusOK
	if j, ok := out.(*Json); ok {
		if j.Status > 0 {
			status = j.Status
		}
		out = j.Content
	}
	if out == nil {
		w.WriteHeader(status)
		return
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
				dw.log(e.Err)
			}
		} else {
			dw.log(err)
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

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(dw.EscapeJsonHtml)
	if err := enc.Encode(out); err != nil {
		dw.log(err)
	}
}

func (dw *DefaultWriter) WriteResponse(w http.ResponseWriter, resp *Response) {
	// Headers must be set BEFORE WriteHeader per HTTP spec
	if resp.ContentType != "" && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", resp.ContentType)
	}
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if resp.Status > 0 {
		w.WriteHeader(resp.Status)
	}
	if len(resp.Content) > 0 {
		if _, err := w.Write(resp.Content); err != nil {
			dw.log(err)
		}
	}
}
