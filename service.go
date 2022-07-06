package search

import (
	"bufio"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/julienschmidt/httprouter"
	"github.com/justinas/alice"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search/identifier"
)

//go:embed routes.json
var routesConfiguration []byte

//go:embed dist
var distFiles embed.FS

var (
	compressedFiles      map[string]map[string][]byte
	compressedFilesEtags map[string]map[string]string
)

type routes struct {
	Routes []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"routes"`
}

type Service struct {
	ESClient     *elastic.Client
	Log          zerolog.Logger
	Development  string
	reverseProxy *httputil.ReverseProxy
	routes       map[string][]pathSegment
}

func connectionIDHandler(fieldKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, ok := req.Context().Value(connectionIDContextKey).(string)
			if ok {
				log := zerolog.Ctx(req.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, id)
				})
			}
			next.ServeHTTP(w, req)
		})
	}
}

func protocolHandler(fieldKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			proto := strings.TrimPrefix(req.Proto, "HTTP/")
			log := zerolog.Ctx(req.Context())
			log.UpdateContext(func(c zerolog.Context) zerolog.Context {
				return c.Str(fieldKey, proto)
			})
			next.ServeHTTP(w, req)
		})
	}
}

// remoteAddrHandler is similar to hlog.remoteAddrHandler, but logs only an IP, not a port.
func remoteAddrHandler(fieldKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip := getHost(req.RemoteAddr)
			if ip != "" {
				log := zerolog.Ctx(req.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, ip)
				})
			}
			next.ServeHTTP(w, req)
		})
	}
}

// requestIDHandler is similar to hlog.requestIDHandler, but uses identifier.NewRandom() for ID.
func requestIDHandler(fieldKey, headerName string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			id := idFromRequest(req)
			if id == "" {
				id = identifier.NewRandom()
				ctx = context.WithValue(ctx, requestIDContextKey, id)
				req = req.WithContext(ctx)
			}
			if fieldKey != "" {
				log := zerolog.Ctx(ctx)
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, id)
				})
			}
			if headerName != "" {
				w.Header().Set(headerName, id)
			}
			next.ServeHTTP(w, req)
		})
	}
}

// urlHandler is similar to hlog.urlHandler, but it adds path and separate query string fields.
// It should be after the parseForm middleware as it uses req.Form.
func urlHandler(pathKey, queryKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			log := zerolog.Ctx(req.Context())
			log.UpdateContext(func(c zerolog.Context) zerolog.Context {
				c = c.Str(pathKey, req.URL.Path)
				if len(req.Form) > 0 {
					c = logValues(c, "query", req.Form)
				}
				return c
			})
			next.ServeHTTP(w, req)
		})
	}
}

func etagHandler(fieldKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
			etag := w.Header().Get("Etag")
			if etag != "" {
				etag = strings.ReplaceAll(etag, `"`, "")
				log := zerolog.Ctx(req.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, etag)
				})
			}
		})
	}
}

func contentEncodingHandler(fieldKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
			contentEncoding := w.Header().Get("Content-Encoding")
			if contentEncoding != "" {
				log := zerolog.Ctx(req.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, contentEncoding)
				})
			}
		})
	}
}

func logHandlerName(name string, h func(http.ResponseWriter, *http.Request, httprouter.Params)) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	if name == "" {
		return h
	}

	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		log := zerolog.Ctx(req.Context())
		log.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str(zerolog.MessageFieldName, name)
		})
		h(w, req, ps)
	}
}

func logHandlerAutoName(h func(http.ResponseWriter, *http.Request, httprouter.Params)) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	name := runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name()
	i := strings.LastIndex(name, ".")
	if i != -1 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, "-fm")

	if name == "" {
		return h
	}

	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		log := zerolog.Ctx(req.Context())
		log.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str(zerolog.MessageFieldName, name)
		})
		h(w, req, ps)
	}
}

func logHandlerAutoNameNoParams(h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	name := runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name()
	i := strings.LastIndex(name, ".")
	if i != -1 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, "-fm")

	if name == "" {
		return h
	}

	return func(w http.ResponseWriter, req *http.Request) {
		log := zerolog.Ctx(req.Context())
		log.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str(zerolog.MessageFieldName, name)
		})
		h(w, req)
	}
}

// accessHandler is similar to hlog.accessHandler, but it uses github.com/felixge/httpsnoop.
// See: https://github.com/rs/zerolog/issues/417
// Afterwards, it was extended with Server-Timing trailer.
func accessHandler(f func(req *http.Request, code int, size int64, duration time.Duration)) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Trailer", servertiming.HeaderKey)
			// We initialize Metrics ourselves so that if Code is never set it is logged as zero.
			// This allows one to detect calls which has been canceled early and websocket upgrades.
			// See: https://github.com/felixge/httpsnoop/issues/17
			m := httpsnoop.Metrics{}
			m.CaptureMetrics(w, func(ww http.ResponseWriter) {
				next.ServeHTTP(ww, req)
			})
			milliseconds := float64(m.Duration) / float64(time.Millisecond)
			w.Header().Set(servertiming.HeaderKey, fmt.Sprintf("t;dur=%.1f", milliseconds))
			f(req, m.Code, m.Written, m.Duration)
		})
	}
}

