package restruct

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strings"
)

type (
	ctxKey string
)

var (
	ErrReaderReturnLen = errors.New("reader args len does not match")

	// Pre-allocated error responses to avoid allocations in hot paths
	errNotFoundTypes        = []reflect.Type{typeError}
	errNotFoundVals         []reflect.Value
	errMethodNotAllowedVals []reflect.Value
)

func init() {
	errNotFoundVals = []reflect.Value{reflect.ValueOf(Error{Status: http.StatusNotFound})}
	errMethodNotAllowedVals = []reflect.Value{reflect.ValueOf(Error{Status: http.StatusMethodNotAllowed})}
}

const (
	keyParams ctxKey = "params"
	keyVals   ctxKey = "vals"
	keyIsAny  ctxKey = "isAny"
	keyRoute  ctxKey = "route"
)

type (
	Middleware func(http.Handler) http.Handler

	Viewer interface {
		View() *View
	}

	Handler struct {
		// Writer controls the output of your service, defaults to DefaultWriter
		Writer ResponseWriter
		// Reader controls the input of your service, defaults to DefaultReader
		Reader RequestReader

		prefix            string
		prefixLen         int
		services          map[string]interface{}
		cache             *methodCache
		middlewares       []Middleware
		writerInitialized bool
		readerInitialized bool
	}

	methodCache struct {
		root        *node
		paramRoutes []paramCache
		byPath      map[string][]*method
	}

	node struct {
		children      map[string]*node
		paramChild    *node
		paramName     string
		wildcardChild *node
		wildcardName  string
		methods       []*method
	}

	paramCache struct {
		path      string
		pathParts []string
		methods   []*method
		isAny     bool
	}

	wrappedHandler struct {
		handler http.Handler
	}
)

func (n *node) insert(parts []string, m *method) {
	if len(parts) == 0 {
		n.methods = append(n.methods, m)
		return
	}

	part := parts[0]
	if len(part) > 3 && part[0] == '{' && part[len(part)-1] == '}' && part[len(part)-2] == '*' {
		// Wildcard node
		if n.wildcardChild == nil {
			n.wildcardChild = &node{
				children: make(map[string]*node),
			}
		}
		n.wildcardChild.wildcardName = part[1 : len(part)-2]
		// Wildcards consume the rest, so we attach method here
		n.wildcardChild.methods = append(n.wildcardChild.methods, m)
	} else if len(part) > 2 && part[0] == '{' && part[len(part)-1] == '}' {
		// Parameter node
		if n.paramChild == nil {
			n.paramChild = &node{
				children: make(map[string]*node),
			}
		}
		n.paramChild.paramName = part[1 : len(part)-1]
		n.paramChild.insert(parts[1:], m)
	} else {
		// Static node
		if n.children == nil {
			n.children = make(map[string]*node)
		}
		child, ok := n.children[part]
		if !ok {
			child = &node{
				children: make(map[string]*node),
			}
			n.children[part] = child
		}
		child.insert(parts[1:], m)
	}
}

func (n *node) search(path string, params map[string]string) []*method {
	// 1. Check if we match the current node and path is done
	// OR if we have a wildcard child that can consume the rest
	if path == "" {
		return n.methods
	}

	// 2. Check wildcard immediately if we can't consume more
	// Actually, we should check children first, then param, then wildcard fallback
	// But wildcard can also match "nothing" if we allow it? usually wildcards match at least something.
	// In this design, wildcard matching happens when we process a part.

	// Keep track of where we could have gone for wildcard fallback
	// Only the most specific wildcard matters? Or hierarchical?
	// The search is recursive-like but iterative here.
	// Implementing backtracking or recursive search is cleaner for priority:
	// Static > Param > Wildcard

	return n.searchRecursive(path, params)
}

func (n *node) searchRecursive(path string, params map[string]string) []*method {
	idx := strings.Index(path, "/")
	var part string
	var remainder string
	var isTerminal bool

	if idx == -1 {
		part = path
		isTerminal = true
	} else {
		part = path[:idx]
		remainder = path[idx+1:]
		isTerminal = false
	}

	// 1. Static Match
	if n.children != nil {
		if child, ok := n.children[part]; ok {
			if isTerminal {
				return child.methods
			}
			if res := child.searchRecursive(remainder, params); res != nil {
				return res
			}
		}
	}

	// 2. Param Match
	if n.paramChild != nil {
		if isTerminal {
			if n.paramChild.paramName != "" {
				params[n.paramChild.paramName] = part
			}
			return n.paramChild.methods
		}
		// Recurse
		if res := n.paramChild.searchRecursive(remainder, params); res != nil {
			if n.paramChild.paramName != "" {
				params[n.paramChild.paramName] = part
			}
			return res
		}
	}

	// 3. Wildcard Match
	if n.wildcardChild != nil {
		if n.wildcardChild.wildcardName != "" {
			params[n.wildcardChild.wildcardName] = path
		}
		return n.wildcardChild.methods
	}

	return nil
}

