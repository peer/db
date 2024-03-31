package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

// Changeset is a batch of changes done to objects.
// It can be prepared and later on committed to a view or discarded.
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
		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, '{}', '{}', $3, $4, '{}'
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`,
			id.String(), c.String(), value, metadata,
		)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			// TODO: Is there a better way to differentiate between WHERE NOT EXISTS and ON CONFLICT DO NOTHING instead of doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)`, c.String()).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			}
			if exists {
				return errors.WithStack(ErrAlreadyCommitted)
			}
			return errors.WithStack(ErrConflict)
		}
		version.Changeset = c.Identifier
		version.Revision = 1
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = c.view.Name
		details["id"] = id.String()
		details["changeset"] = c.String()
	}
	return version, errE
}

func (c *Changeset[Data, Metadata, Patch]) Update(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, patch Patch, metadata Metadata,
) (Version, errors.E) {
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// TODO: Make sure parent changesets really contain object ID.
		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, $3, '{}', $4, $5, $6
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`,
			id.String(), c.String(), []string{parentChangeset.String()}, value, metadata, []Patch{patch},
		)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			// TODO: Is there a better way to differentiate between WHERE NOT EXISTS and ON CONFLICT DO NOTHING instead of doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)`, c.String()).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			}
			if exists {
				return errors.WithStack(ErrAlreadyCommitted)
			}
			return errors.WithStack(ErrConflict)
		}
		version.Changeset = c.Identifier
		version.Revision = 1
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = c.view.Name
		details["id"] = id.String()
		details["changeset"] = c.String()
		details["parentChangeset"] = parentChangeset.String()
	}
	return version, errE
}

func (c *Changeset[Data, Metadata, Patch]) Replace(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, metadata Metadata,
) (Version, errors.E) {
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// TODO: Make sure parent changesets really contain object ID.
		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, $3, '{}', $4, $5, '{}'
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`,
			id.String(), c.String(), []string{parentChangeset.String()}, value, metadata,
		)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			// TODO: Is there a better way to differentiate between WHERE NOT EXISTS and ON CONFLICT DO NOTHING instead of doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)`, c.String()).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			}
			if exists {
				return errors.WithStack(ErrAlreadyCommitted)
			}
			return errors.WithStack(ErrConflict)
		}
		version.Changeset = c.Identifier
		version.Revision = 1
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = c.view.Name
		details["id"] = id.String()
		details["changeset"] = c.String()
		details["parentChangeset"] = parentChangeset.String()
	}
	return version, errE
}

func (c *Changeset[Data, Metadata, Patch]) Delete(ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata) (Version, errors.E) {
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// TODO: Make sure parent changesets really contain object ID.
		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, $3, '{}', NULL, $4, '{}'
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`,
			id.String(), c.String(), []string{parentChangeset.String()}, metadata,
		)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			// TODO: Is there a better way to differentiate between WHERE NOT EXISTS and ON CONFLICT DO NOTHING instead of doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)`, c.String()).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			}
			if exists {
				return errors.WithStack(ErrAlreadyCommitted)
			}
			return errors.WithStack(ErrConflict)
		}
		version.Changeset = c.Identifier
		version.Revision = 1
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = c.view.Name
		details["id"] = id.String()
		details["changeset"] = c.String()
		details["parentChangeset"] = parentChangeset.String()
	}
	return version, errE
}

// TODO: How to make sure is committing/discarding the version of changeset they expect?
//       There is a race condition between decision to commit/discard and until it is done.

// Commit adds the changelog to the view.
func (c *Changeset[Data, Metadata, Patch]) Commit(ctx context.Context, metadata Metadata) errors.E {
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		res, err := tx.Exec(ctx, `
			WITH "currentViewPath" AS (
				SELECT p.* FROM "currentViews", UNNEST("path") AS p("id") WHERE "name"=$3
			), "currentViewChangesets" AS (
				SELECT "changeset" FROM "viewChangesets", "currentViewPath" WHERE "viewChangesets"."id"="currentViewPath"."id"
			), "parentChangesets" AS (
				SELECT UNNEST("parentChangesets") AS "changeset" FROM "currentChanges" WHERE "changeset"=$1
			)
			INSERT INTO "viewChangesets" SELECT "id", $1, $2 FROM "currentViews"
				WHERE	"name"=$3
				-- The changeset should not yet be committed (to any view).
				AND NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)
				-- There must be at least one change in the changeset we want to commit.
				AND EXISTS (SELECT 1 FROM "currentChanges" WHERE "changeset"=$1)
				-- That parent changesets really contain object IDs is checked in Update and Delete.
				-- Here we only check that parent changesets are already committed for the current view.
				AND NOT EXISTS (SELECT * FROM "parentChangesets" EXCEPT SELECT * FROM "currentViewChangesets")
		`,
			c.String(), metadata, c.view.Name,
		)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			// TODO: Is there a better way to differentiate between different EXISTS conditions instead of doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "currentViews" WHERE "name"=$1)`, c.view.Name).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			}
			if !exists {
				return errors.WithStack(ErrViewNotFound)
			}
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)`, c.String()).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			}
			if exists {
				return errors.WithStack(ErrAlreadyCommitted)
			}
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "currentChanges" WHERE "changeset"=$1)`, c.String()).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			}
			if !exists {
				// Committing an empty (or an already discarded) changeset is not an error.
				return nil
			}
			err = tx.QueryRow(ctx, `
				WITH "currentViewPath" AS (
					SELECT p.* FROM "currentViews", UNNEST("path") AS p("id") WHERE "name"=$1
				), "currentViewChangesets" AS (
					SELECT "changeset" FROM "viewChangesets", "currentViewPath" WHERE "viewChangesets"."id"="currentViewPath"."id"
				), "parentChangesets" AS (
					SELECT UNNEST("parentChangesets") AS "changeset" FROM "currentChanges" WHERE "changeset"=$1
				)
				SELECT EXISTS (SELECT * FROM "parentChangesets" EXCEPT SELECT * FROM "currentViewChangesets")
			`,
				c.view.Name,
			).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			}
			if exists {
				return errors.WithStack(ErrParentNotCommitted)
			}
			// This should not happen.
			return errors.New("insert unexpectedly inserted no errors")
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = c.view.Name
		details["changeset"] = c.String()
	}
	return errE
}

// We allow discarding changesets even after they have been used as a parent changeset in some
// other changeset to allow one to prepare a chain changesets to commit. It is up to the higher
// levels to assure changesets and their patches are consistent before committing the chain.
// Discarding should anyway not be used on user-facing changesets.

// Discard deletes the changelog if it has not already been committed.
func (c *Changeset[Data, Metadata, Patch]) Discard(ctx context.Context) errors.E {
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		res, err := tx.Exec(ctx, `
			DELETE FROM "changes"
				WHERE "changeset"=$1
				-- The changeset should not yet be committed (to any view).
				AND NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)
		`,
			c.String(),
		)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			// TODO: Is there a better way to differentiate between WHERE NOT EXISTS and an empty changeset instead of doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)`, c.String()).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			}
			if exists {
				return errors.WithStack(ErrAlreadyCommitted)
			}
			// Discarding an empty (or an already discarded) changeset is not an error.
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = c.view.Name
		details["changeset"] = c.String()
	}
	return errE
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