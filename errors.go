package restruct

import "net/http"

type (
	// ErrorData for errors such as validation with custom data
	ErrorData interface {
		Data() interface{}
	}

	Error struct {
		Status  int
		Message string
		Err     error
	}
)

func (e Error) Error() string {
	if e.Status == 0 {
		return http.StatusText(http.StatusInternalServerError)
	}
	return http.StatusText(e.Status)
}
