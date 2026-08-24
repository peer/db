package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// RepairMetadata goes over every stored change row (every revision of every value, in every changeset,
// including deleted ones) and calls repair with the row's value ID, data (with deleted true when the row
// records a deletion, its data is then the zero value), and metadata. When repair reports the metadata
// changed, the row's metadata is replaced with the returned one, in place: at the same revision, without
// any new version, commit, or notification, because this is a repair of stored data and not a new version
// of it. It is on repair to derive metadata from the row's data alone and to preserve everything in the
// metadata it does not derive. With dryRun nothing is written, only reported.
//
// Change rows are immutable in normal operation (a new version is always a new row), so the row read and
// its later in-place update do not race anything, but nothing may be reading the metadata being repaired
// concurrently. The metadata is round-tripped through the Metadata type, so the type must model the
// stored metadata fully: with a struct Metadata the JSON unmarshal rejects unknown fields, so a stored field
// the type does not carry fails the read.
//
// It returns the number of rows examined and the number of rows which needed repair. When count and size
// are non-nil they track progress: size is increased once by the total number of rows and count is
// increased as rows are examined, ending exactly at that total.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) RepairMetadata(
	ctx context.Context, dryRun bool,
	repair func(id identifier.Identifier, data Data, deleted bool, metadata Metadata) (Metadata, bool, errors.E),
	count, size *x.Counter,
) (int64, int64, errors.E) {
	var total int64
	errE := internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		total = 0

		err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM "`+s.Prefix+`Changes"`).Scan(&total)
		return internalStore.WithPgxError(err)
	})
	if errE != nil {
		return 0, 0, errE
	}
	if size != nil {
		size.Add(total)
	}

	// A change row of one page, together with the repaired metadata to write back when Changed is set.
	type changeRow struct {
		Changeset string
		ID        string
		Revision  int64
		Metadata  Metadata
		Changed   bool
	}

	var examined, repaired int64
	// Keyset pagination cursor over the primary key of the Changes table.
	afterChangeset := ""
	afterID := ""
	afterRevision := int64(0)
	for {
		var page []changeRow
		errE := internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
			// Initialize in the case transaction is retried.
			page = page[:0]

			rows, err := tx.Query(ctx, `
				SELECT "changeset", "id", "revision", "data", "data" IS NULL, "metadata"
					FROM "`+s.Prefix+`Changes"
					WHERE ("changeset", "id", "revision") > ($1, $2, $3)
					ORDER BY "changeset", "id", "revision"
					LIMIT `+maxPageLengthStr,
				afterChangeset, afterID, afterRevision,
			)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
			var changeset, id string
			var revision int64
			var data Data
			var dataIsNull bool
			var metadata Metadata
			_, err = pgx.ForEachRow(rows, []any{&changeset, &id, &revision, &data, &dataIsNull, &metadata}, func() error {
				rowData := data
				if dataIsNull {
					// What a NULL scans into depends on the data type, so a deletion's data is made the zero
					// value explicitly.
					var zero Data
					rowData = zero
				}
				repairedMetadata, changed, errE := repair(identifier.String(id), rowData, dataIsNull, metadata)
				if errE != nil {
					details := errors.Details(errE)
					details["changeset"] = changeset
					details["id"] = id
					details["revision"] = revision
					return errE
				}
				page = append(page, changeRow{Changeset: changeset, ID: id, Revision: revision, Metadata: repairedMetadata, Changed: changed})
				return nil
			})
			return internalStore.WithPgxError(err)
		})
		if errE != nil {
			return examined, repaired, errE
		}
		if len(page) == 0 {
			break
		}

		var changedRows []changeRow
		for _, row := range page {
			if row.Changed {
				changedRows = append(changedRows, row)
			}
		}
		if !dryRun && len(changedRows) > 0 {
			errE := internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
				// Change rows are immutable in normal operation and the store enforces that with a trigger.
				// The repair operates at the database level, beneath that rule, so it disables the trigger
				// and re-enables it before committing. The disabled state is never observable outside this
				// transaction: the ALTER TABLE holds its exclusive lock on the table until the transaction
				// ends, and it either commits with the trigger re-enabled or rolls back the disable itself.
				_, err := tx.Exec(ctx, `ALTER TABLE "`+s.Prefix+`Changes" DISABLE TRIGGER "`+s.Prefix+`ChangesNotAllowed"`)
				if err != nil {
					return internalStore.WithPgxError(err)
				}
				for _, row := range changedRows {
					_, err := tx.Exec(ctx, `
						UPDATE "`+s.Prefix+`Changes" SET "metadata"=$4
							WHERE "changeset"=$1 AND "id"=$2 AND "revision"=$3
					`, row.Changeset, row.ID, row.Revision, row.Metadata)
					if err != nil {
						errE := internalStore.WithPgxError(err)
						details := errors.Details(errE)
						details["changeset"] = row.Changeset
						details["id"] = row.ID
						details["revision"] = row.Revision
						return errE
					}
				}
				_, err = tx.Exec(ctx, `ALTER TABLE "`+s.Prefix+`Changes" ENABLE TRIGGER "`+s.Prefix+`ChangesNotAllowed"`)
				return internalStore.WithPgxError(err)
			})
			if errE != nil {
				return examined, repaired, errE
			}
		}

		examined += int64(len(page))
		repaired += int64(len(changedRows))
		if count != nil {
			count.Add(int64(len(page)))
		}
		if len(page) < MaxPageLength {
			break
		}
		last := page[len(page)-1]
		afterChangeset = last.Changeset
		afterID = last.ID
		afterRevision = last.Revision
	}

	// Reconcile so count was increased by exactly the total reported upfront (matching the size increase),
	// even if the number of rows differed due to concurrent changes between the count and the iteration.
	if count != nil {
		count.Add(total - examined)
	}

	return examined, repaired, nil
}
