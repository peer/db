package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

// TODO: We build query strings again and again based on patchesEnabled. We should create them once during Init and reuse them here.

// Changeset is a batch of changes done to objects.
// It can be prepared and later on committed to a view or discarded.
type Changeset[Data, Metadata, Patch any] struct {
	identifier.Identifier

	view *View[Data, Metadata, Patch]
}

func (c *Changeset[Data, Metadata, Patch]) View() *View[Data, Metadata, Patch] {
	return c.view
}

// We allow changing changesets even after they have been used as a parent changeset in some
// other changeset to allow one to prepare a chain changesets to commit. It is up to the higher
// levels to assure changesets and their patches are consistent before committing the chain.
// We check just that the chain has a reasonable series of changesets and that parent changesets
// are committed before children.

func (c *Changeset[Data, Metadata, Patch]) Insert(ctx context.Context, id identifier.Identifier, value Data, metadata Metadata) (Version, errors.E) {
	arguments := []any{
		id.String(), c.String(), value, metadata,
	}
	patches := ""
	if c.view.store.patchesEnabled {
		patches = ", '{}'" //nolint:goconst
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		//nolint:goconst
		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, '{}', '{}', $3, $4`+patches+`
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`, arguments...)
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
		details["view"] = c.view.name
		details["id"] = id.String()
		details["changeset"] = c.String()
	}
	return version, errE
}

func (c *Changeset[Data, Metadata, Patch]) Update(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, patch Patch, metadata Metadata,
) (Version, errors.E) {
	arguments := []any{
		id.String(), c.String(), []string{parentChangeset.String()}, value, metadata,
	}
	patchesPlaceholders := ""
	if c.view.store.patchesEnabled {
		arguments = append(arguments, []Patch{patch})
		patchesPlaceholders = ", $6"
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// TODO: Make sure parent changesets really contain object ID.
		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, $3, '{}', $4, $5`+patchesPlaceholders+`
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`, arguments...)
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
		details["view"] = c.view.name
		details["id"] = id.String()
		details["changeset"] = c.String()
		details["parentChangeset"] = parentChangeset.String()
	}
	return version, errE
}

