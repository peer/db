package base

import (
	"context"
	"encoding/json"
	"slices"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/store"
)

// MaterializedStateDrift reports, per materialized state table, how the table's current contents differ
// from a rebuild from the stored documents: Extra counts the rows present which the rebuild does not
// produce, and Missing counts the rows the rebuild produces which are not present.
type MaterializedStateDrift = internalSearch.MaterializedStateDrift

// RepairDocumentsMetadata goes over every stored revision of every document and recomputes the extracted
// content fields of its metadata (currently store.DocumentMetadata.InstanceOf) from the revision's
// content, updating changed metadata in place: at the same revision, without any new version, commit, or
// notification, because it is a repair of stored data and not a new version of it. Provenance fields of
// the metadata (At, Users) are never touched. With dryRun nothing is written, only reported.
//
// It returns the number of revisions examined and the number which needed repair. When count and size are
// non-nil they track progress: size is increased once by the total number of revisions and count is
// increased as revisions are examined, ending exactly at that total.
func (b *B) RepairDocumentsMetadata(ctx context.Context, dryRun bool, count, size *x.Counter) (int64, int64, errors.E) {
	return b.Documents().RepairMetadata(ctx, dryRun, func(
		_ identifier.Identifier, data json.RawMessage, deleted bool, metadata *store.DocumentMetadata,
	) (*store.DocumentMetadata, bool, errors.E) {
		changed, errE := repairDocumentMetadata(data, deleted, metadata)
		return metadata, changed, errE
	}, count, size)
}

// repairDocumentMetadata recomputes the extracted content fields of the document metadata from the
// document content, in place, and reports whether anything changed. It is the single place which knows
// which metadata fields are extracted content (each a pure function of the content alone, so future
// extracted fields are added here) as opposed to provenance (At, Users), which it never touches. A
// deleted revision has no content, so its extracted content is empty.
func repairDocumentMetadata(data json.RawMessage, deleted bool, metadata *store.DocumentMetadata) (bool, errors.E) {
	var instanceOf []identifier.Identifier
	if !deleted {
		doc := new(document.D)
		errE := x.UnmarshalWithoutUnknownFields(data, doc)
		if errE != nil {
			return false, errE
		}
		instanceOf = doc.InstanceOf()
	}

	changed := false
	if !slices.Equal(metadata.InstanceOf, instanceOf) {
		metadata.InstanceOf = instanceOf
		changed = true
	}
	return changed, nil
}

// RepairMaterializedState derives the references, inverse-relations, and embedding tables anew, from the
// latest versions of the stored documents as the single source of truth, using converters built from the
// given schema documents the same way Start builds them, and replaces the tables' contents with the
// result when they differ. With dryRun nothing is replaced, only the drift is reported; the drift is
// reported in both modes.
//
// It must run after Init and instead of Start, in a process which never starts the base: nothing else may
// write the tables or render documents from them while they are rebuilt, and it is never safe to run
// concurrently with a live process using the same store.
//
// When count and size are non-nil they track progress: size is increased once by the document total and
// count is increased as documents are processed, ending exactly at that total.
func (b *B) RepairMaterializedState(
	ctx context.Context, documents []StartDocument, dryRun bool, count, size *x.Counter,
) (MaterializedStateDrift, errors.E) {
	targets, errE := b.buildTargets(ctx, documents)
	if errE != nil {
		return MaterializedStateDrift{}, errE
	}
	b.bridge.Prepare(targets)
	return b.bridge.RebuildMaterializedState(ctx, dryRun, count, size)
}
