package restruct

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"log/slog"
)

type (
	// View handles rendering of templates and static files from a file system.
	View struct {
		// Source file system
		FS fs.FS
		// Funcs for templates
		Funcs template.FuncMap
		// Skips files matching this regex from being routed/served directly
		Skips *regexp.Regexp
		// Layouts is a list of glob patterns to include as helper templates (e.g. ["layout/*.html"])
		Layouts []string
		// Error template to use if view/file not found
		Error string
		// Data is a callback to get default data for templates
		Data func(r *http.Request) map[string]any

		// Writer to fallback for non-view responses or errors.
		// If nil, it will use the Handler's writer if available, or default to DefaultWriter.
		Writer *ResponseWriter

		cache   map[string]*viewCache
		routes  map[string]string
		cacheMu sync.RWMutex
	}

	viewCache struct {
		tmpl    *template.Template
		modTime time.Time
	}

	// Render is used to force the View to render a specific template path.
	// When returned from a handler method, the View will use the specified
	// Path instead of deriving it from the request URL.
	Render struct {
		// Path is the template path relative to the View's FS root (e.g. "some/path.html")
		Path string
		// Data is the data to pass to the template
		Data interface{}
	}
)

// ensure View implements ResponseWriter
var _ ResponseWriter = (*View)(nil)

