package restruct

import (
	"fmt"
	"net/http"
)

type (
	Error struct {
		Status  int
		Message string
		Data    interface{}
		Err     error
	}
)

func (e Error) Error() string {
	msg := e.Message
	status := e.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	return fmt.Sprint(status, " ", msg)
}
