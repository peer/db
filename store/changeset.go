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

// Changeset is a batch of changes done to values.
//
// It can be prepared and later on committed to a view or discarded.
// It can be committed to multiple views.
//
// Only one change per value is allowed for a changeset.
type Changeset[Data, Metadata, Patch any] struct {
	id    identifier.Identifier
	store *Store[Data, Metadata, Patch]
}

// ID of this changeset.
//
// Each changeset has an immutable randomly generated ID.
func (c Changeset[Data, Metadata, Patch]) ID() identifier.Identifier {
	return c.id
}

func (c Changeset[Data, Metadata, Patch]) String() string {
	return c.id.String()
}

// Store returns the Store associated with the changeset.
//
// It can return nil if Store is not associated with the changeset.
// You can use WithStore to associate it.
func (c Changeset[Data, Metadata, Patch]) Store() *Store[Data, Metadata, Patch] {
	return c.store
}

// WithStore returns a new Changeset object associated with the given Store.
func (c Changeset[Data, Metadata, Patch]) WithStore(ctx context.Context, store *Store[Data, Metadata, Patch]) (Changeset[Data, Metadata, Patch], errors.E) {
	return store.Changeset(ctx, c.id)
}

// We allow changing changesets even after they have been used as a parent changeset in some
// other changeset to allow one to prepare a chain of changesets to commit. It is up to the higher
// levels to assure changesets and their patches are consistent before committing the chain.
// We check just that the chain has a reasonable series of changesets and that parent changesets
// are committed before children.

// Insert adds the insert change to the changeset.
//
// The changeset must not be already committed to any view.
func (c Changeset[Data, Metadata, Patch]) Insert(ctx context.Context, id identifier.Identifier, value Data, metadata Metadata) (Version, errors.E) {
	arguments := []any{
		c.String(), id.String(), value, metadata,
	}
	patchesEmptyValue := ""
	if c.store.patchesEnabled {
		patchesEmptyValue = ", '{}'" //nolint:goconst
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		version = Version{}

		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetCreate"($1, $2, '{}', $3, $4`+patchesEmptyValue+`)`, arguments...) //nolint:goconst
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
		version.Changeset = c.id
		version.Revision = 1
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["id"] = id.String()
		details["changeset"] = c.String()
	}
	return version, errE
}

// Update adds the update change to the changeset.
//
// The changeset must not be already committed to any view.
// The parent changeset must include a change to the same value.
//
// Patch is a forward patch from the value at parent changeset version
// to the new value version. It is up to the higher levels to assure
// consistency between the patch and values (from the perspective of
// the Store the patch is an opaque value to store).
func (c Changeset[Data, Metadata, Patch]) Update(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, patch Patch, metadata Metadata,
) (Version, errors.E) {
	arguments := []any{
		c.String(), id.String(), []string{parentChangeset.String()}, value, metadata,
	}
	patchesPlaceholders := ""
	if c.store.patchesEnabled {
		arguments = append(arguments, []Patch{patch})
		patchesPlaceholders = ", $6"
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		version = Version{}

		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetCreate"($1, $2, $3, $4, $5`+patchesPlaceholders+`)`, arguments...) //nolint:goconst
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
		version.Changeset = c.id
		version.Revision = 1
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["id"] = id.String()
		details["changeset"] = c.String()
		details["parentChangeset"] = parentChangeset.String()
	}
	return version, errE
}

