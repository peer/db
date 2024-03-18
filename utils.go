package search

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
	gddo "github.com/golang/gddo/httputil"
	"github.com/hashicorp/go-cleanhttp"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/tozd/identifier"
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

var allCompressions = []string{compressionBrotli, compressionGzip, compressionDeflate, compressionIdentity}

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

// NotFound is a HTTP request handler which returns a 404 error to the client.
func (s *Service) NotFound(w http.ResponseWriter, req *http.Request, _ Params) {
	// We do not use http.NotFound because http.StatusText(http.StatusNotFound)
	// is different from what http.NotFound uses, and we want to use the same pattern.
	s.error(w, req, http.StatusNotFound)
}

func (s *Service) MethodNotAllowed(w http.ResponseWriter, req *http.Request, _ Params) {
	s.error(w, req, http.StatusMethodNotAllowed)
}

func (s *Service) NotAcceptable(w http.ResponseWriter, req *http.Request, _ Params) {
	s.error(w, req, http.StatusNotAcceptable)
}

func (s *Service) BadRequest(w http.ResponseWriter, req *http.Request, _ Params) {
	s.error(w, req, http.StatusBadRequest)
}

func (s *Service) InternalServerError(w http.ResponseWriter, req *http.Request, _ Params) {
	s.error(w, req, http.StatusInternalServerError)
}

func (s *Service) error(w http.ResponseWriter, req *http.Request, code int) {
	s.Router.Error(w, req, code)
}

func (s *Service) makeReverseProxy() errors.E {
	target, err := url.Parse(s.Development)
	if err != nil {
		return errors.WithStack(err)
	}

	singleHostDirector := httputil.NewSingleHostReverseProxy(target).Director
	director := func(req *http.Request) {
		singleHostDirector(req)
		// TODO: Map origin and other headers.
	}

	// TODO: Map response cookies, other headers which include origin, and redirect locations.
	s.reverseProxy = &httputil.ReverseProxy{
		Director:      director,
		Transport:     cleanhttp.DefaultPooledTransport(),
		FlushInterval: -1,
		ErrorLog:      log.New(s.Log, "", 0),
	}
	return nil
}

func (s *Service) Proxy(w http.ResponseWriter, req *http.Request, _ Params) {
	s.reverseProxy.ServeHTTP(w, req)
}

func (s *Service) serveStaticFiles(router *Router) errors.E {
	staticName := autoName(s.StaticFile)
	staticH := logHandlerName(staticName, s.StaticFile)
	immutableName := autoName(s.ImmutableFile)
	immutableH := logHandlerName(staticName, s.ImmutableFile)

	for _, site := range s.Sites {
		for path := range site.compressedFiles[compressionIdentity] {
			if path == "/index.html" || path == "/index.json" {
				continue
			}

			var n string
			var h Handler
			if strings.HasPrefix(path, "/assets/") {
				n = fmt.Sprintf("%s:%s", immutableName, path)
				h = immutableH
			} else {
				n = fmt.Sprintf("%s:%s", staticName, path)
				h = staticH
			}

			err := router.Handle(n, http.MethodGet, path, false, h)
			if err != nil {
				return err
			}
		}

		// We can use any site to obtain all static paths,
		// so we break here after the first site.
		break
	}

	return nil
}

func (s *Service) internalServerErrorWithError(w http.ResponseWriter, req *http.Request, err errors.E) {
	var e *elastic.Error
	if errors.As(err, &e) {
		j, errJ := x.MarshalWithoutEscapeHTML(e)
		if errJ != nil {
			s.Log.Err(errJ).Fields(errors.AllDetails(err)).Msg("json marshal ElasticSearch error failure")
		} else {
			errors.Details(err)["elastic"] = json.RawMessage(j)
		}
	}

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

	s.InternalServerError(w, req, nil)
}

func (s *Service) handlePanic(w http.ResponseWriter, req *http.Request, err interface{}) {
	log := hlog.FromRequest(req)
	var e error
	switch ee := err.(type) {
	case error:
		e = errors.WithStack(ee)
	case string:
		e = errors.New(ee)
	}
	log.UpdateContext(func(c zerolog.Context) zerolog.Context {
		if e != nil {
			return c.Err(e).Fields(errors.AllDetails(e))
		}
		return c.Interface("panic", err)
	})

	s.InternalServerError(w, req, nil)
}

func (s *Service) badRequestWithError(w http.ResponseWriter, req *http.Request, err errors.E) {
	log := hlog.FromRequest(req)
	log.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Err(err).Fields(errors.AllDetails(err))
	})

	s.BadRequest(w, req, nil)
}

func (s *Service) notFoundWithError(w http.ResponseWriter, req *http.Request, err errors.E) {
	log := hlog.FromRequest(req)
	log.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Err(err).Fields(errors.AllDetails(err))
	})

	s.NotFound(w, req, nil)
}

func (s *Service) render(path string, data []byte, site Site) ([]byte, errors.E) {
	t, err := template.New(path).Parse(string(data))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var out bytes.Buffer
	err = t.Execute(&out, s.getSiteContext(site))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return out.Bytes(), nil
}