// Write implements ResponseWriter.
// It checks if the request path matches a view file.
// If so, it renders the view. Otherwise, it delegates to DefaultWriter (if set) or handles it as static file if appropriate.
func (v *View) Write(w http.ResponseWriter, r *http.Request, types []reflect.Type, vals []reflect.Value) {
	// First check if we have an error
	for i, t := range types {
		if t == typeError && !vals[i].IsNil() {
			// v.delegate(w, r, types, vals)
			// For now, simple error
			if err, ok := vals[i].Interface().(error); ok {
				v.error(w, r, err, nil)
			}
			return
		}
	}

	// Get data from first return value if any
	var data interface{}
	if len(vals) > 0 {
		data = vals[0].Interface()
	}

	// If the return content is explicitly a Response struct, then we assume
	// the user wants the DefaultWriter to handle it (JSON/Raw).
	// This prevents the View from trying to look up a template for an API response
	// and falling back to the Error page.
	if _, ok := data.(*Response); ok {
		v.delegate(w, r, types, vals)
		return
	}

	// If the return content is explicitly a Render struct, then we use the
	// specified path directly instead of deriving from the URL.
	if render, ok := data.(*Render); ok {
		v.renderPath(w, r, render.Path, render.Data)
		return
	}

	urlPath := strings.TrimPrefix(r.URL.Path, "/")
	if urlPath == "" {
		urlPath = "index.html"
	}

	// Try to find the file
	// ... (candidates logic)

	candidates := []string{urlPath}
	if !strings.HasSuffix(urlPath, ".html") {
		candidates = append(candidates, urlPath+".html")
	}
	if !strings.HasSuffix(urlPath, ".tmpl") {
		candidates = append(candidates, urlPath+".tmpl")
	}
	// simple cleaner for directory index
	if strings.HasSuffix(urlPath, "/") {
		candidates = append(candidates, urlPath+"index.html")
	} else {
		candidates = append(candidates, urlPath+"/index.html")
	}

	var fileName string
	var modTime time.Time
	var isStatic bool

	// Check if FS supports Stat
	statFS, hasStat := v.FS.(fs.StatFS)

	var fallbackParams map[string]string

	// Load routes once for both checks below
	v.loadRoutes()

	// Check if we have a route pattern in context
	var routeMatch string
	if route, ok := r.Context().Value(keyRoute).(string); ok {
		v.cacheMu.RLock()
		if fName, ok := v.routes[strings.TrimPrefix(route, "/")]; ok {
			routeMatch = fName
		}
		v.cacheMu.RUnlock()
	}

	// Fallback: try to match route by pattern if not found by direct key
	if routeMatch == "" {
		v.cacheMu.RLock()
		for pattern, fName := range v.routes {
			// pattern is like "{profile}"
			rawParts := strings.Split(pattern, "/")
			var parts []string
			for _, p := range rawParts {
				if p != "" {
					parts = append(parts, p)
				}
			}
			pc := paramCache{pathParts: parts}
			params, ok := matchPath(pc, urlPath)
			if ok {
				routeMatch = fName
				fallbackParams = params
				break
			}
		}
		v.cacheMu.RUnlock()
	}

	if routeMatch != "" {
		candidates = []string{routeMatch}
		// If we found fallback params, we need to inject them into the context
		// so they are available to the view.
		if len(fallbackParams) > 0 {
			ctx := r.Context()
			existingParams, _ := ctx.Value(keyParams).(map[string]string)
			newParams := make(map[string]string)
			for k, v := range existingParams {
				newParams[k] = v
			}
			for k, v := range fallbackParams {
				newParams[k] = v
			}
			ctx = context.WithValue(ctx, keyParams, newParams)
			r = r.WithContext(ctx)
		}
	}

	for _, c := range candidates {
		// Clean path for FS
		c = strings.TrimPrefix(c, "/")
		if v.Skips != nil && v.Skips.MatchString(c) {
			continue
		}

		if hasStat {
			info, err := statFS.Stat(c)
			if err == nil && !info.IsDir() {
				fileName = c
				modTime = info.ModTime()
				break
			}
		} else {
			f, err := v.FS.Open(c)
			if err == nil {
				fileName = c
				f.Close()
				break
			}
		}
	}

	if fileName == "" {
		// Not found in view
		isAny, _ := r.Context().Value(keyIsAny).(bool)
		if v.Error != "" && isAny {
			// Try to serve error template
			fileName = v.Error
			// We check stat for error file too
			if hasStat {
				info, err := statFS.Stat(fileName)
				if err == nil {
					modTime = info.ModTime()
				} else {
					// Fallback to error delegate if error template not found?
					// Or just let Open fail later.
				}
			}
		} else {
			v.delegate(w, r, types, vals)
			return
		}
	}

	// Check if it's a template
	if !strings.HasSuffix(fileName, ".html") && !strings.HasSuffix(fileName, ".tmpl") {
		isStatic = true
	}

	// Serve static content
	if isStatic {
		file, err := v.FS.Open(fileName)
		if err != nil {
			v.error(w, r, err, nil)
			return
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			v.error(w, r, err, nil)
			return
		}
		http.ServeContent(w, r, fileName, modTime, bytes.NewReader(content))
		return
	}

	// Layouts freshness check
	if hasStat && len(v.Layouts) > 0 {
		layoutTime, err := v.getLayoutModTime()
		if err == nil && layoutTime.After(modTime) {
			modTime = layoutTime
		}
	}

	// Template handling
	tmpl, err := v.getTemplate(fileName, modTime)
	if err != nil {
		v.error(w, r, err, nil)
		return
	}

	v.render(w, r, tmpl, fileName, data)
}

