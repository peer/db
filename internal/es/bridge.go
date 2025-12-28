// Package es provides Elasticsearch integration functionality for PeerDB.
package es

import (
	"context"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"

	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/store"
)

// TODO: Address the issue of what happens if bridge fails before ES indexed the document.
//       It might happen immediately after the changeset is committed, or even while the index request
//       is waiting in the processor. We should store somewhere the changelog for each view until
//       where they were indexed and continue on (new) bridge start from we left the last time.
//       At the same time make it work when peerdb process is horizontally scaled.

// Bridge synchronizes changes from the store to Elasticsearch by listening to committed changesets.
func Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any](
	ctx context.Context, logger zerolog.Logger, s *store.Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch],
	esProcessor *elastic.BulkProcessor, index string,
	committedChangesets <-chan store.CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch],
) {
	for {
		select {
		case <-ctx.Done():
			return
		case c, ok := <-committedChangesets:
			if !ok {
				return
			}

			// The order in which changesets are send to the channel is not necessary
			// the order in which they were committed. We should not relay on the order.

			// We have to reconstruct the committedChangeset and the view using our store.
			committedChangeset, errE := c.WithStore(ctx, s)
			if errE != nil {
				logger.Error().Err(errE).Str("changeset", c.Changeset.String()).Str("view", c.View.Name()).Msg("bridge error: with store")
				continue
			}

			var after *identifier.Identifier
			changes := []store.Change{}
			for {
				page, errE := committedChangeset.Changeset.Changes(ctx, after)
				if errE != nil {
					logger.Error().Err(errE).Str("changeset", c.Changeset.String()).Str("view", c.View.Name()).Msg("bridge error: changes")
					break
				}
				changes = append(changes, page...)
				if len(page) < store.MaxPageLength {
					break
				}
				after = &page[4999].ID
			}

			for _, change := range changes {
				// Because changesets are not necessary in order, we always get the latest version and index it.
				data, _, _, errE := s.GetLatest(ctx, change.ID)
				if errE != nil {
					logger.Error().Err(errE).Str("changeset", c.Changeset.String()).Str("view", c.View.Name()).Msg("bridge error: get current")
					continue
				}

				// TODO: Convert data into searchable document for the general case.
				// TODO: Use also information about the view so that documents are searchable by view as well.
				req := elastic.NewBulkIndexRequest().Index(index).Id(change.ID.String()).Doc(data)
				esProcessor.Add(req)
			}
		}
	}
}
