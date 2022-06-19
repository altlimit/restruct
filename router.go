package restruct

type (
	// Router can be used to override method name to specific path,
	// implement Router interface in your service and return the new mapping:
	// {"ProductEdit": Route{Path: "product/{pid}"}}
	Router interface {
		Routes() map[string]Route
	}

	// Route for doing overrides with router interface and method restrictions.
	Route struct {
		// optional path, will use default behaviour if not present
		Path string
		// optional methods, will allow all if not present
		Methods []string
	}
)