// getTemplate returns a parsed template from cache or parses it from the file system.
func (v *View) getTemplate(fileName string, modTime time.Time) (*template.Template, error) {
	// 1. Check cache with Read Lock
	v.cacheMu.RLock()
	if v.cache != nil {
		if cached, ok := v.cache[fileName]; ok {
			// Check freshness if we have a valid modTime (non-zero)
			// If modTime is zero (no StatFS), we assume cache is always valid (or never stale based on time)
			if modTime.IsZero() || cached.modTime.Equal(modTime) {
				v.cacheMu.RUnlock()
				return cached.tmpl, nil
			}
		}
	}
	v.cacheMu.RUnlock()

	// 2. Cache miss or stale - Prepare to Parse
	// We do parsing OUTSIDE the lock to allow concurrency for other requests serving cached content.

	// Create a new template bucket
	tmpl := template.New("").Funcs(v.Funcs)

	// Helper to read and parse a file into the template set
	parseFile := func(name string) error {
		file, err := v.FS.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			return err
		}
		_, err = tmpl.New(name).Parse(string(content))
		return err
	}

	// 2a. Parse all layouts
	for _, pattern := range v.Layouts {
		matches, err := fs.Glob(v.FS, pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if err := parseFile(m); err != nil {
				return nil, fmt.Errorf("layout parse error %s: %w", m, err)
			}
		}
	}

	// 2b. Parse the requested file
	if err := parseFile(fileName); err != nil {
		slog.Error("Parse error", "file", fileName, "error", err)
		return nil, err
	}

	// 3. Update Cache with Write Lock
	v.cacheMu.Lock()
	defer v.cacheMu.Unlock()

	// Initialize cache map if needed
	if v.cache == nil {
		v.cache = make(map[string]*viewCache)
	}

	// Double check: maybe someone else updated it while we were parsing?
	// If so, and their version is as fresh or fresher, use theirs?
	// Actually, simpler to just overwrite. If multiple routines parse concurrently, last one wins.
	// Since we are parsing the same file content (presumably), the result is same.

	v.cache[fileName] = &viewCache{
		tmpl:    tmpl,
		modTime: modTime,
	}

	return tmpl, nil
}

// getLayoutModTime returns the latest modification time of all layout files.
func (v *View) getLayoutModTime() (time.Time, error) {
	var maxTime time.Time
	statFS, ok := v.FS.(fs.StatFS)
	if !ok {
		return maxTime, nil
	}

	for _, pattern := range v.Layouts {
		matches, err := fs.Glob(v.FS, pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			info, err := statFS.Stat(m)
			if err == nil {
				if t := info.ModTime(); t.After(maxTime) {
					maxTime = t
				}
			}
		}
	}
	return maxTime, nil
}

func (v *View) viewData(r *http.Request, data any) map[string]any {
	var viewData map[string]any

	if v.Data != nil {
		viewData = v.Data(r)
	}
	if viewData == nil {
		viewData = make(map[string]any)
	}

	viewData["Request"] = r
	if params, ok := r.Context().Value(keyParams).(map[string]string); ok {
		for k, v := range params {
			viewData[k] = v
		}
	}

	// If it's a map, we merge it directly
	if m, ok := data.(map[string]any); ok {
		// Copy map so we don't mutate input
		for k, v := range m {
			viewData[k] = v
		}
	} else if data != nil {
		viewData["Data"] = data
	}

	return viewData
}

func (v *View) render(w http.ResponseWriter, r *http.Request, tmpl *template.Template, name string, data interface{}) {

	v.execute(w, r, tmpl, name, data)
}

