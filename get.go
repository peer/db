package search

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/gddo/httputil"
	"github.com/julienschmidt/httprouter"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

func Get(client *elastic.Client) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		ctx := req.Context()

		id := ps.ByName("id")
		if !identifier.Valid(id) {
			http.Error(w, "400 bad request", http.StatusBadRequest)
		}

		contentEncoding := httputil.NegotiateContentEncoding(req, []string{"gzip", "deflate", "identity"})
		if contentEncoding == "" {
			http.Error(w, "406 not acceptable", http.StatusNotAcceptable)
			return
		}

		headers := http.Header{}
		headers.Set("Accept-Encoding", contentEncoding)
		resp, err := client.PerformRequest(ctx, elastic.PerformRequestOptions{
			Method:  "GET",
			Path:    fmt.Sprintf("/docs/_source/%s", id),
			Headers: headers,
		})
		if elastic.IsNotFound(err) {
			NotFound(w, req)
			return
		} else if err != nil {
			Error(w, req, errors.WithStack(err))
		}

		hash := sha256.Sum256(resp.Body)
		etag := `"` + base64.RawURLEncoding.EncodeToString(hash[:]) + `"`

		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		if contentEncoding != "identity" {
			w.Header().Set("Content-Encoding", contentEncoding)
		} else {
			// TODO: Always set Content-Length.
			//       See: https://github.com/golang/go/pull/50904
			w.Header().Set("Content-Length", resp.Header.Get("Content-Length"))
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Etag", etag)

		// See: https://github.com/golang/go/issues/50905
		// See: https://github.com/golang/go/pull/50903
		http.ServeContent(w, req, "", time.Time{}, bytes.NewReader(resp.Body))
	}
}