// removeMetadataHeaders removes PeerDB metadata headers in a response
// if the response is 304 Not Modified because clients will then use the cached
// version of the response (and metadata headers there). This works because metadata
// headers are included in the Etag, so 304 Not Modified means that metadata headers
// have not changed either.
func removeMetadataHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(httpsnoop.Wrap(w, httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(code int) {
					if code == http.StatusNotModified {
						headers := w.Header()
						for header := range headers {
							if strings.HasPrefix(strings.ToLower(header), peerDBMetadataHeaderPrefix) {
								headers.Del(header)
							}
						}
					}
					next(code)
				}
			},
		}), req)
	})
}

// websocketHandler records metrics about a websocket.
func websocketHandler(fieldKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var websocket bool
			var read int64
			var written int64
			next.ServeHTTP(httpsnoop.Wrap(w, httpsnoop.Hooks{
				Hijack: func(next httpsnoop.HijackFunc) httpsnoop.HijackFunc {
					return func() (net.Conn, *bufio.ReadWriter, error) {
						conn, bufrw, err := next()
						if err != nil {
							return conn, bufrw, err
						}
						websocket = true
						return &metricsConn{
							Conn:    conn,
							read:    &read,
							written: &written,
						}, bufrw, err
					}
				},
			}), req)
			if websocket {
				log := zerolog.Ctx(req.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					data := zerolog.Dict()
					data.Int64("fromClient", read)
					data.Int64("toClient", written)
					return c.Dict(fieldKey, data)
				})
			}
		})
	}
}

func (s *Service) configureRoutes(router *httprouter.Router) errors.E {
	var rs routes
	errE := x.UnmarshalWithoutUnknownFields(routesConfiguration, &rs)
	if errE != nil {
		return errE
	}

	v := reflect.ValueOf(s)

	for _, route := range rs.Routes {
		foundGet := false
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			mux := contentTypeMux{}
			vm := reflect.ValueOf(&mux)
			for _, contentType := range []string{"HTML", "JSON"} {
				handlerName := fmt.Sprintf("%s%s%s", route.Name, strings.Title(strings.ToLower(method)), contentType) //nolint:staticcheck
				m := v.MethodByName(handlerName)
				if !m.IsValid() {
					s.Log.Debug().Str("handler", handlerName).Str("name", route.Name).Str("path", route.Path).Msg("route registration: handler not found")
					continue
				}
				s.Log.Debug().Str("handler", handlerName).Str("name", route.Name).Str("path", route.Path).Msg("route registration: handler found")
				h, ok := m.Interface().(func(http.ResponseWriter, *http.Request, httprouter.Params))
				if !ok {
					errE := errors.Errorf("invalid route handler type: %T", m.Interface())
					errors.Details(errE)["handler"] = handlerName
					errors.Details(errE)["name"] = route.Name
					errors.Details(errE)["path"] = route.Path
					return errE
				}
				h = logHandlerName(handlerName, h)
				vf := vm.Elem().FieldByName(contentType)
				vf.Set(reflect.ValueOf(h))
			}
			if mux.IsEmpty() {
				continue
			}
			router.Handle(method, route.Path, mux.Handle)
			if method == http.MethodGet {
				foundGet = true
				router.Handle(http.MethodHead, route.Path, mux.Handle)
			}
		}
		if !foundGet {
			errE := errors.Errorf("no GET route handler found")
			errors.Details(errE)["name"] = route.Name
			errors.Details(errE)["path"] = route.Path
			return errE
		}

		s.routes[route.Name] = parsePath(route.Path)
	}

	return nil
}