func (mc *methodCache) methods() (methods []*method) {
	// BFS or DFS to get all methods from Trie if needed, but for now just iterating what we have
	// This function was used for Routes(), which needs to list all routes.
	// We need to traverse the tree.
	var traverse func(*node)
	traverse = func(n *node) {
		if n == nil {
			return
		}
		methods = append(methods, n.methods...)
		for _, child := range n.children {
			traverse(child)
		}
		traverse(n.paramChild)
		traverse(n.wildcardChild)
	}
	traverse(mc.root)

	for _, param := range mc.paramRoutes {
		methods = append(methods, param.methods...)
	}
	for _, m := range mc.byPath {
		methods = append(methods, m...)
	}
	return
}

func (wh *wrappedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wh.handler.ServeHTTP(w, r)
}

// SetView sets the view for the handler and updates the cache

// NewHandler creates a handler for a given struct.
func NewHandler(svc interface{}) *Handler {
	h := &Handler{
		services: map[string]interface{}{"": svc},
	}
	h.mustCompile("")
	if init, ok := svc.(Init); ok {
		init.Init(h)
	}
	return h
}

// WithPrefix prefixes your service with given path. You can't use parameters here.
// This is useful if you want to register this handler with another third party router.
func (h *Handler) WithPrefix(prefix string) *Handler {
	h.mustCompile(prefix)
	return h
}

// Routes returns a list of routes registered and it's definition
func (h *Handler) Routes() (routes []string) {
	h.updateCache()
	for _, m := range h.cache.methods() {
		var methods []string
		for k := range m.methods {
			methods = append(methods, k)
		}
		if len(methods) == 0 {
			methods = append(methods, "*")
		}
		r := h.prefix + m.path + " [" + strings.Join(methods, ",") + "] -> " + m.location
		var params []string
		for _, v := range m.params {
			params = append(params, v.String())
		}
		var returns []string
		for _, v := range m.returns {
			returns = append(returns, v.String())
		}
		r += "(" + strings.Join(params, ", ") + ")"
		if len(returns) > 0 {
			r += " (" + strings.Join(returns, ", ") + ")"
		}
		routes = append(routes, r)
	}
	sort.Strings(routes)
	return
}

// AddService adds a new service to specified route.
// You can put {param} in this route.
func (h *Handler) AddService(path string, svc interface{}) {
	path = strings.TrimPrefix(path, "/")
	if path != "" && !strings.HasSuffix(path, "/") {
		path += "/"
	}
	if _, ok := h.services[path]; ok {
		panic("service " + path + " already exists")
	}
	h.services[path] = svc
	h.cache = nil
}

// Use adds a middleware to your services.
func (h *Handler) Use(fns ...Middleware) {
	h.middlewares = append(h.middlewares, fns...)
}

