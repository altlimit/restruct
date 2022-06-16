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
	}

	Response struct {
		Status      int
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
	if resp, ok := out.(Response); ok {
		status = resp.Status
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
		var msg string
		if e, ok := err.(Error); ok {
			status = e.Status
			if e.Message != "" {
				msg = e.Message
			}
			if e.Err != nil {
				log.Println("APIError:", e.Err)
			}
		} else {
			log.Println("APIError:", err)
		}
		if msg == "" {
			msg = http.StatusText(status)
		}
		errResp := map[string]interface{}{
			"error": msg,
		}
		if e, ok := err.(ErrorData); ok {
			errResp["data"] = e.Data()
		}
		out = errResp
	}

	w.WriteHeader(status)
	w.Header().Set("Content-Type", cType)
	if b, ok := out.([]byte); ok {
		_, err := w.Write(b)
		if err != nil {
			log.Println("WriteError", err)
		}
	} else if err := json.NewEncoder(w).Encode(out); err != nil {
		log.Println("WriteJsonError", err)
	}
}
