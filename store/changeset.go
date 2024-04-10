package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
// other changeset to allow one to prepare a chain of changesets to commit. It is up to the higher
// levels to assure changesets and their patches are consistent before committing the chain.
// We check just that the chain has a reasonable series of changesets and that parent changesets
// are committed before children.

func (c *Changeset[Data, Metadata, Patch]) Insert(ctx context.Context, id identifier.Identifier, value Data, metadata Metadata) (Version, errors.E) {
	arguments := []any{
		c.String(), id.String(), value, metadata,
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `SELECT "changesetInsert"($1, $2, $3, $4)`, arguments...)
		if err != nil {
			errE := internal.WithPgxError(err)
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) {
				switch pgError.Code {
				case errorCodeAlreadyCommitted:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case internal.ErrorCodeUniqueViolation:
					return errors.WrapWith(errE, ErrConflict)
				}
			}
			return errE
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
		c.String(), id.String(), []string{parentChangeset.String()}, value, metadata,
	}
	patchesPlaceholders := ""
	if c.view.store.patchesEnabled {
		arguments = append(arguments, []Patch{patch})
		patchesPlaceholders = ", $6"
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `SELECT "changesetUpdate"($1, $2, $3, $4, $5`+patchesPlaceholders+`)`, arguments...)
		if err != nil {
			errE := internal.WithPgxError(err)
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) {
				switch pgError.Code {
				case errorCodeAlreadyCommitted:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case errorCodeParentInvalid:
					return errors.WrapWith(errE, ErrParentInvalid)
				case internal.ErrorCodeUniqueViolation:
					return errors.WrapWith(errE, ErrConflict)
				}
			}
			return errE
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
		c.String(), id.String(), []string{parentChangeset.String()}, value, metadata,
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `SELECT "changesetReplace"($1, $2, $3, $4, $5)`, arguments...)
		if err != nil {
			errE := internal.WithPgxError(err)
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) {
				switch pgError.Code {
				case errorCodeAlreadyCommitted:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case errorCodeParentInvalid:
					return errors.WrapWith(errE, ErrParentInvalid)
				case internal.ErrorCodeUniqueViolation:
					return errors.WrapWith(errE, ErrConflict)
				}
			}
			return errE
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
		c.String(), id.String(), []string{parentChangeset.String()}, metadata,
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `SELECT "changesetDelete"($1, $2, $3, $4)`, arguments...)
		if err != nil {
			errE := internal.WithPgxError(err)
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) {
				switch pgError.Code {
				case errorCodeAlreadyCommitted:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case errorCodeParentInvalid:
					return errors.WrapWith(errE, ErrParentInvalid)
				case internal.ErrorCodeUniqueViolation:
					return errors.WrapWith(errE, ErrConflict)
				}
			}
			return errE
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

// Commit adds the changeset to the view.
//
// It commits any non-committed ancestor changesets as well.
func (c *Changeset[Data, Metadata, Patch]) Commit(ctx context.Context, metadata Metadata) ([]Changeset[Data, Metadata, Patch], errors.E) {
	arguments := []any{
		c.String(), metadata, c.view.name,
	}
	var committedChangesets []string
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		err := tx.QueryRow(ctx, `SELECT "changesetCommit"($1, $2, $3)`, arguments...).Scan(&committedChangesets)
		if err != nil {
			errE := internal.WithPgxError(err)
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) {
				switch pgError.Code {
				case errorCodeViewNotFound:
					return errors.WrapWith(errE, ErrViewNotFound)
				case errorCodeChangesetNotFound:
					return errors.WrapWith(errE, ErrChangesetNotFound)
				case internal.ErrorCodeUniqueViolation:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				}
			}
			return errE
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = c.view.name
		details["changeset"] = c.String()
	} else if c.view.store.Committed != nil {
		// There might be more than just this changeset committed if its parent changesets were not committed as well.
		for _, changeset := range committedChangesets {
			// We send over a non-initialized Changeset, requiring the receiver to reconstruct it.
			c.view.store.Committed <- Changeset[Data, Metadata, Patch]{
				Identifier: identifier.MustFromString(changeset),
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
	}
	var chs []Changeset[Data, Metadata, Patch]
	for _, changeset := range committedChangesets {
		id := identifier.MustFromString(changeset)
		if id == c.Identifier {
			chs = append(chs, *c)
		} else {
			ch, e := c.view.Changeset(ctx, id)
			if e != nil {
				return nil, errors.Join(errE, e)
			}
			chs = append(chs, ch)
		}
	}
	return chs, errE
}

// We allow discarding changesets even after they have been used as a parent changeset in some
// other changeset to allow one to prepare a chain changesets to commit. It is up to the higher
// levels to assure changesets and their patches are consistent before committing the chain.
// Discarding should anyway not be used on user-facing changesets.

// Discard deletes the changeset if it has not already been committed.
func (c *Changeset[Data, Metadata, Patch]) Discard(ctx context.Context) errors.E {
	arguments := []any{
		c.String(),
	}
	errE := internal.RetryTransaction(ctx, c.view.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `SELECT "changesetDiscard"($1)`, arguments...)
		if err != nil {
			errE := internal.WithPgxError(err)
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) {
				switch pgError.Code {
				case errorCodeAlreadyCommitted:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case errorCodeInUse:
					return errors.WrapWith(errE, ErrInUse)
				}
			}
			return errE
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

// Rollback discards the changeset but only if it has not already been committed.
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
			WITH "viewPath" AS (
				SELECT UNNEST("path") AS "view" FROM "currentViews" JOIN "views" USING ("view", "revision")
					WHERE "currentViews"."name"=$1
			), "viewChangesets" AS (
				SELECT "changeset" FROM "currentCommittedChangesets" JOIN "viewPath" USING ("view")
			)
			SELECT "id", "revision", "data", "metadata"`+patches+`
				FROM "currentChanges" JOIN "changes" USING ("changeset", "id", "revision")
					JOIN "viewChangesets" USING ("changeset")
				WHERE "changeset"=$2
		`, arguments...)
		if err != nil {
			return internal.WithPgxError(err)
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
		if err != nil {
			return internal.WithPgxError(err)
		}
		if len(changes) == 0 {
			return errors.WithStack(ErrChangesetNotFound)
		}
		return nil
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