// ServeHTTP calls the method with the matched route.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[h.prefixLen:]
	// Lazy initialization with flags to avoid nil checks on every request
	if !h.writerInitialized {
		if h.Writer == nil {
			h.Writer = &DefaultWriter{}
		}
		h.writerInitialized = true
	}
	if !h.readerInitialized {
		if h.Reader == nil {
			h.Reader = &DefaultReader{Bind: Bind}
		}
		h.readerInitialized = true
	}
	// if there are middleware we wrap it in reverse so it's called
	// in the order they were added
	chain := func(m *method) *wrappedHandler {
		handler := &wrappedHandler{handler: h.createHandler(m)}
		middlewares := append(h.middlewares, m.middlewares...)
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = &wrappedHandler{handler: middlewares[i](handler)}
		}
		return handler
	}
	runMethod := func(m *method) {
		chain(m).ServeHTTP(w, r)
	}
	// we check path look up first then see if proper method
	if vals, ok := h.cache.byPath[path]; ok {
		for _, m := range vals {
			ok := m.methods == nil
			if !ok {
				_, ok = m.methods[r.Method]
			}
			if ok {
				runMethod(m)
				return
			}
		}
		h.Writer.Write(w, r, errNotFoundTypes, errMethodNotAllowedVals)
		return
	}
	// we do heavier look up such as path parts or regex then if any match
	// we set path found but still need to match method for proper error return
	status := http.StatusNotFound

	// Try Trie search (now handles static, param, and wildcard routes)
	if h.cache.root != nil {
		params := make(map[string]string)
		methods := h.cache.root.search(path, params)
		if methods != nil {

			for _, v := range methods {
				ok := v.methods == nil
				if !ok {
					_, ok = v.methods[r.Method]
				}
				if ok {
					// Apply params
					if len(params) > 0 {
						ctx := r.Context()
						ctx = context.WithValue(ctx, keyParams, params)
						// If wildcard param exists, flag it?
						// We used to set keyIsAny=true.
						// We can check if any param ends in "*" or is "any"?
						// The tests might check for "any" param specifically.

						// Legacy support: if we have "any" param, treat as catch-all
						if _, hasAny := params["any"]; hasAny {
							ctx = context.WithValue(ctx, keyIsAny, true)
						}

						ctx = context.WithValue(ctx, keyRoute, v.path)
						r = r.WithContext(ctx)
					} else {
						ctx := r.Context()
						ctx = context.WithValue(ctx, keyRoute, v.path)
						r = r.WithContext(ctx)
					}
					runMethod(v)
					return
				}
			}
			status = http.StatusMethodNotAllowed
		}
	}

	// Use pre-allocated error values for common cases
	if status == http.StatusNotFound {
		h.Writer.Write(w, r, errNotFoundTypes, errNotFoundVals)
	} else {
		h.Writer.Write(w, r, errNotFoundTypes, errMethodNotAllowedVals)
	}
}

// wrapped handler that calls the actual method and processes the returns
// the parameter allowed here are *http.Request and http.ResponseWriter
// the returns can be anything or an error which will be sent to the ResponseWriter
// a multiple return is passed as slice of interface{}
func (h *Handler) createHandler(m *method) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		args := make([]reflect.Value, len(m.params))
		for k, v := range m.params {
			switch v {
			case typeHttpRequest:
				args[k] = reflect.ValueOf(r)
			case typeHttpWriter:
				args[k] = reflect.ValueOf(w)
			case typeContext:
				args[k] = reflect.ValueOf(r.Context())
			}
		}
		// has unknown types in parameters, use RequestReader (pre-computed at init)
		if len(m.readerIndexes) > 0 {
			typeArgs, err := h.Reader.Read(r, m.readerTypes)
			if err != nil {
				h.Writer.Write(w, r, refTypes(typeError), refVals(err))
				return
			}
			if len(typeArgs) != len(m.readerIndexes) {
				h.Writer.Write(w, r, refTypes(typeError), refVals(Error{Err: ErrReaderReturnLen}))
				return
			}
			for k, i := range m.readerIndexes {
				args[i] = typeArgs[k]
			}
		}
		out := m.source.Call(args)
		ot := len(out)
		if ot == 0 {
			return
		}
		writer := h.Writer
		if m.writer != nil {
			writer = m.writer
		}
		writer.Write(w, r, m.returns, out)
	})
}

