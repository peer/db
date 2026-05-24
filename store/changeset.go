package store

import (
	"context"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// TODO: We build query strings again and again based on patchesEnabled. We should create them once during Init and reuse them here.

// baseChangeset holds the identity and store association shared by Changeset
// and CommittedChangeset. The read-only accessors live here so both outer
// types get them via embedding.
type baseChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any] struct {
	id    identifier.Identifier
	store *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
}

// ID of this changeset.
//
// Each changeset has an immutable ID.
func (b baseChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) ID() identifier.Identifier {
	return b.id
}

func (b baseChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) String() string {
	return b.id.String()
}

// Store returns the Store associated with the changeset.
//
// It can return nil if Store is not associated with the changeset.
// You can use WithStore to associate it.
//
//nolint:lll
func (b baseChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Store() *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch] {
	return b.store
}

// Changeset is a batch of changes done to values.
//
// It can be prepared and later on committed to a view or discarded.
// It can be committed to multiple views.
//
// Only one change per value is allowed for a changeset.
type Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any] struct {
	baseChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
}

// WithStore returns a new Changeset object associated with the given Store.
func (c Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) WithStore(
	ctx context.Context, store *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch],
) (Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	return store.Changeset(ctx, c.id)
}

// CommittedChangeset represents a changeset that has been committed to a view.
type CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any] struct {
	baseChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]

	view string
}

// View returns the view this changeset is committed to.
func (cc CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) View() View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch] { //nolint:lll
	return View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
		name:  cc.view,
		store: cc.store,
	}
}

// Metadata returns the commit metadata persisted when this changeset was
// committed to the view.
func (cc CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Metadata( //nolint:ireturn
	ctx context.Context,
) (CommitMetadata, errors.E) {
	arguments := []any{
		cc.id.String(), cc.view,
	}
	var metadata CommitMetadata
	errE := internalStore.RetryTransaction(ctx, cc.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		metadata = *new(CommitMetadata)

		err := tx.QueryRow(ctx, `
			SELECT cc."metadata"
				FROM "`+cc.store.Prefix+`CurrentCommittedChangesets" cur
				JOIN "`+cc.store.Prefix+`CurrentViews" v ON v."view"=cur."view"
				JOIN "`+cc.store.Prefix+`CommittedChangesets" cc
					ON cc."changeset"=cur."changeset" AND cc."view"=cur."view" AND cc."revision"=cur."revision"
				WHERE cur."changeset"=$1 AND v."name"=$2`,
			arguments...).Scan(&metadata)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				// This store has no record of the changeset being committed to this view.
				// Commits are append-only, so this should normally not happen for a
				// CommittedChangeset obtained from Views on the same store. The typical
				// trigger is re-attaching via WithStore to a different store that does
				// not have this commit, or the view being renamed since this
				// CommittedChangeset was fetched.
				return errors.WrapWith(errE, ErrChangesetNotFound)
			}
			return errE
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["changeset"] = cc.id.String()
		details["view"] = cc.view
	}
	return metadata, errE
}

// WithStore returns a new CommittedChangeset object associated with the given Store.
func (cc CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) WithStore(
	ctx context.Context, store *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch],
) (CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	c, errE := store.Changeset(ctx, cc.id)
	if errE != nil {
		return CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{}, errE
	}
	return CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
		baseChangeset: c.baseChangeset,
		view:          cc.view,
	}, nil
}

// We allow changing changesets even after they have been used as a parent changeset in some
// other changeset to allow one to prepare a chain of changesets to commit. It is up to the higher
// levels to assure changesets and their patches are consistent before committing the chain.
// We check just that the chain has a reasonable series of changesets and that parent changesets
// are committed before children.

