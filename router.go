package search

import (
	"net/http"
	"regexp"
	"strings"

	gddo "github.com/golang/gddo/httputil"
	"gitlab.com/tozd/go/errors"
)

type pathSegment struct {
	Value     string
	Parameter bool
}

func parsePath(path string) ([]pathSegment, errors.E) {
	if !strings.HasPrefix(path, "/") {
		return nil, errors.Errorf(`path does not start with "/": %s`, path)
	}
	p := strings.TrimPrefix(path, "/")
	segments := []pathSegment{}
	// If path is "/".
	if p == "" {
		return segments, nil
	}
	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == "" {
			return nil, errors.Errorf(`path has an empty part`)
		}
		var segment pathSegment
		if strings.HasPrefix(part, ":") {
			segment.Value = strings.TrimPrefix(part, ":")
			segment.Parameter = true
		} else {
			segment.Value = part
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

// TODO: Support custom regex in params.
//       See: https://router.vuejs.org/guide/essentials/route-matching-syntax.html#custom-regex-in-params

func compileRegexp(segments []pathSegment) (*regexp.Regexp, func([]string) Params, errors.E) {
	matchMap := make(map[int]string)
	expr := strings.Builder{}
	expr.WriteString("^")
	i := 0
	for _, segment := range segments {
		expr.WriteString("/")
		if segment.Parameter {
			expr.WriteString("([^/]+)")
			i++
			matchMap[i] = segment.Value
		} else {
			expr.WriteString(regexp.QuoteMeta(segment.Value))
		}
	}
	if len(segments) == 0 {
		expr.WriteString("/")
	}
	expr.WriteString("$")
	re, err := regexp.Compile(expr.String())
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	return re, func(match []string) Params {
		p := make(map[string]string, len(match)-1)
		for i, v := range match {
			if i == 0 {
				continue
			}
			p[matchMap[i]] = v
		}
		return p
	}, nil
}

type matcher struct {
	Regexp    *regexp.Regexp
	GetParams func([]string) Params
	Route     *route
}

type route struct {
	Name     string
	Path     string
	Segments []pathSegment
	// A map between methods and a map between content types and handlers.
	Handlers map[string]map[string]Handler
}

type Params map[string]string

type Handler func(http.ResponseWriter, *http.Request, Params)

// TODO: Implement RedirectTrailingSlash = true
// TODO: Implement RedirectFixedPath = true.

type Router struct {
	NotFound         Handler
	MethodNotAllowed Handler
	NotAcceptable    Handler
	Panic            func(w http.ResponseWriter, req *http.Request, err interface{})
	// A map between route name and routes.
	routes   map[string]*route
	matchers []matcher
}

func NewRouter() *Router {
	return &Router{
		routes: make(map[string]*route),
	}
}

func (r *Router) Handle(name, method, contentType, path string, handler Handler) errors.E {
	ro, ok := r.routes[name]
	if !ok {
		segments, err := parsePath(path)
		if err != nil {
			return errors.WithMessagef(err, `parsing path "%s" failed for route "%s"`, path, name)
		}
		re, get, err := compileRegexp(segments)
		if err != nil {
			return errors.WithMessagef(err, `compiling regexp for path "%s" failed for route "%s"`, path, name)
		}
		ro = &route{
			Name:     name,
			Path:     path,
			Segments: segments,
			Handlers: make(map[string]map[string]Handler),
		}
		r.routes[name] = ro
		r.matchers = append(r.matchers, matcher{
			Regexp:    re,
			GetParams: get,
			Route:     ro,
		})
	}

	if ro.Path != path {
		return errors.Errorf(`route with name "%s" but different paths "%s" vs. "%s"`, name, ro.Path, path)
	}

	for _, rr := range r.routes {
		if rr.Name == name {
			continue
		}

		if rr.Path == path {
			return errors.Errorf(`route with path "%s" but different names "%s" vs. "%s"`, path, rr.Name, name)
		}
	}

	m, ok := ro.Handlers[method]
	if !ok {
		m = make(map[string]Handler)
		ro.Handlers[method] = m
	}

	_, ok = m[contentType]
	if ok {
		return errors.Errorf(`route "%s" for "%s" has already handler for method "%s" and content type "%s"`, name, path, method, contentType)
	}

	m[contentType] = handler

	return nil
}

func (r *Router) recv(w http.ResponseWriter, req *http.Request) {
	if rcv := recover(); rcv != nil {
		r.Panic(w, req, rcv)
	}
}

func (r *Router) Error(w http.ResponseWriter, req *http.Request, code int) {
	http.Error(w,
		http.StatusText(code),
		code,
	)
}

// TODO: Compile all regexes into one large regex.

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if r.Panic != nil {
		defer r.recv(w, req)
	}

	path := req.URL.Path

	for _, matcher := range r.matchers {
		match := matcher.Regexp.FindStringSubmatch(path)
		if match == nil {
			continue
		}

		params := matcher.GetParams(match)

		m, ok := matcher.Route.Handlers[req.Method]
		if !ok {
			allow := []string{}
			for method := range matcher.Route.Handlers {
				allow = append(allow, method)
			}
			w.Header().Add("Allow", strings.Join(allow, ", "))

			if r.MethodNotAllowed != nil {
				r.MethodNotAllowed(w, req, params)
			} else {
				r.Error(w, req, http.StatusMethodNotAllowed)
			}
			return
		}

		// If we got to here, then the rest depends on the content type negotiation, so we set caching header
		// to signal that. Even if there is just one handler defined currently and no content negotiation really
		// happened, additional handlers might be defined in the future.
		w.Header().Add("Vary", "Accept")

		offers := []string{}
		for contentType := range m {
			if contentType != "" {
				offers = append(offers, contentType)
			}
		}

		contentType := gddo.NegotiateContentType(req, offers, "")
		handler, ok := m[contentType]
		if !ok {
			// offers is always non-empty here because otherwise there is a handler
			// for catch-all "" content type and we would not be here.
			switch req.Method {
			case http.MethodPatch:
				w.Header().Add("Accept-Patch", strings.Join(offers, ", "))
			case http.MethodPost:
				w.Header().Add("Accept-Post", strings.Join(offers, ", "))
			}

			if r.NotAcceptable != nil {
				r.NotAcceptable(w, req, params)
			} else {
				r.Error(w, req, http.StatusNotAcceptable)
			}
			return
		}

		// This might be a catch-all "" content type handler.
		handler(w, req, params)
		return
	}

	if r.NotFound != nil {
		r.NotFound(w, req, nil)
	} else {
		r.Error(w, req, http.StatusNotFound)
	}
}

func (r *Router) Path(name string, params Params, query string) (string, errors.E) {
	route, ok := r.routes[name]
	if !ok {
		return "", errors.Errorf(`route with name "%s" does not exist`, name)
	}

	var res strings.Builder
	for _, segment := range route.Segments {
		if !segment.Parameter {
			res.WriteString("/")
			res.WriteString(segment.Value)
			continue
		}

		val := params[segment.Value]
		if val == "" {
			return "", errors.Errorf(`parameter "%s" for route "%s" is missing`, segment.Value, name)
		}

		res.WriteString("/")
		res.WriteString(val)
	}

	if res.Len() == 0 {
		res.WriteString("/")
	}

	if query != "" {
		res.WriteString("?")
		res.WriteString(query)
	}

	return res.String(), nil
}
