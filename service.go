package search

import (
	"context"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/julienschmidt/httprouter"
	"github.com/justinas/alice"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"

	"gitlab.com/peerdb/search/identifier"
)

type Service struct {
	ESClient *elastic.Client
	Log      zerolog.Logger
}

func ConnectionIDHandler(fieldKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, ok := req.Context().Value(ConnectionIDContextKey).(string)
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

// RemoteAddrHandler is similar to hlog.RemoteAddrHandler, but logs only an IP, not a port.
func RemoteAddrHandler(fieldKey string) func(next http.Handler) http.Handler {
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

// RequestIDHandler is similar to hlog.RequestIDHandler, but uses identifier.NewRandom() for ID.
func RequestIDHandler(fieldKey, headerName string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			id := IDFromRequest(req)
			if id == "" {
				id = identifier.NewRandom()
				ctx = context.WithValue(ctx, RequestIDContextKey, id)
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

// URLHandler is similar to hlog.URLHandler, but it adds path and separate query string fields.
// It should be after the parseForm middleware as it uses req.Form.
func URLHandler(pathKey, queryKey string) func(next http.Handler) http.Handler {
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

func EtagHandler(fieldKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
			etag := w.Header().Get("Etag")
			if etag != "" {
				etag = strings.TrimPrefix(etag, `"`)
				etag = strings.TrimSuffix(etag, `"`)
				log := zerolog.Ctx(req.Context())
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, etag)
				})
			}
		})
	}
}

func ContentEncodingHandler(fieldKey string) func(next http.Handler) http.Handler {
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

func LogHandlerName(h func(http.ResponseWriter, *http.Request, httprouter.Params)) func(http.ResponseWriter, *http.Request, httprouter.Params) {
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

func LogHandlerNameNoParams(h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
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

// AccessHandler is similar to hlog.AccessHandler, but it uses github.com/felixge/httpsnoop.
// See: https://github.com/rs/zerolog/issues/417
func AccessHandler(f func(req *http.Request, code int, size int64, duration time.Duration)) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			m := httpsnoop.CaptureMetrics(next, w, req)
			f(req, m.Code, m.Written, m.Duration)
		})
	}
}

func (s *Service) RouteWith(router *httprouter.Router) http.Handler {
	router.RedirectTrailingSlash = true
	router.RedirectFixedPath = true
	router.HandleMethodNotAllowed = true

	router.GET("/d", LogHandlerName(s.ListGet))
	router.HEAD("/d", LogHandlerName(s.ListGet))
	router.POST("/d", LogHandlerName(s.ListPost))
	router.GET("/d/:id", LogHandlerName(s.Get))
	router.HEAD("/d/:id", LogHandlerName(s.Get))

	router.NotFound = http.HandlerFunc(LogHandlerNameNoParams(s.notFound))
	router.PanicHandler = s.handlePanic

	c := alice.New()

	c = c.Append(hlog.NewHandler(s.Log))
	c = c.Append(AccessHandler(func(req *http.Request, code int, size int64, duration time.Duration) {
		level := zerolog.InfoLevel
		if code >= http.StatusBadRequest {
			level = zerolog.WarnLevel
		}
		if code >= http.StatusInternalServerError {
			level = zerolog.ErrorLevel
		}
		zerolog.Ctx(req.Context()).WithLevel(level).
			Int("code", code).
			Int64("size", size).
			Dur("duration", duration).
			Send()
	}))
	c = c.Append(hlog.MethodHandler("method"))
	c = c.Append(RemoteAddrHandler("client"))
	c = c.Append(hlog.UserAgentHandler("agent"))
	c = c.Append(hlog.RefererHandler("referer"))
	c = c.Append(ConnectionIDHandler("connection"))
	c = c.Append(RequestIDHandler("request", "Request-ID"))
	c = c.Append(EtagHandler("etag"))
	c = c.Append(ContentEncodingHandler("encoding"))
	// parseForm should be as late as possible because it can fail
	// and we want other fields to be logged.
	c = c.Append(s.parseForm)
	// URLHandler should be after the parseForm middleware.
	c = c.Append(URLHandler("path", "query"))

	return c.Then(router)
}
