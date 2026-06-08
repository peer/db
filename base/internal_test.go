package base

import (
	"context"
	"encoding/json"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/store"
)

func (b *B) TestingWithDocumentHooks(
	ctx context.Context, id identifier.Identifier, version *store.Version,
	fetch func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E),
) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	return b.withDocumentHooks(ctx, id, version, fetch)
}