// TODO: Use a pool of compression workers?
func compress(compression string, data []byte) ([]byte, errors.E) {
	switch compression {
	case compressionBrotli:
		var buf bytes.Buffer
		writer := brotli.NewWriter(&buf)
		_, err := writer.Write(data)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}
		data = buf.Bytes()
	case compressionGzip:
		var buf bytes.Buffer
		writer := gzip.NewWriter(&buf)
		_, err := writer.Write(data)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}
		data = buf.Bytes()
	case compressionDeflate:
		var buf bytes.Buffer
		writer, err := flate.NewWriter(&buf, -1)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		_, err = writer.Write(data)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}
		data = buf.Bytes()
	case compressionIdentity:
		// Nothing.
	default:
		return nil, errors.Errorf("unknown compression: %s", compression)
	}
	return data, nil
}

func (s *Service) writeJSON(w http.ResponseWriter, req *http.Request, contentEncoding string, data interface{}, metadata http.Header) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	m := timing.NewMetric("j").Start()

	var encoded []byte
	switch d := data.(type) {
	case json.RawMessage:
		encoded = []byte(d)
	default:
		e, err := x.MarshalWithoutEscapeHTML(data)
		if err != nil {
			s.internalServerErrorWithError(w, req, errors.WithStack(err))
			return
		}
		encoded = e
	}

	m.Stop()

	if len(encoded) <= minCompressionSize {
		contentEncoding = compressionIdentity
	}

	m = timing.NewMetric("c").Start()

	encoded, errE := compress(contentEncoding, encoded)
	if errE != nil {
		s.internalServerErrorWithError(w, req, errE)
		return
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
	if len(w.Header().Values("Cache-Control")) == 0 {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Add("Vary", "Accept-Encoding")
	w.Header().Set("Etag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// See: https://github.com/golang/go/issues/50905
	// See: https://github.com/golang/go/pull/50903
	http.ServeContent(w, req, "", time.Time{}, bytes.NewReader(encoded))
}

func (s *Service) StaticFile(w http.ResponseWriter, req *http.Request, _ Params) {
	s.staticFile(w, req, req.URL.Path, false)
}

func (s *Service) ImmutableFile(w http.ResponseWriter, req *http.Request, _ Params) {
	s.staticFile(w, req, req.URL.Path, true)
}

// TODO: Use Vite's manifest.json to send preload headers.
func (s *Service) staticFile(w http.ResponseWriter, req *http.Request, path string, immutable bool) {
	contentEncoding := gddo.NegotiateContentEncoding(req, allCompressions)
	if contentEncoding == "" {
		s.NotAcceptable(w, req, nil)
		return
	}

	site, err := s.getSite(req)
	if err != nil {
		s.notFoundWithError(w, req, err)
		return
	}

	data, ok := site.compressedFiles[contentEncoding][path]
	if !ok {
		s.internalServerErrorWithError(w, req, errors.Errorf(`no data for compression %s and file "%s"`, contentEncoding, path))
		return
	}

	if len(data) <= minCompressionSize {
		contentEncoding = compressionIdentity
		data, ok = site.compressedFiles[contentEncoding][path]
		if !ok {
			s.internalServerErrorWithError(w, req, errors.Errorf(`no data for compression %s and file "%s"`, contentEncoding, path))
			return
		}
	}

	etag, ok := site.compressedFilesEtags[contentEncoding][path]
	if !ok {
		s.internalServerErrorWithError(w, req, errors.Errorf(`no etag for compression %s and file "%s"`, contentEncoding, path))
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		s.internalServerErrorWithError(w, req, errors.Errorf(`unable to determine content type for file "%s"`, path))
		return
	}

	w.Header().Set("Content-Type", contentType)
	if contentEncoding != compressionIdentity {
		w.Header().Set("Content-Encoding", contentEncoding)
	} else {
		// TODO: Always set Content-Length.
		//       See: https://github.com/golang/go/pull/50904
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	}
	if immutable {
		w.Header().Set("Cache-Control", "public,max-age=31536000,immutable,stale-while-revalidate=86400")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Add("Vary", "Accept-Encoding")
	w.Header().Set("Etag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// See: https://github.com/golang/go/issues/50905
	// See: https://github.com/golang/go/pull/50903
	http.ServeContent(w, req, "", time.Time{}, bytes.NewReader(data))
}

func (s *Service) ConnContext(ctx context.Context, c net.Conn) context.Context {
	return context.WithValue(ctx, connectionIDContextKey, identifier.New().String())
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
			s.badRequestWithError(w, req, errors.WithStack(err))
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

type metricsConn struct {
	net.Conn
	read    *int64
	written *int64
}

func (c *metricsConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	atomic.AddInt64(c.read, int64(n))
	return n, err
}

func (c *metricsConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	atomic.AddInt64(c.written, int64(n))
	return n, err
}

// insertOrReplaceDocument inserts or replaces the document based on its ID.
func InsertOrReplaceDocument(processor *elastic.BulkProcessor, index string, doc *Document) {
	req := elastic.NewBulkIndexRequest().Index(index).Id(doc.ID.String()).Doc(doc)
	processor.Add(req)
}

// updateDocument updates the document in the index, if it has not changed in the database since it was fetched (based on seqNo and primaryTerm).
func UpdateDocument(processor *elastic.BulkProcessor, index string, seqNo, primaryTerm int64, doc *Document) {
	req := elastic.NewBulkIndexRequest().Index(index).Id(doc.ID.String()).IfSeqNo(seqNo).IfPrimaryTerm(primaryTerm).Doc(&doc)
	processor.Add(req)
}
