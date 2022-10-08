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
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
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

type routes struct {
	Routes []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"routes"`
}

type Site struct {
	Domain string `json:"domain,omitempty"`
	Index  string `json:"index"`
	Title  string `json:"title"`
	// TODO: How to keep propertiesTotal in sync with the number of properties available, if they are added or removed after initialization?
	propertiesTotal int64
	// Maps between content types, paths, and content/etags.
	compressedFiles      map[string]map[string][]byte
	compressedFilesEtags map[string]map[string]string
}

type Service struct {
	ESClient     *elastic.Client
	Log          zerolog.Logger
	Sites        map[string]Site
	Development  string
	Router       *Router
	reverseProxy *httputil.ReverseProxy
}

func NewService(esClient *elastic.Client, log zerolog.Logger, sites map[string]Site, development string) (*Service, errors.E) {
	s := &Service{
		ESClient:    esClient,
		Log:         log,
		Sites:       sites,
		Development: development,
	}

	err := s.populateProperties(context.Background())
	if err != nil {
		return s, err
	}

	return s, nil
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

func hostHandler(fieldKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			host := getHost(req.Host)
			if host != "" {
				log := zerolog.Ctx(req.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, host)
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

func logHandlerName(name string, h Handler) Handler {
	if name == "" {
		return h
	}

	return func(w http.ResponseWriter, req *http.Request, params Params) {
		log := zerolog.Ctx(req.Context())
		log.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str(zerolog.MessageFieldName, name)
		})
		h(w, req, params)
	}
}

func autoName(h Handler) string {
	fn := runtime.FuncForPC(reflect.ValueOf(h).Pointer())
	if fn == nil {
		return ""
	}
	name := fn.Name()
	i := strings.LastIndex(name, ".")
	if i != -1 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, "-fm")
	return name
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

// TODO: Move to Router struct, accepting interface{} as an object on which to search for handlers.

func (s *Service) configureRoutes(router *Router) errors.E {
	var rs routes
	errE := x.UnmarshalWithoutUnknownFields(routesConfiguration, &rs)
	if errE != nil {
		return errE
	}

	v := reflect.ValueOf(s)

	for _, route := range rs.Routes {
		foundGet := false
		foundAnyHandler := false
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			for contentTypeSuffix, contentType := range map[string]string{"HTML": "text/html", "JSON": "application/json"} {
				handlerName := fmt.Sprintf("%s%s%s", route.Name, strings.Title(strings.ToLower(method)), contentTypeSuffix) //nolint:staticcheck
				m := v.MethodByName(handlerName)
				if !m.IsValid() {
					s.Log.Debug().Str("handler", handlerName).Str("name", route.Name).Str("path", route.Path).Msg("route registration: handler not found")
					continue
				}
				s.Log.Debug().Str("handler", handlerName).Str("name", route.Name).Str("path", route.Path).Msg("route registration: handler found")
				// We cannot use Handler here because it is a named type.
				h, ok := m.Interface().(func(http.ResponseWriter, *http.Request, Params))
				if !ok {
					errE := errors.Errorf("invalid route handler type: %T", m.Interface())
					errors.Details(errE)["handler"] = handlerName
					errors.Details(errE)["name"] = route.Name
					errors.Details(errE)["path"] = route.Path
					return errE
				}
				h = logHandlerName(handlerName, h)
				errE := router.Handle(route.Name, method, contentType, route.Path, h)
				if errE != nil {
					errors.Details(errE)["handler"] = handlerName
					errors.Details(errE)["name"] = route.Name
					errors.Details(errE)["path"] = route.Path
					return errE
				}
				foundAnyHandler = true
				if method == http.MethodGet {
					foundGet = true
					errE := router.Handle(route.Name, http.MethodHead, contentType, route.Path, h)
					if errE != nil {
						errors.Details(errE)["handler"] = handlerName
						errors.Details(errE)["name"] = route.Name
						errors.Details(errE)["path"] = route.Path
						return errE
					}
				}
			}
		}
		if !foundGet {
			errE := errors.Errorf("no GET route handler found")
			errors.Details(errE)["name"] = route.Name
			errors.Details(errE)["path"] = route.Path
			return errE
		}
		if !foundAnyHandler {
			errE := errors.Errorf("no route handler found")
			errors.Details(errE)["name"] = route.Name
			errors.Details(errE)["path"] = route.Path
			return errE
		}
	}

	return nil
}

func (s *Service) RouteWith(router *Router, version string) (http.Handler, errors.E) {
	if s.Router != nil {
		panic(errors.New("RouteWith called more than once"))
	}
	s.Router = router

	errE := s.configureRoutes(router)
	if errE != nil {
		return nil, errE
	}

	if s.Development != "" {
		errE := s.makeReverseProxy()
		if errE != nil {
			return nil, errE
		}
		router.NotFound = logHandlerName(autoName(s.Proxy), s.Proxy)
		router.MethodNotAllowed = logHandlerName(autoName(s.Proxy), s.Proxy)
		router.NotAcceptable = logHandlerName(autoName(s.Proxy), s.Proxy)
	} else {
		errE := s.renderAndCompressFiles()
		if errE != nil {
			return nil, errE
		}
		errE = s.computeEtags()
		if errE != nil {
			return nil, errE
		}
		errE = s.serveStaticFiles(router)
		if errE != nil {
			return nil, errE
		}
		router.NotFound = logHandlerName(autoName(s.NotFound), s.NotFound)
		router.MethodNotAllowed = logHandlerName(autoName(s.MethodNotAllowed), s.MethodNotAllowed)
		router.NotAcceptable = logHandlerName(autoName(s.NotAcceptable), s.NotAcceptable)
	}
	router.Panic = s.handlePanic

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
	c = c.Append(hostHandler("host"))
	c = c.Append(etagHandler("etag"))
	c = c.Append(contentEncodingHandler("encoding"))
	// parseForm should be as late as possible because it can fail
	// and we want other fields to be logged.
	c = c.Append(s.parseForm)
	// URLHandler should be after the parseForm middleware.
	c = c.Append(urlHandler("path", "query"))

	return c.Then(router), nil
}

func (s *Service) renderAndCompressFiles() errors.E {
	for domain, site := range s.Sites {
		if site.compressedFiles != nil {
			return errors.New("renderAndCompressFiles called more than once")
		}

		site.compressedFiles = make(map[string]map[string][]byte)

		for _, compression := range allCompressions {
			site.compressedFiles[compression] = make(map[string][]byte)

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

				var errE errors.E
				if strings.HasSuffix(path, ".html") {
					data, errE = render(path, data, site)
					if errE != nil {
						return errE
					}
				}

				data, errE = compress(compression, data)
				if errE != nil {
					return errE
				}

				site.compressedFiles[compression][path] = data
				return nil
			})
			if err != nil {
				return errors.WithStack(err)
			}
		}

		// Map cannot be modified directly, so we modify the copy
		// and store it back into the map.
		s.Sites[domain] = site
	}

	return nil
}

func (s *Service) computeEtags() errors.E {
	for domain, site := range s.Sites {
		if site.compressedFilesEtags != nil {
			return errors.New("computeEtags called more than once")
		}

		site.compressedFilesEtags = make(map[string]map[string]string)

		for compression, files := range site.compressedFiles {
			site.compressedFilesEtags[compression] = make(map[string]string)

			for path, data := range files {
				hash := sha256.New()
				_, _ = hash.Write(data)
				etag := `"` + base64.RawURLEncoding.EncodeToString(hash.Sum(nil)) + `"`
				site.compressedFilesEtags[compression][path] = etag
			}
		}

		// Map cannot be modified directly, so we modify the copy
		// and store it back into the map.
		s.Sites[domain] = site
	}

	return nil
}
