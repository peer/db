package es

import (
	"context"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"

	"gitlab.com/peerdb/peerdb/store"
)

// TODO: Address the issue of what happens if bridge fails before ES indexed the document.
//       It might happen immediately after the changeset is committed, or even while the index request
//       is waiting in the processor. We should store somewhere the changelog for each view until
//       where they were indexed and continue on (new) bridge start from we left the last time.
//       At the same time make it work when peerdb process is horizontally scaled.

func Bridge[Data, Metadata, Patch any](
	ctx context.Context, logger zerolog.Logger, store *store.Store[Data, Metadata, Patch],
	processor *elastic.BulkProcessor, index string, changesets <-chan store.Changeset[Data, Metadata, Patch],
) {
	for {
		select {
		case <-ctx.Done():
			return
		case c, ok := <-changesets:
			if !ok {
				return
			}

			// The order in which changesets are send to the channel is not necessary
			// the order in which they were committed. We should not relay on the order.

			// We have to reconstruct the changeset using our store.
			changeset, errE := c.WithStore(ctx, store)
			if errE != nil {
				logger.Error().Err(errE).Str("changeset", c.String()).Msg("bridge error: with store")
				continue
			}

			changes, errE := changeset.Changes(ctx)
			if errE != nil {
				logger.Error().Err(errE).Str("changeset", c.String()).Msg("bridge error: changes")
				continue
			}

			for _, change := range changes {
				// Because changesets are not necessary in order, we always get the latest version and index it.
				data, _, _, errE := store.GetLatest(ctx, change.ID)
				if errE != nil {
					logger.Error().Err(errE).Str("changeset", c.String()).Msg("bridge error: get current")
					continue
				}

				// TODO: Convert data into searchable document for the general case.
				req := elastic.NewBulkIndexRequest().Index(index).Id(change.ID.String()).Doc(data)
				processor.Add(req)
			}
		}
	}
}
