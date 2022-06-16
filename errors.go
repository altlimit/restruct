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
	if e.Status == 0 {
		return http.StatusText(http.StatusInternalServerError)
	}
	return http.StatusText(e.Status)
}