// Called every time you add a handler to create a cached info about
// your routes and which methods it points to. This will also look up
// exported structs to add as a service. You can avoid this by adding
// route:"-" or to specify specific route add route:"path/{hello}"
func (h *Handler) updateCache() {
	if h.cache != nil {
		return
	}
	if h.prefix == "" {
		h.mustCompile("")
	}
	h.cache = &methodCache{
		byPath: make(map[string][]*method),
	}
	// cache all same paths so we only compare it once
	pathCache := make(map[string][]*method)
	// we store ordered paths so it's still looked up in order you enter it
	var orderedPaths []string
	for k, svc := range h.services {
		var svcView *View
		if v, ok := svc.(Viewer); ok {
			svcView = v.View()
			// Inject handler's writer to view
			svcView.Writer = &h.Writer
		}

		for _, v := range serviceToMethods(k, svc) {

			if v.Name == "Any" || strings.HasSuffix(v.Name, "_Any") {
				// Any is a catch-all. Capture path minus "any"
				// For "Any", path is empty (relative to service) or "{any*}".
				// For "Backup_Any", path is "backup/any".
				basePath := v.path
				if v.Name == "Any" {
					basePath = strings.TrimSuffix(basePath, "any")
					if basePath == "{any*}" {
						basePath = ""
					}
				} else {
					// v.path is lowercase. "Backup_Any" -> "backup/{any*}"
					basePath = strings.TrimSuffix(basePath, "/{any*}")
					basePath = strings.TrimSuffix(basePath, "{any*}")
				}

				// If the path contains parameters (wildcards), we should NOT use "isAny" prefix logic.
				// We should let matchPath handle it (standard parameter extraction).
				// Exception: "Root Any" which we canonicalized to "" should use isAny=true.
				// Update: We now support wildcards in Trie! So logic simplifies.
				// If it's a true wildcard suffix (e.g. {any*}), we put it in Trie.
				// If it's a global catch-all (isAny=true, path=""), we might still need paramRoutes
				// or insert it as a root wildcard? root wildcard IS a thing now.

				// Standardize path ending with `{any*}` if implied
				if v.Name == "Any" && basePath == "" {
					// Root Any -> treat as {any*} at root?
					// If we put it in Trie, it's a root wildcard.
					v.pathParts = []string{"{any*}"}
				}

				// If it's NOT a global root catch-all, we can put it in the Trie.
				// If it IS a global root catch-all, `insert` at root level wildcard works too!
				// So we can remove special paramRoutes handling for these entirely?
				// There is "isAny" logic used in ServeHTTP.
				// Let's rely on standard Trie.

				// Re-construct the full path for keying
				if v.pathParts == nil {
					// Should have pathParts if it came from serviceToMethods
				}

				// We just fall through to normal processing!
				// BUT we need to ensure the path corresponds to what we want: "app/{any*}"
				// serviceToMethods likely already set v.path to "app/{any*}"
				// So we don't need to do anything special here unless we want to override something.

				// Actually, earlier code was manually adding to paramRoutes to AVOID Trie.
				// We want to DELETE all that special logic and let it fall through.
			}

			if v.Name == "Any" || strings.HasSuffix(v.Name, "_Any") {
				// Just ensure path parts are correct for wildcard
				// nameToPath already converts "Any" -> "{any*}" or "App_Any" -> "app/{any*}"
				// So v.pathParts should be correct.
			}

			if v.pathParts != nil {
				_, ok := pathCache[v.path]
				if !ok {
					orderedPaths = append(orderedPaths, v.path)
				}
				pathCache[v.path] = append(pathCache[v.path], v)
			} else {
				h.cache.byPath[v.path] = append(h.cache.byPath[v.path], v)
			}
		}

		if svcView != nil {
			viewRoutes := svcView.Routes()
			// Create base method for view
			// We need to construct a dummy method that invokes View.Serve (or similar)
			// But since we removed global h.View, we need to locate "Serve" on the View instance?
			// svcView is *rs.View. It has Serve() method.
			viewMethod := &method{
				source: reflect.ValueOf(svcView).MethodByName("Serve"),
				writer: svcView,
			}
			viewMethod.mustParse()

			for _, r := range viewRoutes {
				if r == "/" {
					r = ""
				} else {
					r = strings.TrimPrefix(r, "/")
				}

				// The routes from View are generally root-relative or service-relative?
				// User wants "applies to the routes of the service".
				// IF service is mounted at root, View routes are root.
				// If service mounted at /api, View routes (e.g. index.html) -> /api/index.html?
				// View.Routes() returns relative paths from FS root.
				// Preserving prefix logic:
				// If prefix is "/", fullPath = r.
				// If prefix is "/api", fullPath = "api/" + r.
				// `serviceToMethods` uses `k` (prefix)?
				// But we are outside `serviceToMethods`.
				// `k` is the key? No, `k` is key in map?
				// Iterate `h.services` map[string]interface{}. Key is prefix?
				// Yes, `serviceToMethods(k, svc)`. `k` is prefix.

				fullPath := r
				if k != "" && k != "/" {
					fullPath = strings.TrimRight(k, "/") + "/" + r
				}

				// Check if already exists
				_, existingInCache := pathCache[fullPath]
				_, existingInByPath := h.cache.byPath[fullPath]
				if !existingInCache && !existingInByPath {
					orderedPaths = append(orderedPaths, fullPath)

					// Clone method with specific path
					m := &method{
						source:  viewMethod.source,
						path:    fullPath,
						params:  viewMethod.params,
						returns: viewMethod.returns,
						writer:  svcView,
					}
					m.mustParse()
					// Check if path has params (e.g. /posts/{id}.html - unlikely from FS)
					// FS paths usually static. Unless we treat some files as templates param?
					// Currently assuming static.
					pathCache[fullPath] = []*method{m}
				}
			}
		}
	}
	for _, path := range orderedPaths {
		// all of methods here have the same path so we use first one
		m := pathCache[path][0]

		if m.pathParts != nil {
			// Add to Trie
			if h.cache.root == nil {
				h.cache.root = &node{
					children: make(map[string]*node),
				}
			}
			for _, method := range pathCache[path] {
				h.cache.root.insert(method.pathParts, method)
			}
		} else {
			h.cache.byPath[path] = pathCache[path]
		}
	}

	// Sort paramRoutes once at cache build time for optimal lookup order
	sort.Slice(h.cache.paramRoutes, func(i, j int) bool {
		p1 := h.cache.paramRoutes[i]
		p2 := h.cache.paramRoutes[j]

		// Prefer longer base path (more specific)
		l1 := len(p1.path)
		l2 := len(p2.path)
		if l1 != l2 {
			return l1 > l2 // Longest first
		}

		// If lengths equal, prefer NON-Any (specific wildcards)
		if p1.isAny != p2.isAny {
			return !p1.isAny
		}

		return p1.path > p2.path // determinism
	})
}