func (s *Service) RouteWith(router *httprouter.Router, version string) (http.Handler, errors.E) {
	if s.routes != nil {
		panic(errors.New("RouteWith called more than once"))
	}

	s.routes = make(map[string][]pathSegment)

	router.RedirectTrailingSlash = true
	router.RedirectFixedPath = true
	router.HandleMethodNotAllowed = true

	errE := s.configureRoutes(router)
	if errE != nil {
		return nil, errE
	}

	if s.Development != "" {
		errE := s.makeReverseProxy()
		if errE != nil {
			return nil, errE
		}
		router.NotFound = http.HandlerFunc(logHandlerAutoNameNoParams(s.Proxy))
	} else {
		// TODO: Convert index.html into a template to be able to inject data it.
		errE := compressFiles()
		if errE != nil {
			return nil, errE
		}
		errE = computeEtags()
		if errE != nil {
			return nil, errE
		}
		errE = s.serveStaticFiles(router)
		if errE != nil {
			return nil, errE
		}
		router.NotFound = http.HandlerFunc(logHandlerAutoNameNoParams(s.NotFound))
	}
	router.PanicHandler = s.handlePanic

	c := alice.New()

	c = c.Append(hlog.NewHandler(s.Log))
	// It has to be before accessHandler so that it can access the timing context.
	c = c.Append(func(next http.Handler) http.Handler {
		return servertiming.Middleware(next, nil)
	})
	c = c.Append(accessHandler(func(req *http.Request, code int, size int64, duration time.Duration) {
		level := zerolog.InfoLevel
		if code >= http.StatusBadRequest {
			level = zerolog.WarnLevel
		}
		if code >= http.StatusInternalServerError {
			level = zerolog.ErrorLevel
		}
		timing := servertiming.FromContext(req.Context())
		metrics := zerolog.Dict()
		for _, metric := range timing.Metrics {
			metrics.Dur(metric.Name, metric.Duration)
		}
		metrics.Dur("t", duration)
		l := zerolog.Ctx(req.Context()).WithLevel(level)
		if version != "" {
			l = l.Str("version", version)
		}
		if code != 0 {
			l = l.Int("code", code)
		}
		l.Int64("size", size).
			Dict("metrics", metrics).
			Send()
	}))
	c = c.Append(removeMetadataHeaders)
	c = c.Append(websocketHandler("ws"))
	c = c.Append(hlog.MethodHandler("method"))
	c = c.Append(remoteAddrHandler("client"))
	c = c.Append(hlog.UserAgentHandler("agent"))
	c = c.Append(hlog.RefererHandler("referer"))
	c = c.Append(connectionIDHandler("connection"))
	c = c.Append(requestIDHandler("request", "Request-ID"))
	c = c.Append(protocolHandler("proto"))
	c = c.Append(etagHandler("etag"))
	c = c.Append(contentEncodingHandler("encoding"))
	// parseForm should be as late as possible because it can fail
	// and we want other fields to be logged.
	c = c.Append(s.parseForm)
	// URLHandler should be after the parseForm middleware.
	c = c.Append(urlHandler("path", "query"))

	return c.Then(router), nil
}

type pathSegment struct {
	Value     string
	Parameter bool
	Optional  bool
}

func parsePath(path string) []pathSegment {
	parts := strings.Split(path, "/")
	segments := []pathSegment{}
	for _, part := range parts {
		if part == "" {
			continue
		}
		var segment pathSegment
		if strings.HasPrefix(part, ":") {
			segment.Value = strings.TrimPrefix(part, ":")
			segment.Parameter = true
			segment.Optional = false
		} else if strings.HasPrefix(part, "*") {
			segment.Value = strings.TrimPrefix(part, "*")
			segment.Parameter = true
			segment.Optional = true
		} else {
			segment.Value = part
		}
		segments = append(segments, segment)
	}
	return segments
}

func (s *Service) path(name string, params url.Values, query string) (string, errors.E) {
	segments, ok := s.routes[name]
	if !ok {
		return "", errors.Errorf(`route with name "%s" does not exist`, name)
	}

	var res strings.Builder
	for _, segment := range segments {
		if !segment.Parameter {
			res.WriteString("/")
			res.WriteString(segment.Value)
			continue
		}

		val := params.Get(segment.Value)
		if val != "" {
			res.WriteString("/")
			res.WriteString(val)
			continue
		}

		if !segment.Optional {
			return "", errors.Errorf(`parameter "%s" for route "%s" is required`, segment.Value, name)
		}
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

func compressFiles() errors.E {
	if compressedFiles != nil {
		return errors.New("compressFiles called more than once")
	}

	compressedFiles = make(map[string]map[string][]byte)

	for _, compression := range allCompressions {
		compressedFiles[compression] = make(map[string][]byte)

		err := fs.WalkDir(distFiles, "dist", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return errors.WithStack(err)
			}
			if d.IsDir() {
				return nil
			}

			data, err := distFiles.ReadFile(path)
			if err != nil {
				return errors.WithStack(err)
			}
			path = strings.TrimPrefix(path, "dist")

			data, errE := compress(compression, data)
			if errE != nil {
				return errE
			}

			compressedFiles[compression][path] = data
			return nil
		})
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func computeEtags() errors.E {
	if compressedFilesEtags != nil {
		return errors.New("computeEtags called more than once")
	}

	compressedFilesEtags = make(map[string]map[string]string)

	for compression, files := range compressedFiles {
		compressedFilesEtags[compression] = make(map[string]string)

		for path, data := range files {
			hash := sha256.New()
			_, _ = hash.Write(data)
			etag := `"` + base64.RawURLEncoding.EncodeToString(hash.Sum(nil)) + `"`
			compressedFilesEtags[compression][path] = etag
		}
	}

	return nil
}