// Merge adds the merge change to the changeset.
//
// The changeset must not be already committed to any view.
// The parent changesets must include a change to the same value.
//
// Merge is similar to Update only that multiple parent changesets
// and patches can be provided. The number and order of parent changesets
// and patches should match. Each patch should be a forward patch from
// the corresponding value at that parent changeset version to the new
// value version. All patches must result in the same value. It is up
// to the higher levels to assure consistency between patches and values
// (from the perspective of the Store patches are opaque values to store).
func (c Changeset[Data, Metadata, Patch]) Merge(
	ctx context.Context, id identifier.Identifier, parentChangesets []identifier.Identifier, value Data, patches []Patch, metadata Metadata,
) (Version, errors.E) {
	if c.store.patchesEnabled && len(parentChangesets) != len(patches) {
		return Version{}, errors.WithStack(ErrParentInvalid) //nolint:exhaustruct
	}
	parentChangesetsString := []string{}
	for _, p := range parentChangesets {
		parentChangesetsString = append(parentChangesetsString, p.String())
	}
	arguments := []any{
		c.String(), id.String(), parentChangesetsString, value, metadata,
	}
	patchesPlaceholders := ""
	if c.store.patchesEnabled {
		arguments = append(arguments, patches)
		patchesPlaceholders = ", $6"
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		version = Version{}

		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetCreate"($1, $2, $3, $4, $5`+patchesPlaceholders+`)`, arguments...)
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
		version.Changeset = c.id
		version.Revision = 1
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["id"] = id.String()
		details["changeset"] = c.String()
		details["parentChangesets"] = parentChangesetsString
	}
	return version, errE
}

// Replace adds the replace change to the changeset.
//
// The changeset must not be already committed to any view.
// The parent changeset must include a change to the same value.
//
// Replace is similar to Update only that the forward patch is not stored.
// Replace is useful when Store is configured with patches, but for a
// particular change the patch makes little sense, e.g., the whole value
// is replaced with a new value and the patch would be a copy of the
// whole new value or even larger than the value itself. Or maybe the
// patch is simply not available.
func (c Changeset[Data, Metadata, Patch]) Replace(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, metadata Metadata,
) (Version, errors.E) {
	arguments := []any{
		c.String(), id.String(), []string{parentChangeset.String()}, value, metadata,
	}
	patchesEmptyValue := ""
	if c.store.patchesEnabled {
		patchesEmptyValue = ", '{}'"
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		version = Version{}

		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetCreate"($1, $2, $3, $4, $5`+patchesEmptyValue+`)`, arguments...)
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
		version.Changeset = c.id
		version.Revision = 1
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["id"] = id.String()
		details["changeset"] = c.String()
		details["parentChangeset"] = parentChangeset.String()
	}
	return version, errE
}

// Delete adds the delete change to the changeset.
//
// The changeset must not be already committed to any view.
// The parent changeset must include a change to the same value.
//
// Delete does not really delete anything from the store, it only
// marks the value as deleted. Previous versions of the value are
// still available.
func (c Changeset[Data, Metadata, Patch]) Delete(ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata) (Version, errors.E) {
	arguments := []any{
		c.String(), id.String(), []string{parentChangeset.String()}, metadata,
	}
	patchesEmptyValue := ""
	if c.store.patchesEnabled {
		patchesEmptyValue = ", '{}'"
	}
	var version Version
	errE := internal.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		version = Version{}

		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetCreate"($1, $2, $3, NULL, $4`+patchesEmptyValue+`)`, arguments...)
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
		version.Changeset = c.id
		version.Revision = 1
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["id"] = id.String()
		details["changeset"] = c.String()
		details["parentChangeset"] = parentChangeset.String()
	}
	return version, errE
}

// TODO: How to make sure is committing/discarding the version of changeset they expect?
//       There is a race condition between decision to commit/discard and until it is done.

// TODO: Provide a way to access commit metadata (e.g., list all commits for a view).

// Commit commits the changeset to the view.
//
// It commits any non-committed ancestor changesets as well.
// It returns a slice of committed changesets.
//
// The changeset together with any non-committed ancestor changesets must
// not introduce multiple concurrent versions of a value.
func (c Changeset[Data, Metadata, Patch]) Commit(
	ctx context.Context, view View[Data, Metadata, Patch], metadata Metadata,
) ([]Changeset[Data, Metadata, Patch], errors.E) {
	arguments := []any{
		c.String(), metadata, view.name,
	}
	var committedChangesets []string
	errE := internal.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		committedChangesets = nil

		err := tx.QueryRow(ctx, `SELECT "`+c.store.Prefix+`ChangesetCommit"($1, $2, $3)`, arguments...).Scan(&committedChangesets)
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
				case internal.ErrorExclusionViolation:
					return errors.WrapWith(errE, ErrConflict)
				}
			}
			return errE
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = view.name
		details["changeset"] = c.String()
	} else if c.store.Committed != nil {
		// There might be more than just this changeset committed if its parent changesets were not committed as well.
		for _, changeset := range committedChangesets {
			// We send over a changeset and view without store, requiring the receiver to use WithStore on them.
			c.store.Committed <- CommittedChangeset[Data, Metadata, Patch]{
				Changeset: Changeset[Data, Metadata, Patch]{
					id:    identifier.MustFromString(changeset),
					store: nil,
				},
				View: View[Data, Metadata, Patch]{
					name:  view.name,
					store: nil,
				},
			}
		}
	}
	var chs []Changeset[Data, Metadata, Patch]
	for _, changeset := range committedChangesets {
		id := identifier.MustFromString(changeset)
		if id == c.id {
			chs = append(chs, c)
		} else {
			ch, e := c.store.Changeset(ctx, id)
			if e != nil {
				return nil, errors.Join(errE, e)
			}
			chs = append(chs, ch)
		}
	}
	return chs, errE
}

