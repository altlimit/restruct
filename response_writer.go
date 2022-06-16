package restruct

import (
	"encoding/json"
	"log"
	"net/http"
)

type (
	ResponseWriter interface {
		Write(http.ResponseWriter, interface{})
	}

	DefaultWriter struct {
		Errors map[error]Error
	}

	Response struct {
		Status      int
		Headers     map[string]string
		ContentType string
		Content     interface{}
	}
)

func (dw *DefaultWriter) Write(w http.ResponseWriter, out interface{}) {
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
		_, ok := err.(Error)
		if dw.Errors != nil && !ok {
			if ee, ok := dw.Errors[err]; ok {
				err = ee
			}
		}
		if e, ok := err.(Error); ok {
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
