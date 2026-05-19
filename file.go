package peerdb

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

// rfc5987Filename returns filename percent-encoded for use as the value of a
// Content-Disposition filename* parameter. url.PathEscape leaves the apostrophe
// unencoded, but in RFC 5987's value-chars an apostrophe is the language-tag
// delimiter and breaks parsers; ReplaceAll fixes that one specific over-allowance.
func rfc5987Filename(filename string) string {
	return strings.ReplaceAll(url.PathEscape(filename), "'", "%27")
}

// 10 MB.
const maxPayloadSize = int64(10 << 20)

type storageBeginUploadRequest struct {
	Size      int64  `json:"size"`
	MediaType string `json:"mediaType"`
	Filename  string `json:"filename"`
}

// TODO: Add Validate to beginUploadRequest.
//       We should validate that Size is non-negative and under some limit.
//       That media type looks correct (or is of allowlisted media type.
//       We should UTF8 normalize filename and sanitize it to really be a filename only.

type storageBeginUploadResponse struct {
	Session identifier.Identifier `json:"session"`
}

// StorageBeginUploadPostAPI handles POST requests to begin a chunked file upload session.
func (s *Service) StorageBeginUploadPostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	var payload storageBeginUploadRequest
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &payload)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	// TODO: Support configuring base and not just use the domain.
	base := []string{site.Domain}

	session, errE := site.Base.BeginUploadNew(ctx, base, payload.Size, payload.MediaType, payload.Filename)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, storageBeginUploadResponse{Session: session}, nil)
}

// StorageUploadChunkPostAPI handles POST requests to upload a chunk of data during a file upload session.
func (s *Service) StorageUploadChunkPostAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	if !req.Form.Has("start") {
		s.BadRequestWithError(w, req, errors.New(`"start" query parameter is missing`))
		return
	}

	start, err := strconv.ParseInt(req.Form.Get("start"), 10, 64)
	if err != nil {
		s.BadRequestWithError(w, req, errors.WithStack(err))
		return
	}

	if start < 0 {
		s.BadRequestWithError(w, req, errors.New(`negative "start" query parameter`))
		return
	}

	if req.ContentLength < 0 || req.ContentLength > maxPayloadSize {
		s.BadRequestWithError(w, req, errors.New("invalid content length"))
		return
	}

	buffer := make([]byte, req.ContentLength)
	_, err = io.ReadFull(req.Body, buffer)
	if err != nil {
		s.BadRequestWithError(w, req, errors.WithStack(err))
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	errE = site.Base.UploadChunk(ctx, session, buffer, start)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, storage.ErrInvalidChunk) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, []byte(`{"success":true}`), nil)
}

// StorageListChunksGetAPI handles GET requests to list all uploaded chunks for a file upload session.
func (s *Service) StorageListChunksGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	chunks, errE := site.Base.ListChunks(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, chunks, nil)
}

type storageGetChunkResponse struct {
	Start  int64 `json:"start"`
	Length int64 `json:"length"`
}

// StorageGetChunkGetAPI handles GET requests to retrieve start position and length of a specific chunk in a file upload session.
func (s *Service) StorageGetChunkGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	chunk, err := strconv.ParseInt(params["chunk"], 10, 64)
	if err != nil {
		s.BadRequestWithError(w, req, errors.WithStack(err))
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	start, length, errE := site.Base.GetChunk(ctx, session, chunk)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrOperationNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, storageGetChunkResponse{Start: start, Length: length}, nil)
}

type emptyRequest struct{}

// StorageEndUploadPostAPI handles POST requests to finalize a file upload session.
func (s *Service) StorageEndUploadPostAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	var ea emptyRequest
	errE = x.DecodeJSONWithoutUnknownFields(req.Body, &ea)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	errE = site.Base.EndUpload(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, storage.ErrEndNotPossible) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, []byte(`{"success":true}`), nil)
}

// StorageDiscardUploadPostAPI handles POST requests to discard a file upload session.
func (s *Service) StorageDiscardUploadPostAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	var ea emptyRequest
	errE = x.DecodeJSONWithoutUnknownFields(req.Body, &ea)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	errE = site.Base.DiscardUpload(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, []byte(`{"success":true}`), nil)
}

// StorageUploadGetAPI handles GET requests to retrieve the status of a file upload session.
func (s *Service) StorageUploadGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	sessionEnded, completeMetadata, errE := site.Base.GetUploadSession(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	if completeMetadata != nil {
		s.WriteJSON(w, req, struct {
			*storage.CompleteMetadata

			Active bool `json:"active"`
		}{
			CompleteMetadata: completeMetadata,
			Active:           false,
		}, nil)
	} else if sessionEnded {
		s.WriteJSON(w, req, `{"active":false}`, nil)
	} else {
		s.WriteJSON(w, req, `{"active":true}`, nil)
	}
}

// StorageGetGet handles GET requests to retrieve a stored file by its ID.
//
// An optional "version" query parameter can be used to retrieve a specific version.
func (s *Service) StorageGetGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	var reqVersion *store.Version
	if req.Form.Has("version") {
		v, errE := store.VersionFromString(req.Form.Get("version"))
		if errE != nil {
			s.BadRequestWithError(w, req, errE)
			return
		}
		reqVersion = &v
	}

	site := waf.MustGetSite[*Site](ctx)

	var data []byte
	var metadata *storage.FileMetadata
	var version store.Version

	if reqVersion != nil {
		data, metadata, version, _, errE = site.Base.GetFile(ctx, id, *reqVersion)
	} else {
		data, metadata, version, _, errE = site.Base.GetFileLatest(ctx, id)
	}

	if errors.Is(errE, store.ErrValueNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		waf.Error(w, req, http.StatusUnauthorized)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	w.Header().Set("Content-Type", metadata.MediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Etag", metadata.Etag)
	w.Header().Set("Version", version.String())
	if metadata.Filename != "" {
		w.Header().Set("Content-Disposition", `inline; filename*=UTF-8''`+rfc5987Filename(metadata.Filename))
	}

	http.ServeContent(w, req, "", time.Time(metadata.At), bytes.NewReader(data))
}

// StorageChangesGetAPI handles GET requests to list changes in a file changeset.
func (s *Service) StorageChangesGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	s.changesetChangesGetAPI(w, req, params, func(ctx context.Context, changesetID identifier.Identifier, after *identifier.Identifier) ([]store.Change, errors.E) {
		return waf.MustGetSite[*Site](ctx).Base.GetFileChangesetChanges(ctx, changesetID, after)
	})
}

// StorageChangesGetGet handles GET requests to retrieve a file from a changeset.
func (s *Service) StorageChangesGetGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	changesetID, errE := identifier.MaybeString(params["changeset"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"changeset" is not a valid identifier`))
		return
	}

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	// Revision 0 means latest revision.
	data, metadata, version, _, errE := site.Base.GetFileFromChangeset(ctx, changesetID, id, 0)
	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrChangesetNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	w.Header().Set("Content-Type", metadata.MediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Etag", metadata.Etag)
	w.Header().Set("Version", version.String())
	if metadata.Filename != "" {
		w.Header().Set("Content-Disposition", `inline; filename*=UTF-8''`+rfc5987Filename(metadata.Filename))
	}

	http.ServeContent(w, req, "", time.Time(metadata.At), bytes.NewReader(data))
}
