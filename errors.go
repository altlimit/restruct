package restruct

import "net/http"

type (
	Error struct {
		Status  int
		Message string
		Data    interface{}
		Err     error
	}
)

func (e Error) Error() string {
	var msg string
	if e.Message != "" {
		msg = " " + e.Message
	}
	if e.Status == 0 {
		return http.StatusText(http.StatusInternalServerError) + msg
	}
	return http.StatusText(e.Status) + msg
}
