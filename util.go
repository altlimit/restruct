package restruct

import (
	"context"
	"net/http"
)

func Param(ctx context.Context, name string) string {
	if params, ok := ctx.Value(keyParams).(map[string]string); ok {
		return params[name]
	}
	return ""
}

func Query(r *http.Request, name string) string {
	if v, ok := r.URL.Query()[name]; ok && len(v) > 0 {
		return v[0]
	}
	return ""
}
