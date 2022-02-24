package search

import (
	"context"
	"fmt"
	"net/http"
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

	"gitlab.com/peerdb/search/identifier"
)

type Service struct {
	ESClient *elastic.Client
	Log      zerolog.Logger
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

func logHandlerName(h func(http.ResponseWriter, *http.Request, httprouter.Params)) func(http.ResponseWriter, *http.Request, httprouter.Params) {
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

func logHandlerNameNoParams(h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
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
			m := httpsnoop.CaptureMetrics(next, w, req)
			milliseconds := float64(m.Duration) / float64(time.Millisecond)
			w.Header().Set(servertiming.HeaderKey, fmt.Sprintf("t;dur=%.1f", milliseconds))
			f(req, m.Code, m.Written, m.Duration)
		})
	}
}

func (s *Service) RouteWith(router *httprouter.Router) http.Handler {
	router.RedirectTrailingSlash = true
	router.RedirectFixedPath = true
	router.HandleMethodNotAllowed = true

	router.GET("/d", logHandlerName(s.listGet))
	router.HEAD("/d", logHandlerName(s.listGet))
	router.POST("/d", logHandlerName(s.listPost))
	router.GET("/d/:id", logHandlerName(s.get))
	router.HEAD("/d/:id", logHandlerName(s.get))

	router.NotFound = http.HandlerFunc(logHandlerNameNoParams(s.notFound))
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
		zerolog.Ctx(req.Context()).WithLevel(level).
			Int("code", code).
			Int64("size", size).
			Dict("metrics", metrics).
			Send()
	}))
	c = c.Append(hlog.MethodHandler("method"))
	c = c.Append(remoteAddrHandler("client"))
	c = c.Append(hlog.UserAgentHandler("agent"))
	c = c.Append(hlog.RefererHandler("referer"))
	c = c.Append(connectionIDHandler("connection"))
	c = c.Append(requestIDHandler("request", "Request-ID"))
	c = c.Append(etagHandler("etag"))
	c = c.Append(contentEncodingHandler("encoding"))
	// parseForm should be as late as possible because it can fail
	// and we want other fields to be logged.
	c = c.Append(s.parseForm)
	// URLHandler should be after the parseForm middleware.
	c = c.Append(urlHandler("path", "query"))

	return c.Then(router)
}