func (h *Handler) mustCompile(prefix string) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	h.prefix = prefix
	h.prefixLen = len(h.prefix)
	h.updateCache()
}

// extractParamsFromPath extracts route params from a URL path using the given pathParts.
// This is used to re-extract params with correct names after Trie lookup.
func extractParamsFromPath(urlPath string, pathParts []string) map[string]string {
	params := make(map[string]string)
	for _, part := range pathParts {
		if urlPath == "" {
			break
		}
		idx := strings.Index(urlPath, "/")
		var segment string
		if idx == -1 {
			segment = urlPath
			urlPath = ""
		} else {
			segment = urlPath[:idx]
			urlPath = urlPath[idx+1:]
		}

		if len(part) > 2 && part[0] == '{' && part[len(part)-1] == '}' {
			name := part[1 : len(part)-1]
			// Handle wildcards
			if len(name) > 0 && name[len(name)-1] == '*' {
				name = name[:len(name)-1]
				params[name] = segment
				if urlPath != "" {
					params[name] += "/" + urlPath
				}
				break
			}
			params[name] = segment
		}
	}
	return params
}

// Checks path against request path if it's valid, this accepts a stripped path and not a full path
func matchPath(pc paramCache, path string) (params map[string]string, ok bool) {
	// Pre-allocate with capacity based on pathParts (each param segment needs one entry)
	paramCount := 0
	for _, p := range pc.pathParts {
		if len(p) > 2 && p[0] == '{' && p[len(p)-1] == '}' {
			paramCount++
		}
	}
	params = make(map[string]string, paramCount)
	// match by parts
	idx := -1
	pt := len(pc.pathParts)

	for {
		idx++
		if idx+1 > pt {
			return
		}

		mPart := pc.pathParts[idx]

		if mPart[0] == '{' {
			name := mPart[1 : len(mPart)-1]
			isWild := len(name) > 0 && name[len(name)-1] == '*'
			if isWild {
				name = name[:len(name)-1]
				params[name] = path
				ok = true
				return
			}
			if mPart == "{any}" {
				params[name] = path
				ok = true
				return
			}

			i := strings.Index(path, "/")
			var part string
			if i == -1 {
				part = path
				path = ""
			} else {
				part = path[:i]
				path = path[i+1:]
			}
			if part == "" {
				return
			}

			params[name] = part
		} else {
			i := strings.Index(path, "/")
			var part string
			if i == -1 {
				part = path
				path = ""
			} else {
				part = path[:i]
				path = path[i+1:]
			}

			if mPart != part {
				// Failed match
				return
			}
		}

		// If we processed the last part of definition
		if idx+1 == pt {
			if path == "" {
				ok = true
				return
			}
			// if path is not empty but we matched everything, then it fails?
			// unless the last part was a wildcard, but that returns early.
			// so if we are here, we must have exactly consumed everything.
			return
		}
	}
}