// Insert adds the insert change to the changeset.
//
// The changeset must not be already committed to any view.
func (c Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Insert(
	ctx context.Context, id identifier.Identifier, value Data, metadata Metadata,
) (Version, errors.E) {
	arguments := []any{
		c.String(), id.String(), value, metadata,
	}
	patchesEmptyValue := ""
	if c.store.patchesEnabled {
		patchesEmptyValue = ", '{}'" //nolint:goconst
	}
	var version Version
	errE := internalStore.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		version = Version{}

		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetCreate"($1, $2, '{}', $3, $4`+patchesEmptyValue+`)`, arguments...)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code {
				case errorCodeAlreadyCommitted:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case pgerrcode.UniqueViolation:
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
func (c Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Update(
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
	errE := internalStore.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		version = Version{}

		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetCreate"($1, $2, $3, $4, $5`+patchesPlaceholders+`)`, arguments...)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code {
				case errorCodeAlreadyCommitted:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case errorCodeParentInvalid:
					return errors.WrapWith(errE, ErrParentInvalid)
				case pgerrcode.UniqueViolation:
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
func (c Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Merge(
	ctx context.Context, id identifier.Identifier, parentChangesets []identifier.Identifier, value Data, patches []Patch, metadata Metadata,
) (Version, errors.E) {
	if c.store.patchesEnabled && len(parentChangesets) != len(patches) {
		return Version{}, errors.WithStack(ErrParentInvalid)
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
	errE := internalStore.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		version = Version{}

		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetCreate"($1, $2, $3, $4, $5`+patchesPlaceholders+`)`, arguments...)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code {
				case errorCodeAlreadyCommitted:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case errorCodeParentInvalid:
					return errors.WrapWith(errE, ErrParentInvalid)
				case pgerrcode.UniqueViolation:
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
func (c Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Replace(
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
	errE := internalStore.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		version = Version{}

		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetCreate"($1, $2, $3, $4, $5`+patchesEmptyValue+`)`, arguments...)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code {
				case errorCodeAlreadyCommitted:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case errorCodeParentInvalid:
					return errors.WrapWith(errE, ErrParentInvalid)
				case pgerrcode.UniqueViolation:
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
func (c Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Delete(
	ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata,
) (Version, errors.E) {
	arguments := []any{
		c.String(), id.String(), []string{parentChangeset.String()}, metadata,
	}
	patchesEmptyValue := ""
	if c.store.patchesEnabled {
		patchesEmptyValue = ", '{}'"
	}
	var version Version
	errE := internalStore.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		version = Version{}

		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetCreate"($1, $2, $3, NULL, $4`+patchesEmptyValue+`)`, arguments...)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code {
				case errorCodeAlreadyCommitted:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case errorCodeParentInvalid:
					return errors.WrapWith(errE, ErrParentInvalid)
				case pgerrcode.UniqueViolation:
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
func (c Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Commit(
	ctx context.Context, view View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], metadata CommitMetadata,
) ([]Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	arguments := []any{
		c.String(), metadata, view.name,
	}
	var committedChangesets []string
	errE := internalStore.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		committedChangesets = nil

		err := tx.QueryRow(ctx, `SELECT "`+c.store.Prefix+`ChangesetCommit"($1, $2, $3)`, arguments...).Scan(&committedChangesets)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code {
				case errorCodeViewNotFound:
					return errors.WrapWith(errE, ErrViewNotFound)
				case errorCodeChangesetNotFound:
					return errors.WrapWith(errE, ErrChangesetNotFound)
				case pgerrcode.UniqueViolation:
					return errors.WrapWith(errE, ErrAlreadyCommitted)
				case pgerrcode.ExclusionViolation:
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
	}
	var chs []Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
	for _, changeset := range committedChangesets {
		id := identifier.String(changeset)
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
func (c Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Discard(ctx context.Context) errors.E {
	arguments := []any{
		c.String(),
	}
	errE := internalStore.RetryTransaction(ctx, c.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `SELECT "`+c.store.Prefix+`ChangesetDiscard"($1)`, arguments...)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
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
func (c Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Rollback(ctx context.Context) errors.E {
	errE := c.Discard(ctx)
	if errE != nil && errors.Is(errE, ErrAlreadyCommitted) {
		return nil
	}
	return errE
}

// TODO: Should we provide also "archive" for archiving user-facing changesets (instead of rollback/discard).
//       When they will not be used anymore, but we should keep them around (and we want to
//       prevent accidentally changing them.)

// Views returns one CommittedChangeset per view this changeset has been committed to.
//
// The result is empty (with no error) when the changeset has not been committed anywhere.
func (c Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Views(
	ctx context.Context,
) ([]CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	arguments := []any{
		c.String(),
	}
	var committed []CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
	errE := internalStore.RetryTransaction(ctx, c.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		committed = nil

		rows, err := tx.Query(ctx, `
			SELECT v."name"
				FROM "`+c.store.Prefix+`CurrentCommittedChangesets" cur
				JOIN "`+c.store.Prefix+`CurrentViews" v USING ("view")
				WHERE cur."changeset"=$1
				ORDER BY v."name"
		`, arguments...)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		var view string
		_, err = pgx.ForEachRow(rows, []any{&view}, func() error {
			committed = append(committed, CommittedChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
				baseChangeset: c.baseChangeset,
				view:          view,
			})
			return nil
		})
		return internalStore.WithPgxError(err)
	})
	if errE != nil {
		details := errors.Details(errE)
		details["changeset"] = c.String()
	}
	return committed, errE
}

// Change represents a change to the value.
type Change struct {
	// ID of the value.
	ID identifier.Identifier

	// Version of the change.
	Version Version
}

// Changes returns up to MaxPageLength changes of the changeset, ordered by ID, after optional ID, to support keyset pagination.
func (b baseChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Changes(
	ctx context.Context, after *identifier.Identifier,
) ([]Change, errors.E) {
	arguments := []any{
		b.String(),
	}
	afterCondition := ""
	if after != nil {
		arguments = append(arguments, after.String())
		// We want to make sure that after value really exists.
		afterCondition = `AND EXISTS (SELECT 1 FROM "` + b.store.Prefix + `CurrentChanges" WHERE "changeset"=$1 AND "id"=$2) AND "id">$2`
	}
	var changes []Change
	errE := internalStore.RetryTransaction(ctx, b.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		changes = nil

		rows, err := tx.Query(ctx, `
			SELECT "id", "revision"	FROM "`+b.store.Prefix+`CurrentChanges"
			WHERE "changeset"=$1
			`+afterCondition+`
			ORDER BY "id"
			LIMIT `+maxPageLengthStr, arguments...)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		var id string
		var revision int64
		_, err = pgx.ForEachRow(rows, []any{&id, &revision}, func() error {
			changes = append(changes, Change{
				ID: identifier.String(id),
				Version: Version{
					Changeset: b.id,
					Revision:  revision,
				},
			})
			return nil
		})
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		if len(changes) == 0 {
			if after == nil {
				return errors.WithStack(ErrChangesetNotFound)
			}
			// TODO: Is there a better way to check without doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+b.store.Prefix+`CurrentChanges" WHERE "changeset"=$1)`, b.String()).Scan(&exists)
			if err != nil {
				return internalStore.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrChangesetNotFound)
			}
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+b.store.Prefix+`CurrentChanges" WHERE "changeset"=$1 AND "id"=$2)`, arguments...).Scan(&exists)
			if err != nil {
				return internalStore.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrValueNotFound)
			}
			// There is nothing wrong with having no values.
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["changeset"] = b.String()
	}
	return changes, errE
}

// TODO: Add a method which returns patches for a requested change.

// Get returns the data and metadata for the value at the given version in this changeset.
//
// If revision is 0, the value with the latest revision is returned
// and returned version contains this revision number.
//
// If value has been deleted at a given version, ErrValueDeleted error is returned,
// but other returned values are valid as well.
func (b baseChangeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Get( //nolint:ireturn
	ctx context.Context, id identifier.Identifier, revision int64,
) (Data, Metadata, Version, []Version, errors.E) {
	arguments := []any{
		id.String(), b.String(),
	}
	revisionCondition := ""
	if revision > 0 {
		arguments = append(arguments, revision)
		revisionCondition = `AND "revision"=$3`
	} else {
		revisionCondition = `ORDER BY "revision" DESC LIMIT 1`
	}
	var data Data
	var metadata Metadata
	var resolved Version
	var parentChangesets []Version
	errE := internalStore.RetryTransaction(ctx, b.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		data = *new(Data)
		metadata = *new(Metadata)
		resolved = Version{}
		parentChangesets = nil

		var dataIsNull bool
		var resolvedRevision int64
		var parentChangesetsString []string
		err := tx.QueryRow(ctx, `
			SELECT "revision", "data", "data" IS NULL, "metadata", "parentChangesets"
				FROM "`+b.store.Prefix+`Changes"
				WHERE "id"=$1 AND "changeset"=$2
				`+revisionCondition,
			arguments...).Scan(&resolvedRevision, &data, &dataIsNull, &metadata, &parentChangesetsString)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				return errors.WrapWith(errE, ErrValueNotFound)
			}
			return errE
		}
		resolved.Changeset = b.id
		resolved.Revision = resolvedRevision
		for _, s := range parentChangesetsString {
			parentChangesets = append(parentChangesets, Version{Changeset: identifier.String(s), Revision: 0})
		}
		if dataIsNull {
			// We return an error because this method is asking for a particular version of the value
			// but the value does not exist anymore at this version. Other returned values are valid though.
			return errors.WithStack(ErrValueDeleted)
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["id"] = id.String()
		details["changeset"] = b.String()
		details["revision"] = revision
	}
	return data, metadata, resolved, parentChangesets, errE
}
