package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

type Changeset[Data, Metadata, Patch any] struct {
	identifier.Identifier

	view *View[Data, Metadata, Patch]
}

// We allow changing changesets even after they have been used as a parent changeset in some
// other changeset to allow one to prepare a chain changesets to commit. It is up to the higher
// levels to assure changesets and their patches are consistent before committing the chain.
// We check just that the chain has a reasonable series of changesets and that parent changesets
// are committed before children.

func (c *Changeset[Data, Metadata, Patch]) Insert(ctx context.Context, id identifier.Identifier, value Data, metadata Metadata) (Version, errors.E) {
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// TODO: How to differentiate and return ErrAlreadyCommitted if row was not added because changeset is already in viewChangesets?
		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, '{}', '{}', $3, $4, '{}'
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXIST (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`,
			id.String(), c.String(), value, metadata,
		)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			return errors.WithStack(ErrConflict)
		}
		version.Changeset = c.Identifier
		version.Revision = 1
		return nil
	})
	return version, errE
}

func (c *Changeset[Data, Metadata, Patch]) Update(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, patch Patch, metadata Metadata,
) (Version, errors.E) {
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// TODO: How to differentiate and return ErrAlreadyCommitted if row was not added because changeset is already in viewChangesets?
		// TODO: Make sure parent changesets really contain object ID.
		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, $3, '{}', $4, $5, $6
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXIST (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`,
			id.String(), c.String(), []string{parentChangeset.String()}, value, metadata, []Patch{patch},
		)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			return errors.WithStack(ErrConflict)
		}
		version.Changeset = c.Identifier
		version.Revision = 1
		return nil
	})
	return version, errE
}

func (c *Changeset[Data, Metadata, Patch]) Delete(ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata) (Version, errors.E) {
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// TODO: How to differentiate and return ErrAlreadyCommitted if row was not added because changeset is already in viewChangesets?
		// TODO: Make sure parent changesets really contain object ID.
		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, $3, '{}', NULL, $4, '{}'
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXIST (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`,
			id.String(), c.String(), []string{parentChangeset.String()}, metadata,
		)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			return errors.WithStack(ErrConflict)
		}
		version.Changeset = c.Identifier
		version.Revision = 1
		return nil
	})
	return version, errE
}

// TODO: How to make sure is committing/discarding the version of changeset they expect?
//       There is a race condition between decision to commit/discard and until it is done.

// Commit adds the changelog to the view.
func (c *Changeset[Data, Metadata, Patch]) Commit(ctx context.Context, metadata Metadata) errors.E {
	return internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `
			WITH "currentViewPath" AS (
				SELECT p.* FROM "currentViews", UNNEST("path") AS p("id") WHERE "name"=$1
			), "currentViewChangesets" AS (
				SELECT "changeset" FROM "viewChangesets", "currentViewPath" WHERE "viewChangesets"."id"="currentViewPath"."id"
			), "parentChangesets" AS (
				SELECT UNNEST("parentChangesets") AS "changeset" FROM "currentChanges" WHERE "changeset"=$1
			)
			INSERT INTO "viewChangesets" SELECT "id", $1, $2 FROM "currentViews"
				WHERE	"name"=$3
				-- The changeset should not yet be committed (to any view).
				AND NOT EXIST (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)
				-- There must be at least one change in the changeset we want to commit.
				AND EXIST (SELECT 1 FROM "currentChanges" WHERE "changeset"=$1)
				-- That parent changesets really contain object IDs is checked in Update and Delete.
				-- Here we only check that parent changesets are already committed for the current view.
				AND NOT EXIST (SELECT * FROM "parentChangesets" EXCEPT SELECT * FROM "currentViewChangesets")
		`,
			c.String(), metadata, c.view.Name,
		)
		return internal.WithPgxError(err)
	})
}

// We allow discarding changesets even after they have been used as a parent changeset in some
// other changeset to allow one to prepare a chain changesets to commit. It is up to the higher
// levels to assure changesets and their patches are consistent before committing the chain.
// Discarding should anyway not be used on user-facing changesets.

// Discard deletes the changelog if it has not already been committed.
func (c *Changeset[Data, Metadata, Patch]) Discard(ctx context.Context) errors.E {
	return internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// TODO: Return ErrAlreadyCommitted if already committed.
		_, err := tx.Exec(ctx, `
			DELETE FROM "changes"
				WHERE "changeset"=$1
				-- The changeset should not yet be committed (to any view).
				AND NOT EXIST (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)
		`,
			c.String(),
		)
		return internal.WithPgxError(err)
	})
}

// Rollback discards the changelog but only if it has not already been committed.
func (c *Changeset[Data, Metadata, Patch]) Rollback(ctx context.Context) errors.E {
	errE := c.Discard(ctx)
	if errE != nil && errors.Is(errE, ErrAlreadyCommitted) {
		return nil
	}
	return errE
}

// TODO: Should we provide also "archive" for archiving user-facing changesets (instead of discard).
//       When they will not be used anymore, but we should keep them around (and we want to
//       prevent accidentally changing them.)