// Discard deletes the changeset.
//
// The changeset must not be already committed to any view.
// The changeset must not be used as a parent changeset by any other changeset.
//
// Discard cannot be undone.
func (c Changeset[Data, Metadata, Patch]) Discard(ctx context.Context) errors.E {
	arguments := []any{
		c.String(),
	}
	errE := internal.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetDiscard"($1)`, arguments...)
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
		details["changeset"] = c.String()
	}
	return errE
}

// Rollback discards the changeset but only if it has not already been committed.
//
// If the changeset has already been committed to any view, it is a noop.
// The changeset must not be used as a parent changeset by any other changeset.
//
// Rollback cannot be undone.
func (c Changeset[Data, Metadata, Patch]) Rollback(ctx context.Context) errors.E {
	errE := c.Discard(ctx)
	if errE != nil && errors.Is(errE, ErrAlreadyCommitted) {
		return nil
	}
	return errE
}

// TODO: Should we provide also "archive" for archiving user-facing changesets (instead of discard).
//       When they will not be used anymore, but we should keep them around (and we want to
//       prevent accidentally changing them.)

// Change represents a change to the value.
type Change struct {
	// ID of the value.
	ID identifier.Identifier

	// Version of the change.
	Version Version
}

// Changes returns up to MaxPageLength changes of the changeset, ordered by ID, after optional ID, to support keyset pagination.
func (c Changeset[Data, Metadata, Patch]) Changes(ctx context.Context, after *identifier.Identifier) ([]Change, errors.E) {
	arguments := []any{
		c.String(),
	}
	afterCondition := ""
	if after != nil {
		arguments = append(arguments, after.String())
		// We want to make sure that after value really exists.
		afterCondition = `AND EXISTS (SELECT 1 FROM "` + c.store.Prefix + `CurrentChanges" WHERE "changeset"=$1 AND "id"=$2) AND "id">$2`
	}
	var changes []Change
	errE := internal.RetryTransaction(ctx, c.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		changes = nil

		rows, err := tx.Query(ctx, `
			SELECT "id", "revision"	FROM "`+c.store.Prefix+`CurrentChanges"
			WHERE "changeset"=$1
			`+afterCondition+`
			ORDER BY "id"
			LIMIT `+maxPageLengthStr, arguments...)
		if err != nil {
			return internal.WithPgxError(err)
		}
		var id string
		var revision int64
		_, err = pgx.ForEachRow(rows, []any{&id, &revision}, func() error {
			changes = append(changes, Change{
				ID: identifier.MustFromString(id),
				Version: Version{
					Changeset: c.id,
					Revision:  revision,
				},
			})
			return nil
		})
		if err != nil {
			return internal.WithPgxError(err)
		}
		if len(changes) == 0 {
			if after == nil {
				return errors.WithStack(ErrChangesetNotFound)
			}
			// TODO: Is there a better way to check without doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+c.store.Prefix+`CurrentChanges" WHERE "changeset"=$1)`, c.String()).Scan(&exists) //nolint:goconst
			if err != nil {
				return internal.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrChangesetNotFound)
			}
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+c.store.Prefix+`CurrentChanges" WHERE "changeset"=$1 AND "id"=$2)`, arguments...).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrValueNotFound)
			}
			// There is nothing wrong with having no values.
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["changeset"] = c.String()
	}
	return changes, errE
}