func (v *View) error(w http.ResponseWriter, r *http.Request, err error, data interface{}) {
	slog.Error("View Error", "error", err, "path", r.URL.Path)
	// Try to render the Error template if defined
	var errs []error
	if v.Error != "" {
		// We need to resolve modTime for error template too if possible
		var actErrModTime time.Time
		if statFS, ok := v.FS.(fs.StatFS); ok {
			if info, _ := statFS.Stat(v.Error); info != nil {
				actErrModTime = info.ModTime()
			}
			// Also check layouts
			if len(v.Layouts) > 0 {
				if lTime, lErr := v.getLayoutModTime(); lErr == nil && lTime.After(actErrModTime) {
					actErrModTime = lTime
				}
			}
		}

		errTmpl, parseErr := v.getTemplate(v.Error, actErrModTime)
		if parseErr == nil {
			if data == nil {
				data = v.viewData(r, nil)
			}
			if d, ok := data.(map[string]interface{}); ok {
				d["Error"] = err
			}
			if err2 := errTmpl.ExecuteTemplate(w, v.Error, data); err2 == nil {
				return
			} else {
				errs = append(errs, err2)
			}
		} else {
			errs = append(errs, parseErr)
		}
	}

	// Fallback to DefaultWriter (or custom Writer)
	errVal := reflect.ValueOf(err)
	errType := reflect.TypeOf((*error)(nil)).Elem()
	types := []reflect.Type{errType}
	vals := []reflect.Value{errVal}

	if len(errs) > 0 {
		slog.Error("Render Error", "errors", errs, "path", r.URL.Path)
	}

	if v.Writer != nil && *v.Writer != nil {
		(*v.Writer).Write(w, r, types, vals)
	} else {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (v *View) execute(w http.ResponseWriter, r *http.Request, tmpl *template.Template, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Execute the specific file template
	data = v.viewData(r, data)
	err := tmpl.ExecuteTemplate(w, name, data)
	if err != nil {
		v.error(w, r, err, data)
	}
}

// Serve is a placeholder method used to register routes for static files/views
// that don't have a backing struct method.
// It returns nil so that the ResponseWriter (View.Write) is triggered.
func (v *View) Serve() interface{} {
	return nil
}

// Routes scans the FS and returns a Map of path -> method name (which is "Serve")
// It returns a list of paths that should be registered.
func (v *View) Routes() []string {
	v.loadRoutes()
	var routes []string
	for r := range v.routes {
		if r == "" {
			routes = append(routes, "/")
		} else {
			routes = append(routes, "/"+r)
		}
	}
	return routes
}

func (v *View) loadRoutes() {
	v.cacheMu.Lock()
	defer v.cacheMu.Unlock()

	if v.routes != nil {
		return
	}

	// Only walk if ReadDirFS is supported
	if _, ok := v.FS.(fs.ReadDirFS); !ok {
		return
	}

	v.routes = make(map[string]string)

	fs.WalkDir(v.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if v.Skips != nil && v.Skips.MatchString(p) {
			return nil
		}
		// Convert path to route
		// public/index.html -> /
		// public/about.html -> /about
		// public/css/style.css -> /css/style.css

		// We assume FS root is the public root.

		route := "/" + p
		if strings.HasSuffix(p, "index.html") {
			route = "/" + strings.TrimSuffix(p, "index.html")
		} else if strings.HasSuffix(p, ".html") {
			route = "/" + strings.TrimSuffix(p, ".html")
		} else if strings.HasSuffix(p, ".tmpl") {
			route = "/" + strings.TrimSuffix(p, ".tmpl")
		}

		// Normalize
		if route != "/" {
			route = strings.TrimRight(route, "/")
		}

		v.routes[strings.TrimPrefix(route, "/")] = p
		return nil
	})
}

func (v *View) delegate(w http.ResponseWriter, r *http.Request, types []reflect.Type, vals []reflect.Value) {
	if v.Writer != nil && *v.Writer != nil {
		(*v.Writer).Write(w, r, types, vals)
	} else {
		// Fallback simple JSON
		(&DefaultWriter{}).Write(w, r, types, vals)
	}
}

// renderPath renders a specific template path with the given data.
// This is used when a handler returns a *Render struct.
func (v *View) renderPath(w http.ResponseWriter, r *http.Request, path string, data interface{}) {
	// Clean and normalize the path
	path = strings.TrimPrefix(path, "/")

	var modTime time.Time
	statFS, hasStat := v.FS.(fs.StatFS)
	data = v.viewData(r, data)
	// Check if the file exists and get mod time
	if hasStat {
		info, err := statFS.Stat(path)
		if err != nil {
			v.error(w, r, err, data)
			return
		}
		modTime = info.ModTime()
	} else {
		f, err := v.FS.Open(path)
		if err != nil {
			v.error(w, r, err, data)
			return
		}
		f.Close()
	}

	// Layouts freshness check
	if hasStat && len(v.Layouts) > 0 {
		layoutTime, err := v.getLayoutModTime()
		if err == nil && layoutTime.After(modTime) {
			modTime = layoutTime
		}
	}

	// Get or parse the template
	tmpl, err := v.getTemplate(path, modTime)
	if err != nil {
		v.error(w, r, err, data)
		return
	}

	v.render(w, r, tmpl, path, data)
}
