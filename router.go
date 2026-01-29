package restruct

type (
	// Router can be used to override method name to specific path,
	// implement Router interface in your service and return a slice of Route:
	// [Route{Handler:"ProductEdit", Path: "product/{pid}"}]
	Router interface {
		Routes() []Route
	}

	// Middlewares interface for common middleware for a struct
	Middlewares interface {
		Middlewares() []Middleware
	}

	// Init interface to access and override handler configs
	Init interface {
		Init(*Handler)
	}

	// Route for doing overrides with router interface and method restrictions.
	Route struct {
		// Handler is the method name you want to use for this route
		Handler string
		// optional path, will use default behaviour if not present
		Path string
		// optional methods, will allow all if not present
		Methods []string
		// optional middlewares, run specific middleware for this route
		Middlewares []Middleware
	}
)
