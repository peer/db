package search

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"time"

	"github.com/andybalholm/brotli"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

const (
	compressionBrotli   = "br"
	compressionGzip     = "gzip"
	compressionDeflate  = "deflate"
	compressionIdentity = "identity"

	// Compress only if content is larger than 1 KB.
	minCompressionSize = 1024

	// It should be kept all lower case so that it is easier to
	// compare against in the case insensitive manner.
	peerDBMetadataHeaderPrefix = "peerdb-"
)

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

// connectionIDContextKey provides a random ID for each HTTP connection.
var connectionIDContextKey = &contextKey{"connection-id"}

// requestIDContextKey provides a random ID for each HTTP request.
var requestIDContextKey = &contextKey{"request-id"}

func getHost(hostPort string) string {
	if hostPort == "" {
		return ""
	}

	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return hostPort
	}
	return host
}

// notFound is a HTTP request handler which returns a 404 error to the client.
func (s *Service) notFound(w http.ResponseWriter, req *http.Request) {
	http.NotFound(w, req)
}

func (s *Service) internalServerError(w http.ResponseWriter, req *http.Request, err errors.E) {
	log := hlog.FromRequest(req)
	log.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Err(err).Fields(errors.AllDetails(err))
	})
	if errors.Is(err, context.Canceled) {
		log.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("context", "canceled")
		})
		return
	} else if errors.Is(err, context.DeadlineExceeded) {
		log.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("context", "deadline exceeded")
		})
		return
	}

	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}

func (s *Service) handlePanic(w http.ResponseWriter, req *http.Request, err interface{}) {
	log := hlog.FromRequest(req)
	var e error
	switch ee := err.(type) {
	case error:
		e = ee
	case string:
		e = errors.New(ee)
	}
	log.UpdateContext(func(c zerolog.Context) zerolog.Context {
		if e != nil {
			return c.Err(e).Fields(errors.AllDetails(e))
		}
		return c.Interface("panic", err)
	})

	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}

func (s *Service) badRequest(w http.ResponseWriter, req *http.Request, err errors.E) {
	log := hlog.FromRequest(req)
	log.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Err(err).Fields(errors.AllDetails(err))
	})

	http.Error(w, "400 bad request", http.StatusBadRequest)
}

func (s *Service) writeJSON(w http.ResponseWriter, req *http.Request, contentEncoding string, data interface{}, metadata http.Header) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	m := timing.NewMetric("j").Start()

	encoded, err := json.Marshal(data)
	if err != nil {
		s.internalServerError(w, req, errors.WithStack(err))
		return
	}

	m.Stop()

	if len(encoded) <= minCompressionSize {
		contentEncoding = compressionIdentity
	}

	m = timing.NewMetric("c").Start()

	// TODO: Use a pool of compression workers?
	switch contentEncoding {
	case compressionBrotli:
		var buf bytes.Buffer
		writer := brotli.NewWriter(&buf)
		_, err := writer.Write(encoded)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			s.internalServerError(w, req, errors.WithStack(err))
			return
		}
		encoded = buf.Bytes()
	case compressionGzip:
		var buf bytes.Buffer
		writer := gzip.NewWriter(&buf)
		_, err := writer.Write(encoded)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			s.internalServerError(w, req, errors.WithStack(err))
			return
		}
		encoded = buf.Bytes()
	case compressionDeflate:
		var buf bytes.Buffer
		writer, err := flate.NewWriter(&buf, -1)
		if err != nil {
			s.internalServerError(w, req, errors.WithStack(err))
			return
		}
		_, err = writer.Write(encoded)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			s.internalServerError(w, req, errors.WithStack(err))
			return
		}
		encoded = buf.Bytes()
	case compressionIdentity:
		// Nothing.
	}

	m.Stop()

	hash := sha256.New()
	_, _ = hash.Write(encoded)

	for key, value := range metadata {
		w.Header()[textproto.CanonicalMIMEHeaderKey(peerDBMetadataHeaderPrefix+key)] = value
		_, _ = hash.Write([]byte(key))
		for _, v := range value {
			_, _ = hash.Write([]byte(v))
		}
	}

	etag := `"` + base64.RawURLEncoding.EncodeToString(hash.Sum(nil)) + `"`

	log := hlog.FromRequest(req)
	if len(metadata) > 0 {
		log.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return logValues(c, "metadata", metadata)
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if contentEncoding != compressionIdentity {
		w.Header().Set("Content-Encoding", contentEncoding)
	} else {
		// TODO: Always set Content-Length.
		//       See: https://github.com/golang/go/pull/50904
		w.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Etag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// See: https://github.com/golang/go/issues/50905
	// See: https://github.com/golang/go/pull/50903
	http.ServeContent(w, req, "", time.Time{}, bytes.NewReader(encoded))
}

func (s *Service) ConnContext(ctx context.Context, c net.Conn) context.Context {
	return context.WithValue(ctx, connectionIDContextKey, identifier.NewRandom())
}

func idFromRequest(req *http.Request) string {
	if req == nil {
		return ""
	}
	id, ok := req.Context().Value(requestIDContextKey).(string)
	if ok {
		return id
	}
	return ""
}

func (s *Service) parseForm(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		err := req.ParseForm()
		if err != nil {
			s.badRequest(w, req, errors.WithStack(err))
			return
		}
		next.ServeHTTP(w, req)
	})
}

type valuesLogObjectMarshaler map[string][]string

func (v valuesLogObjectMarshaler) MarshalZerologObject(e *zerolog.Event) {
	for key, values := range v {
		arr := zerolog.Arr()
		for _, val := range values {
			arr.Str(val)
		}
		e.Array(key, arr)
	}
}

func logValues(c zerolog.Context, field string, values map[string][]string) zerolog.Context {
	if len(values) == 0 {
		return c
	}

	return c.Object(field, valuesLogObjectMarshaler(values))
}
