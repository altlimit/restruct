package restruct

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery middleware handles panics and returns 500 error
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered", "error", err, "stack", string(debug.Stack()))
				er := Error{
					Status:  http.StatusInternalServerError,
					Message: fmt.Sprintf("Internal Server Error: %v", err),
				}
				if r.Header.Get("Content-Type") == "application/json" {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, er.Message)))
				} else {
					http.Error(w, er.Error(), http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}
