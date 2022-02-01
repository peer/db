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
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/andybalholm/brotli"
	"gitlab.com/tozd/go/errors"
)

// NotFound is a HTTP request handler which returns a 404 error to the client.
func NotFound(w http.ResponseWriter, req *http.Request) {
	http.NotFound(w, req)
}

func internalServerError(w http.ResponseWriter, req *http.Request, err errors.E) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}

	// TODO: Use logger.
	fmt.Fprintf(os.Stderr, "internal server error: %+v", err)
	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}

func badRequest(w http.ResponseWriter, req *http.Request, err errors.E) {
	// TODO: Use logger.
	fmt.Fprintf(os.Stderr, "bad request: %+v", err)
	http.Error(w, "400 bad request", http.StatusBadRequest)
}

func writeJSON(w http.ResponseWriter, req *http.Request, contentEncoding string, data interface{}, metadata http.Header) {
	encoded, err := json.Marshal(data)
	if err != nil {
		internalServerError(w, req, errors.WithStack(err))
		return
	}

	// Compress only if content is larger than 1 KB.
	if len(encoded) <= 1024 {
		contentEncoding = "identity"
	}

	// TODO: Use a pool of compression workers?
	switch contentEncoding {
	case "br":
		var buf bytes.Buffer
		writer := brotli.NewWriter(&buf)
		_, err := writer.Write(encoded)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			internalServerError(w, req, errors.WithStack(err))
			return
		}
		encoded = buf.Bytes()
	case "gzip":
		var buf bytes.Buffer
		writer := gzip.NewWriter(&buf)
		_, err := writer.Write(encoded)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			internalServerError(w, req, errors.WithStack(err))
			return
		}
		encoded = buf.Bytes()
	case "deflate":
		var buf bytes.Buffer
		writer, err := flate.NewWriter(&buf, -1)
		if err != nil {
			internalServerError(w, req, errors.WithStack(err))
			return
		}
		_, err = writer.Write(encoded)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			internalServerError(w, req, errors.WithStack(err))
			return
		}
		encoded = buf.Bytes()
	case "identity":
		// Nothing.
	}

	hash := sha256.New()
	_, _ = hash.Write(encoded)

	for key, value := range metadata {
		w.Header()["PeerDB-"+key] = value
		_, _ = hash.Write([]byte(key))
		for _, v := range value {
			_, _ = hash.Write([]byte(v))
		}
	}

	etag := `"` + base64.RawURLEncoding.EncodeToString(hash.Sum(nil)) + `"`

	w.Header().Set("Content-Type", "application/json")
	if contentEncoding != "identity" {
		w.Header().Set("Content-Encoding", contentEncoding)
	} else {
		// TODO: Always set Content-Length.
		//       See: https://github.com/golang/go/pull/50904
		w.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Etag", etag)

	// See: https://github.com/golang/go/issues/50905
	// See: https://github.com/golang/go/pull/50903
	http.ServeContent(w, req, "", time.Time{}, bytes.NewReader(encoded))
}
