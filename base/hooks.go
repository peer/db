package base

import (
	"context"
	"encoding/json"
	"io"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

// unmarshalFetchedDoc unmarshals a fetched raw document, nil when the document is deleted at the
// read version, and passes the read error through. An unmarshal failure is joined with the read
// error.
func unmarshalFetchedDoc(data json.RawMessage, readErrE errors.E) (*document.D, errors.E) {
	if data == nil {
		return nil, readErrE
	}
	doc := new(document.D)
	errE := x.UnmarshalWithoutUnknownFields(data, doc)
	if errE != nil {
		return nil, errors.Join(readErrE, errE)
	}
	return doc, readErrE
}

// readVersionedDocumentWithLatest reads the document at the given version together with the document
// at its latest version for the post-hooks. It returns the fetched document, the document at the
// latest version, the fetched document's metadata, resolved version, and parent changesets, and any
// error for the post-hooks to observe.
func (b *B) readVersionedDocumentWithLatest(
	ctx context.Context, id identifier.Identifier, version store.Version,
	fetch func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E),
) (*document.D, *document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	data, metadata, resolved, parentChangesets, errE := b.Documents().GetLatest(ctx, id)
	latest, errE := unmarshalFetchedDoc(data, errE)
	if errE == nil && version.Equals(resolved) {
		// Optimization. If the requested version is the latest version, we can return the latest
		// document directly without fetching it again. We do clone it so that the caller can mutate
		// the document without affecting the latest document.
		doc, errE := latest.Clone()
		return doc, latest, metadata, resolved, parentChangesets, errE
	}
	// The requested version is read even when reading the latest version failed (e.g. the document
	// is deleted at its latest version): the errors are joined and the post-hooks decide about
	// access by transforming the error.
	data, metadata, resolved, parentChangesets, errE2 := fetch()
	if errE2 != nil {
		errE = errors.Join(errE, errE2)
	}
	doc, errE := unmarshalFetchedDoc(data, errE)
	return doc, latest, metadata, resolved, parentChangesets, errE
}

// withDocumentHooks runs the document pre-hooks, reads the raw document, unmarshals it, and runs the
// document post-hooks, returning the post-hook document together with its metadata, version, and
// parent changesets.
//
// version is the version fetch reads and is passed to the pre-hooks: nil means the document's
// latest version. fetch is the store read and returns the raw document, which is nil when the
// document is deleted at the read version. Besides the fetched document the post-hooks receive the
// document at its latest version (see DocumentPostHooks), and they always run, even when doc is nil
// or the read errored, so they can observe and transform the error.
func (b *B) withDocumentHooks(
	ctx context.Context, id identifier.Identifier, version *store.Version,
	fetch func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E),
) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	for _, hook := range b.DocumentPreHooks {
		errE := hook(ctx, id, version)
		if errE != nil {
			return nil, nil, store.Version{}, nil, errE
		}
	}
	var doc, latest *document.D
	var metadata *store.DocumentMetadata
	var resolved store.Version
	var parentChangesets []store.Version
	var errE errors.E
	if version == nil || len(b.DocumentPostHooks) == 0 {
		var data json.RawMessage
		data, metadata, resolved, parentChangesets, errE = fetch()
		doc, errE = unmarshalFetchedDoc(data, errE)
		if doc != nil && len(b.DocumentPostHooks) > 0 {
			// The latest document is a separate instance of the same content, so hooks which
			// transform the fetched document do not affect the latest document later hooks receive.
			var errE2 errors.E
			latest, errE2 = doc.Clone()
			if errE2 != nil {
				errE = errors.Join(errE, errE2)
			}
		}
	} else {
		doc, latest, metadata, resolved, parentChangesets, errE = b.readVersionedDocumentWithLatest(ctx, id, *version, fetch)
	}
	for _, hook := range b.DocumentPostHooks {
		doc, metadata, resolved, parentChangesets, errE = hook(ctx, doc, latest, metadata, resolved, parentChangesets, errE)
	}
	return doc, metadata, resolved, parentChangesets, errE
}

// FilterSessionDocument runs the session document hooks on the state of an edit session and returns what
// they leave of it (see SessionDocumentHooks), so the state can be handed to the caller filtered the same
// as a document read through the store is.
//
// A hook which hides the state whole reports it with auth.ErrAccessDenied, and so does this method when
// a hook keeps nothing of the state: there is then nothing of the session to show the caller.
func (b *B) FilterSessionDocument(ctx context.Context, doc *document.D, beginMetadata *DocumentBeginMetadata) (*document.D, errors.E) {
	for _, hook := range b.SessionDocumentHooks {
		var errE errors.E
		doc, errE = hook(ctx, doc, beginMetadata)
		if errE != nil {
			return nil, errE
		}
		if doc == nil {
			return nil, errors.WithStack(auth.ErrAccessDenied)
		}
	}
	return doc, nil
}