func (c *Changeset[Data, Metadata, Patch]) Replace(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, metadata Metadata,
) (Version, errors.E) {
	arguments := []any{
		id.String(), c.String(), []string{parentChangeset.String()}, value, metadata,
	}
	patches := ""
	if c.view.store.patchesEnabled {
		patches = ", '{}'"
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// TODO: Make sure parent changesets really contain object ID.

		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, $3, '{}', $4, $5`+patches+`
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`, arguments...)
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
		details["view"] = c.view.name
		details["id"] = id.String()
		details["changeset"] = c.String()
		details["parentChangeset"] = parentChangeset.String()
	}
	return version, errE
}

func (c *Changeset[Data, Metadata, Patch]) Delete(ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata) (Version, errors.E) {
	arguments := []any{
		id.String(), c.String(), []string{parentChangeset.String()}, metadata,
	}
	patches := ""
	if c.view.store.patchesEnabled {
		patches = ", '{}'"
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// TODO: Make sure parent changesets really contain object ID.

		res, err := tx.Exec(ctx, `
			INSERT INTO "changes" SELECT $1, $2, 1, $3, '{}', NULL, $4`+patches+`
				-- The changeset should not yet be committed (to any view).
				WHERE NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$2)
				ON CONFLICT DO NOTHING
		`, arguments...)
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
		details["view"] = c.view.name
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
	arguments := []any{
		c.String(), metadata, c.view.name,
	}
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		res, err := tx.Exec(ctx, `
			WITH "currentViewPath" AS (
				SELECT p.* FROM "currentViews", "views", UNNEST("path") AS p("id")
					WHERE "currentViews"."name"=$3
					AND "currentViews"."id"="views"."id"
					AND "currentViews"."revision"="views"."revision"
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
		`, arguments...)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			// TODO: Is there a better way to differentiate between different EXISTS conditions instead of doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "currentViews" WHERE "name"=$1)`, c.view.name).Scan(&exists)
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
					SELECT p.* FROM "currentViews", "views", UNNEST("path") AS p("id")
						WHERE "currentViews"."name"=$1
						AND "currentViews"."id"="views"."id"
						AND "currentViews"."revision"="views"."revision"
				), "currentViewChangesets" AS (
					SELECT "changeset" FROM "viewChangesets", "currentViewPath" WHERE "viewChangesets"."id"="currentViewPath"."id"
				), "parentChangesets" AS (
					SELECT UNNEST("parentChangesets") AS "changeset" FROM "currentChanges" WHERE "changeset"=$1
				)
				SELECT EXISTS (SELECT * FROM "parentChangesets" EXCEPT SELECT * FROM "currentViewChangesets")
			`,
				c.view.name,
			).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			}
			if exists {
				return errors.WithStack(ErrParentNotCommitted)
			}
			// This should not happen.
			return errors.New("database insert unexpectedly inserted no rows")
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = c.view.name
		details["changeset"] = c.String()
	} else if c.view.store.Committed != nil {
		// We send over a non-initialized Changeset, requiring the receiver to reconstruct it.
		c.view.store.Committed <- Changeset[Data, Metadata, Patch]{
			Identifier: c.Identifier,
			view: &View[Data, Metadata, Patch]{
				name: c.view.name,
				store: &Store[Data, Metadata, Patch]{
					Schema:         c.view.store.Schema,
					Committed:      nil,
					DataType:       "",
					MetadataType:   "",
					PatchType:      "",
					dbpool:         nil,
					patchesEnabled: false,
				},
			},
		}
	}
	return errE
}

// We allow discarding changesets even after they have been used as a parent changeset in some
// other changeset to allow one to prepare a chain changesets to commit. It is up to the higher
// levels to assure changesets and their patches are consistent before committing the chain.
// Discarding should anyway not be used on user-facing changesets.

// Discard deletes the changelog if it has not already been committed.
func (c *Changeset[Data, Metadata, Patch]) Discard(ctx context.Context) errors.E {
	arguments := []any{
		c.String(),
	}
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		res, err := tx.Exec(ctx, `
			DELETE FROM "changes"
				WHERE "changeset"=$1
				-- The changeset should not yet be committed (to any view).
				AND NOT EXISTS (SELECT 1 FROM "viewChangesets" WHERE "changeset"=$1)
		`, arguments...)
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
		details["view"] = c.view.name
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

type Change[Data, Metadata, Patch any] struct {
	ID       identifier.Identifier
	Version  Version
	Data     Data
	Metadata Metadata
	Patches  []Patch
}

func (c *Changeset[Data, Metadata, Patch]) Changes(ctx context.Context) ([]Change[Data, Metadata, Patch], errors.E) {
	arguments := []any{
		c.view.name, c.String(),
	}
	patches := ", NULL"
	if c.view.store.patchesEnabled {
		patches = `, "patches"`
	}
	var changes []Change[Data, Metadata, Patch]
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		changes = nil

		rows, err := tx.Query(ctx, `
			WITH "currentViewPath" AS (
				-- We do not care about order of views here because we have en explicit version we are searching for.
				SELECT p.* FROM "currentViews", "views", UNNEST("path") AS p("id")
					WHERE "currentViews"."name"=$1
					AND "currentViews"."id"="views"."id"
					AND "currentViews"."revision"="views"."revision"
			), "currentViewChangesets" AS (
				SELECT "changeset" FROM "viewChangesets", "currentViewPath" WHERE "viewChangesets"."id"="currentViewPath"."id"
			)
			SELECT "id", "revision", "data", "metadata"`+patches+` FROM "currentChanges", "currentViewChangesets"
				WHERE "currentChanges"."changeset"=$2 AND "currentChanges"."changeset"="currentViewChangesets"."changeset"
		`, arguments...)
		if err != nil {
			return errors.WithStack(err)
		}
		var id string
		var revision int64
		var data Data
		var metadata Metadata
		var patches []Patch
		_, err = pgx.ForEachRow(rows, []any{&id, &revision, &data, &metadata, &patches}, func() error {
			changes = append(changes, Change[Data, Metadata, Patch]{
				ID: identifier.MustFromString(id),
				Version: Version{
					Changeset: c.Identifier,
					Revision:  revision,
				},
				Data:     data,
				Metadata: metadata,
				Patches:  patches,
			})
			return nil
		})
		return errors.WithStack(err)
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = c.view.name
		details["changeset"] = c.String()
	}
	return changes, errE
}

func (c *Changeset[Data, Metadata, Patch]) WithStore(ctx context.Context, store *Store[Data, Metadata, Patch]) (Changeset[Data, Metadata, Patch], errors.E) {
	view, errE := store.View(ctx, c.View().Name())
	if errE != nil {
		return Changeset[Data, Metadata, Patch]{}, errE //nolint:exhaustruct
	}
	return view.Changeset(ctx, c.Identifier)
}
