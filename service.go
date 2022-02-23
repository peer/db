package search

import (
	"context"
	"net/http"
	"time"

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
					c = c.Interface("query", req.Form)
				}
				return c
			})
			next.ServeHTTP(w, req)
		})
	}
}

func (s *Service) RouteWith(router *httprouter.Router) http.Handler {
	router.RedirectTrailingSlash = true
	router.RedirectFixedPath = true
	router.HandleMethodNotAllowed = true

	router.GET("/d", s.ListGet)
	router.HEAD("/d", s.ListGet)
	router.POST("/d", s.ListPost)
	router.GET("/d/:id", s.Get)
	router.HEAD("/d/:id", s.Get)

	router.NotFound = http.HandlerFunc(s.NotFound)

	c := alice.New()

	c = c.Append(hlog.NewHandler(s.Log))

	c = c.Append(hlog.AccessHandler(func(req *http.Request, status, size int, duration time.Duration) {
		level := zerolog.InfoLevel
		if status >= http.StatusBadRequest {
			level = zerolog.WarnLevel
		}
		if status >= http.StatusInternalServerError {
			level = zerolog.ErrorLevel
		}
		hlog.FromRequest(req).WithLevel(level).
			Int("code", status).
			Int("size", size).
			Dur("duration", duration).
			Msg("request")
	}))

	c = c.Append(hlog.MethodHandler("method"))
	c = c.Append(RemoteAddrHandler("client"))
	c = c.Append(hlog.UserAgentHandler("agent"))
	c = c.Append(hlog.RefererHandler("referer"))
	c = c.Append(ConnectionIDHandler("connection"))
	c = c.Append(RequestIDHandler("request", "Request-ID"))
	c = c.Append(s.parseForm)
	// URLHandler should be after the parseForm middleware.
	c = c.Append(URLHandler("path", "query"))

	return c.Then(router)
}