// readVersionedFileWithLatest reads the file at the given version together with the version of the
// file's latest version for the post-hooks. It returns the fetched file, the file's latest version,
// the fetched file's metadata, resolved version, and parent changesets, and any error for the
// post-hooks to observe.
func (b *B) readVersionedFileWithLatest(
	ctx context.Context, id identifier.Identifier, version store.Version,
	fetch func() (io.ReadSeekCloser, *storage.FileMetadata, store.Version, []store.Version, errors.E),
) (io.ReadSeekCloser, *store.Version, *storage.FileMetadata, store.Version, []store.Version, errors.E) {
	// The latest version is read raw (without opening the file): it serves only as input for the
	// post-hooks.
	hash, metadata, lv, parentChangesets, errE := b.Files().GetLatest(ctx, id)
	// It is nil for the post-hooks when reading it failed, e.g. when the file is deleted
	// at its latest version (see FilePostHooks).
	var latestVersion *store.Version
	if errE == nil {
		latestVersion = &lv
	}
	if errE == nil && version.Equals(lv) {
		// Optimization. If the requested version is the latest version, we can open the file
		// directly from the hash we already read without reading the store again.
		file, errE := b.files.Open(hash)
		return file, latestVersion, metadata, lv, parentChangesets, errE
	}
	// The requested version is read even when reading the file's latest version above failed (e.g.
	// the file is deleted at its latest version): the errors are joined and the post-hooks decide
	// about access by transforming the error.
	file, metadata, resolved, parentChangesets, errE2 := fetch()
	if errE2 != nil {
		errE = errors.Join(errE, errE2)
	}
	return file, latestVersion, metadata, resolved, parentChangesets, errE
}

// withFileHooks runs the file pre-hooks, reads the file via fetch, and runs the file post-hooks,
// returning the post-hook file handle together with its metadata, version, and parent changesets.
//
// version is the version fetch reads and is passed to the pre-hooks: nil means the file's latest
// version. fetch is the store read and returns an open handle on the file, which is nil when the
// file is deleted at the read version. Besides the fetched file the post-hooks receive the version
// of the file's latest version (see FilePostHooks), and they always run, even when file is nil or
// the read errored, so they can observe and transform the error.
func (b *B) withFileHooks(
	ctx context.Context, id identifier.Identifier, version *store.Version,
	fetch func() (io.ReadSeekCloser, *storage.FileMetadata, store.Version, []store.Version, errors.E),
) (io.ReadSeekCloser, *storage.FileMetadata, store.Version, []store.Version, errors.E) {
	for _, hook := range b.FilePreHooks {
		errE := hook(ctx, id, version)
		if errE != nil {
			return nil, nil, store.Version{}, nil, errE
		}
	}
	var file io.ReadSeekCloser
	var latestVersion *store.Version
	var metadata *storage.FileMetadata
	var resolved store.Version
	var parentChangesets []store.Version
	var errE errors.E
	if version == nil || len(b.FilePostHooks) == 0 {
		file, metadata, resolved, parentChangesets, errE = fetch()
		if errE == nil && len(b.FilePostHooks) > 0 {
			// The fetched version is the latest version. A copy is used, so hooks which transform the
			// returned version do not affect the latest version later hooks receive.
			lv := resolved
			latestVersion = &lv
		}
	} else {
		file, latestVersion, metadata, resolved, parentChangesets, errE = b.readVersionedFileWithLatest(ctx, id, *version, fetch)
	}
	for _, hook := range b.FilePostHooks {
		file, metadata, resolved, parentChangesets, errE = hook(ctx, file, latestVersion, metadata, resolved, parentChangesets, errE)
	}
	return file, metadata, resolved, parentChangesets, errE
}

func (b *B) withHooks(
	ctx context.Context, id identifier.Identifier, version *store.Version,
	fetch func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E),
) (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	if len(b.DocumentPostHooks) == 0 {
		for _, hook := range b.DocumentPreHooks {
			errE := hook(ctx, id, version)
			if errE != nil {
				return nil, nil, store.Version{}, nil, errE
			}
		}
		return fetch()
	}
	doc, metadata, resolved, parentChangesets, errE := b.withDocumentHooks(ctx, id, version, fetch)
	if doc == nil {
		return nil, metadata, resolved, parentChangesets, errE
	}
	data, errE2 := x.MarshalWithoutEscapeHTML(doc)
	if errE2 != nil {
		return nil, metadata, resolved, parentChangesets, errors.Join(errE, errE2)
	}
	return data, metadata, resolved, parentChangesets, errE
}
